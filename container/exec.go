// Package container implements the exec operation.
package container

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/term"

	"runc-go/spec"
)

// ExecOptions contains options for exec.
type ExecOptions struct {
	// Tty allocates a pseudo-TTY.
	Tty bool

	// User specifies the user to run as (uid:gid).
	User string

	// Cwd is the working directory inside the container.
	Cwd string

	// Env are additional environment variables.
	Env []string

	// Detach runs the process in the background.
	Detach bool

	// PidFile writes the process ID to a file.
	PidFile string

	// ConsoleSocket is the path to a unix socket for PTY master.
	ConsoleSocket string
}

// ExecWithProcessFile executes using a process spec file (Docker/containerd style).
func ExecWithProcessFile(containerID, stateRoot, processFile string, opts *ExecOptions) error {
	// Read and parse the process spec file
	data, err := os.ReadFile(processFile)
	if err != nil {
		return fmt.Errorf("read process file: %w", err)
	}

	var process spec.Process
	if err := json.Unmarshal(data, &process); err != nil {
		return fmt.Errorf("parse process file: %w", err)
	}

	// Extract args from process spec
	if len(process.Args) == 0 {
		return fmt.Errorf("no command in process spec")
	}

	// Update options from process spec
	if process.Terminal {
		opts.Tty = true
	}
	if process.Cwd != "" {
		opts.Cwd = process.Cwd
	}
	opts.Env = append(opts.Env, process.Env...)

	return Exec(containerID, stateRoot, process.Args, opts)
}

// Exec executes a new process inside a running container.
func Exec(containerID, stateRoot string, args []string, opts *ExecOptions) error {
	if opts == nil {
		opts = &ExecOptions{}
	}

	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Load container
	c, err := Load(containerID, stateRoot)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	// Check if container is running
	c.RefreshStatus()
	if c.State.Status != spec.StatusRunning {
		return fmt.Errorf("container is not running (status: %s)", c.State.Status)
	}

	if c.InitProcess <= 0 {
		return fmt.Errorf("container has no init process")
	}

	// Get path to our own executable for re-exec
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	// Build the exec-init command
	cmd := exec.Command(self, "exec-init")

	// Pass information via environment
	encodedArgs := encodeArgs(args)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("_RUNC_GO_EXEC_PID=%d", c.InitProcess),
		fmt.Sprintf("_RUNC_GO_EXEC_ROOTFS=%s", c.State.Rootfs),
		fmt.Sprintf("_RUNC_GO_EXEC_CWD=%s", getCwd(opts, c)),
		fmt.Sprintf("_RUNC_GO_EXEC_ARGS=%s", encodedArgs),
	)

	// Add additional env vars
	for _, e := range opts.Env {
		cmd.Env = append(cmd.Env, "_RUNC_GO_EXEC_ENV_"+e)
	}

	// Handle TTY with console socket (containerd style)
	if opts.Tty && opts.ConsoleSocket != "" {
		return execWithConsoleSocket(cmd, opts)
	}

	// Handle TTY without console socket (direct terminal)
	if opts.Tty {
		cmd.Env = append(cmd.Env, "_RUNC_GO_EXEC_TTY=1")
		return execWithPTY(cmd, opts)
	}

	// Non-TTY mode: just pass through stdin/stdout/stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start exec process: %w", err)
	}

	// Write PID file if requested
	if opts.PidFile != "" {
		pidContent := fmt.Sprintf("%d", cmd.Process.Pid)
		if err := os.WriteFile(opts.PidFile, []byte(pidContent), 0644); err != nil {
			cmd.Process.Kill()
			return fmt.Errorf("write pid file: %w", err)
		}
	}

	// If detached, exit immediately
	if opts.Detach {
		return nil
	}

	// Wait for the process to complete
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}

