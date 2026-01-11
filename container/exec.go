// Package container implements the exec operation.
package container

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/term"

	"runc-go/linux"
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
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("_RUNC_GO_EXEC_PID=%d", c.InitProcess),
		fmt.Sprintf("_RUNC_GO_EXEC_ROOTFS=%s", c.State.Rootfs),
		fmt.Sprintf("_RUNC_GO_EXEC_CWD=%s", getCwd(opts, c)),
		fmt.Sprintf("_RUNC_GO_EXEC_ARGS=%s", encodeArgs(args)),
	)

	// Add additional env vars
	for _, e := range opts.Env {
		cmd.Env = append(cmd.Env, "_RUNC_GO_EXEC_ENV_"+e)
	}

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
	done := make(chan struct{})
	go func() {
		io.Copy(ptmx, os.Stdin)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(os.Stdout, ptmx)
		done <- struct{}{}
	}()

	// Wait for the process to complete
	err = cmd.Wait()

	// Wait for I/O goroutines
	<-done

	if err != nil {
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
// This runs in a separate process that will join namespaces.
func ExecInit() error {
	// Get parameters from environment
	pidStr := os.Getenv("_RUNC_GO_EXEC_PID")
	rootfs := os.Getenv("_RUNC_GO_EXEC_ROOTFS")
	cwd := os.Getenv("_RUNC_GO_EXEC_CWD")
	argsStr := os.Getenv("_RUNC_GO_EXEC_ARGS")

	if pidStr == "" || argsStr == "" {
		return fmt.Errorf("missing exec environment variables")
	}

	var pid int
	fmt.Sscanf(pidStr, "%d", &pid)

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

	// Join namespaces of the container
	nsTypes := []struct {
		name   string
		nsType spec.LinuxNamespaceType
	}{
		{"ipc", spec.IPCNamespace},
		{"uts", spec.UTSNamespace},
		{"net", spec.NetworkNamespace},
		{"pid", spec.PIDNamespace},
		{"mnt", spec.MountNamespace},
		{"cgroup", spec.CgroupNamespace},
	}

	for _, ns := range nsTypes {
		nsPath := filepath.Join("/proc", pidStr, "ns", ns.name)
		if _, err := os.Stat(nsPath); err == nil {
			if err := joinNamespace(nsPath, ns.nsType); err != nil {
				// Some namespaces may not be available, continue
				fmt.Fprintf(os.Stderr, "[exec] warning: join %s namespace: %v\n", ns.name, err)
			}
		}
	}

	// Change to container's root if available
	if rootfs != "" {
		// We're already in the mount namespace, so we should be able to
		// access the container's view of the filesystem.
		// The root from the container's perspective is /
		if err := syscall.Chdir("/"); err != nil {
			fmt.Fprintf(os.Stderr, "[exec] warning: chdir /: %v\n", err)
		}
	}

	// Change to working directory
	if cwd != "" {
		if err := syscall.Chdir(cwd); err != nil {
			return fmt.Errorf("chdir %s: %w", cwd, err)
		}
	}

	// Clear the internal env vars and add extra ones
	env := []string{}
	for _, e := range os.Environ() {
		if len(e) < 13 || e[:13] != "_RUNC_GO_EXEC" {
			env = append(env, e)
		}
	}
	env = append(env, extraEnv...)

	// Find the executable
	execPath, err := exec.LookPath(args[0])
	if err != nil {
		// Try with /proc/1/root prefix if in container
		containerPath := filepath.Join("/proc", pidStr, "root", args[0])
		if _, statErr := os.Stat(containerPath); statErr == nil {
			execPath = containerPath
		} else {
			return fmt.Errorf("executable not found: %s", args[0])
		}
	}

	// Exec the command
	return syscall.Exec(execPath, args, env)
}

// joinNamespace joins a namespace by path.
func joinNamespace(path string, nsType spec.LinuxNamespaceType) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer syscall.Close(fd)

	_, _, errno := syscall.Syscall(linux.SYS_SETNS, uintptr(fd), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
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