// execWithPTY runs the command with a pseudo-terminal for interactive use.
func execWithPTY(cmd *exec.Cmd, opts *ExecOptions) error {
	// Open PTY master
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/ptmx: %w", err)
	}
	// Note: ptmx is closed explicitly after cmd.Wait() to signal EOF

	// Get the slave PTY number
	var ptyNum uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNum))); errno != 0 {
		return fmt.Errorf("get pty number: %v", errno)
	}

	// Unlock the slave PTY
	var unlock int32 = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		return fmt.Errorf("unlock pty: %v", errno)
	}

	// Open slave PTY
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open slave pty %s: %w", slavePath, err)
	}
	defer slave.Close()

	// Set up the command to use the slave PTY
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	// Put terminal into raw mode (only if stdin is a terminal)
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make terminal raw: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		// Copy terminal size to PTY
		copyTerminalSize(os.Stdin, ptmx)

		// Handle window size changes
		sigwinch := make(chan os.Signal, 1)
		signal.Notify(sigwinch, syscall.SIGWINCH)
		go func() {
			for range sigwinch {
				copyTerminalSize(os.Stdin, ptmx)
			}
		}()
		defer signal.Stop(sigwinch)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start exec process: %w", err)
	}

	// Close slave in parent (child has it)
	slave.Close()

	// Write PID file if requested
	if opts.PidFile != "" {
		pidContent := fmt.Sprintf("%d", cmd.Process.Pid)
		if err := os.WriteFile(opts.PidFile, []byte(pidContent), 0644); err != nil {
			cmd.Process.Kill()
			return fmt.Errorf("write pid file: %w", err)
		}
	}

	// Copy I/O between terminal and PTY
	go func() {
		io.Copy(ptmx, os.Stdin)
	}()

	outputDone := make(chan struct{})
	go func() {
		io.Copy(os.Stdout, ptmx)
		close(outputDone)
	}()

	// Wait for the process to complete
	err = cmd.Wait()

	// Close PTY to signal EOF to output goroutine
	ptmx.Close()

	// Wait for output to be flushed
	<-outputDone

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}

// execWithConsoleSocket runs with PTY and sends master FD to console socket.
// This is used by containerd to handle the PTY I/O.
func execWithConsoleSocket(cmd *exec.Cmd, opts *ExecOptions) error {
	// Open PTY master
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/ptmx: %w", err)
	}
	defer ptmx.Close()

	// Get the slave PTY number
	var ptyNum uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNum))); errno != 0 {
		return fmt.Errorf("get pty number: %v", errno)
	}

	// Unlock the slave PTY
	var unlock int32 = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		return fmt.Errorf("unlock pty: %v", errno)
	}

	// Open slave PTY
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open slave pty %s: %w", slavePath, err)
	}
	defer slave.Close()

	// Set up the command to use the slave PTY
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start exec process: %w", err)
	}

	// Close slave in parent (child has it)
	slave.Close()

	// Send PTY master to console socket
	conn, err := net.Dial("unix", opts.ConsoleSocket)
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("connect to console socket: %w", err)
	}
	defer conn.Close()

	// Send the PTY master FD over the unix socket
	unixConn := conn.(*net.UnixConn)
	f, err := unixConn.File()
	if err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("get socket file: %w", err)
	}
	defer f.Close()

	rights := syscall.UnixRights(int(ptmx.Fd()))
	if err := syscall.Sendmsg(int(f.Fd()), []byte{0}, rights, nil, 0); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("send pty fd: %w", err)
	}

	// Write PID file if requested
	if opts.PidFile != "" {
		pidContent := fmt.Sprintf("%d", cmd.Process.Pid)
		if err := os.WriteFile(opts.PidFile, []byte(pidContent), 0644); err != nil {
			cmd.Process.Kill()
			return fmt.Errorf("write pid file: %w", err)
		}
	}

	// If detached, exit immediately
	if opts.Detach {
		return nil
	}

	// Wait for the process to complete
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	return nil
}

// copyTerminalSize copies the terminal size from src to dst.
func copyTerminalSize(src, dst *os.File) {
	width, height, err := term.GetSize(int(src.Fd()))
	if err != nil {
		return
	}
	setTerminalSize(dst, width, height)
}

// winsize is the struct for TIOCSWINSZ ioctl.
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// setTerminalSize sets the terminal size.
func setTerminalSize(f *os.File, width, height int) {
	ws := winsize{
		Row: uint16(height),
		Col: uint16(width),
	}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}

// ExecInit is called to actually join the container and exec.
// This uses nsenter to properly join all namespaces (including mount).
func ExecInit() error {
	// Get parameters from environment
	pidStr := os.Getenv("_RUNC_GO_EXEC_PID")
	cwd := os.Getenv("_RUNC_GO_EXEC_CWD")
	argsStr := os.Getenv("_RUNC_GO_EXEC_ARGS")

	if pidStr == "" || argsStr == "" {
		return fmt.Errorf("missing exec environment variables")
	}

	args := decodeArgs(argsStr)
	if len(args) == 0 {
		return fmt.Errorf("no command to execute")
	}

	// Collect additional environment variables
	var extraEnv []string
	for _, e := range os.Environ() {
		if len(e) > 18 && e[:18] == "_RUNC_GO_EXEC_ENV_" {
			extraEnv = append(extraEnv, e[18:])
		}
	}

	// Build nsenter command to join all namespaces
	// nsenter -t <pid> -m -u -i -n -p [--wd <cwd>] <command>
	nsenterArgs := []string{
		"-t", pidStr,
		"-m", // mount namespace
		"-u", // UTS namespace
		"-i", // IPC namespace
		"-n", // network namespace
		"-p", // PID namespace
	}

	// Add separator and the command to execute
	nsenterArgs = append(nsenterArgs, "--")

	// If cwd specified, use sh -c with cd
	if cwd != "" && cwd != "/" {
		shellCmd := fmt.Sprintf("cd %s && exec %s", cwd, shellQuoteArgs(args))
		nsenterArgs = append(nsenterArgs, "sh", "-c", shellCmd)
	} else {
		nsenterArgs = append(nsenterArgs, args...)
	}

	// Build environment (filter out our internal vars, add container PATH)
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"TERM=xterm",
	}
	for _, e := range os.Environ() {
		if len(e) < 13 || e[:13] != "_RUNC_GO_EXEC" {
			// Skip PATH since we set container-appropriate one above
			if len(e) > 5 && e[:5] == "PATH=" {
				continue
			}
			env = append(env, e)
		}
	}
	env = append(env, extraEnv...)

	// Find nsenter
	nsenterPath, err := exec.LookPath("nsenter")
	if err != nil {
		return fmt.Errorf("nsenter not found: %w", err)
	}

	// Exec nsenter (replaces this process)
	return syscall.Exec(nsenterPath, append([]string{"nsenter"}, nsenterArgs...), env)
}

// getCwd returns the working directory for exec.
func getCwd(opts *ExecOptions, c *Container) string {
	if opts.Cwd != "" {
		return opts.Cwd
	}
	if c.Spec != nil && c.Spec.Process != nil && c.Spec.Process.Cwd != "" {
		return c.Spec.Process.Cwd
	}
	return "/"
}

// encodeArgs encodes command arguments for environment variable passing.
func encodeArgs(args []string) string {
	// Use JSON encoding to handle all characters
	data, _ := json.Marshal(args)
	return string(data)
}

// decodeArgs decodes command arguments from environment variable.
func decodeArgs(encoded string) []string {
	if encoded == "" {
		return nil
	}
	var args []string
	json.Unmarshal([]byte(encoded), &args)
	return args
}

// shellQuoteArgs quotes arguments for shell.
func shellQuoteArgs(args []string) string {
	var quoted []string
	for _, arg := range args {
		// Simple quoting - wrap in single quotes, escape existing single quotes
		escaped := ""
		for _, c := range arg {
			if c == '\'' {
				escaped += `'\''`
			} else {
				escaped += string(c)
			}
		}
		quoted = append(quoted, "'"+escaped+"'")
	}
	return fmt.Sprintf("%s", joinStrings(quoted, " "))
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
