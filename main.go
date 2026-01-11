// runc-go is an OCI-compliant container runtime.
//
// This is an educational implementation that follows the OCI Runtime Specification.
// It can be used as a drop-in replacement for runc with Docker or other container engines.
//
// Commands:
//
//	create  - Create a container (but don't start it)
//	start   - Start a created container
//	run     - Create and start a container
//	state   - Output the state of a container
//	kill    - Send a signal to a container
//	delete  - Delete a container
//	list    - List containers
//	spec    - Generate a default OCI spec
//	init    - Internal command for container initialization
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"runc-go/container"
	"runc-go/spec"
)

const (
	version   = "0.1.0"
	specVer   = "1.0.2"
	stateRoot = "/run/runc-go"
)

// Global options (Docker/containerd compatibility)
var (
	globalRoot      string
	globalLog       string
	globalLogFormat string
	logWriter       io.Writer = os.Stderr
)

// logJSON writes a JSON log entry if logging is configured
func logJSON(level, msg string) {
	if globalLogFormat == "json" && globalLog != "" {
		entry := map[string]interface{}{
			"level": level,
			"msg":   msg,
			"time":  time.Now().Format(time.RFC3339Nano),
		}
		data, _ := json.Marshal(entry)
		fmt.Fprintln(logWriter, string(data))
	}
}

func main() {
	// Parse global flags first (Docker/containerd puts them before the command)
	args := os.Args[1:]
	for len(args) > 0 {
		switch {
		case args[0] == "--root" && len(args) > 1:
			globalRoot = args[1]
			args = args[2:]
		case len(args[0]) > 7 && args[0][:7] == "--root=":
			globalRoot = args[0][7:]
			args = args[1:]
		case args[0] == "--log" && len(args) > 1:
			globalLog = args[1]
			args = args[2:]
		case len(args[0]) > 6 && args[0][:6] == "--log=":
			globalLog = args[0][6:]
			args = args[1:]
		case args[0] == "--log-format" && len(args) > 1:
			globalLogFormat = args[1]
			args = args[2:]
		case len(args[0]) > 13 && args[0][:13] == "--log-format=":
			globalLogFormat = args[0][13:]
			args = args[1:]
		case args[0] == "--systemd-cgroup":
			// Accept but ignore for now
			args = args[1:]
		case args[0] == "--debug":
			// Accept but ignore for now
			args = args[1:]
		default:
			// Found the command
			goto parseCommand
		}
	}

parseCommand:
	// Setup logging if configured
	if globalLog != "" {
		f, err := os.OpenFile(globalLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			logWriter = f
			defer f.Close()
		}
	}

	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	var err error
	cmd := args[0]
	os.Args = append([]string{os.Args[0], cmd}, args[1:]...)

	switch cmd {
	case "create":
		err = cmdCreate()
	case "start":
		err = cmdStart()
	case "run":
		err = cmdRun()
	case "state":
		err = cmdState()
	case "kill":
		err = cmdKill()
	case "delete":
		err = cmdDelete()
	case "list", "ps":
		err = cmdList()
	case "spec":
		err = cmdSpec()
	case "init":
		err = cmdInit()
	case "exec":
		err = cmdExec()
	case "exec-init":
		err = cmdExecInit()
	case "version", "--version", "-v":
		fmt.Printf("runc-go version %s\n", version)
		fmt.Printf("spec: %s\n", specVer)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		logJSON("error", err.Error())
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`runc-go - OCI Container Runtime

Usage:
  runc-go <command> [options] [arguments]

Commands:
  create <container-id> <bundle>   Create a container
  start <container-id>             Start a created container
  run <container-id> <bundle>      Create and start a container
  exec <container-id> <command>    Execute a command in a running container
  state <container-id>             Output the state of a container
  kill <container-id> [signal]     Send a signal to a container (default: SIGTERM)
  delete <container-id>            Delete a container
  list                             List containers
  spec                             Generate a default OCI spec

Options:
  --root <path>        State directory (default: /run/runc-go)
  --bundle <path>      Bundle directory (alternative to positional arg)
  --pid-file <path>    Write container PID to file
  --console-socket     Socket for console (not implemented)
  -f, --force          Force action (for delete)
  -t, --tty            Allocate a pseudo-TTY (for exec)
  --cwd <path>         Working directory inside container (for exec)

Examples:
  # Create a container from a bundle
  runc-go create mycontainer /path/to/bundle

  # Start the container
  runc-go start mycontainer

  # Or create and start in one step
  runc-go run mycontainer /path/to/bundle

  # Execute a command in a running container
  runc-go exec mycontainer /bin/sh

  # Check container state
  runc-go state mycontainer

  # Kill the container
  runc-go kill mycontainer SIGTERM

  # Delete the container
  runc-go delete mycontainer

  # Generate a default config.json
  runc-go spec > config.json
`)
}

func cmdCreate() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	bundle := args.pos(1)
	if bundle == "" {
		bundle = args.get("bundle")
	}
	if bundle == "" {
		bundle = "."
	}

	// Use global root if set, otherwise use flag or default
	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	c, err := container.New(containerID, bundle, root)
	if err != nil {
		return err
	}

	opts := &container.CreateOptions{
		PidFile:       args.get("pid-file"),
		ConsoleSocket: args.get("console-socket"),
	}

	return c.Create(opts)
}

func cmdStart() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	c, err := container.Load(containerID, root)
	if err != nil {
		return err
	}

	return c.Start()
}

func cmdRun() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	bundle := args.pos(1)
	if bundle == "" {
		bundle = args.get("bundle")
	}
	if bundle == "" {
		bundle = "."
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	c, err := container.New(containerID, bundle, root)
	if err != nil {
		return err
	}

	opts := &container.CreateOptions{
		PidFile:       args.get("pid-file"),
		ConsoleSocket: args.get("console-socket"),
	}

	if err := c.Run(opts); err != nil {
		return err
	}

	// Wait for container to exit
	code, err := c.Wait()
	if err != nil {
		return err
	}

	os.Exit(code)
	return nil
}

func cmdState() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	return container.State(containerID, root)
}

func cmdKill() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	sigStr := args.pos(1)
	if sigStr == "" {
		sigStr = "SIGTERM"
	}

	sig, err := container.ParseSignal(sigStr)
	if err != nil {
		return err
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	all := args.has("all") || args.has("a")
	return container.Kill(containerID, root, sig, all)
}

func cmdDelete() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	opts := &container.DeleteOptions{
		Force: args.has("force") || args.has("f"),
	}

	return container.Delete(containerID, root, opts)
}

func cmdList() error {
	args := parseArgs(os.Args[2:])

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	containers, err := container.List(root)
	if err != nil {
		return err
	}

	if args.has("quiet") || args.has("q") {
		for _, c := range containers {
			fmt.Println(c.ID)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPID\tSTATUS\tBUNDLE\tCREATED")

	for _, c := range containers {
		created := c.State.Created.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			c.ID, c.State.Pid, c.State.Status, c.Bundle, created)
	}

	w.Flush()
	return nil
}

func cmdSpec() error {
	args := parseArgs(os.Args[2:])

	s := spec.DefaultSpec()

	bundle := args.get("bundle")
	if bundle == "" {
		bundle = "."
	}

	if args.has("rootless") {
		// Add user namespace for rootless
		s.Linux.Namespaces = append(s.Linux.Namespaces, spec.LinuxNamespace{
			Type: spec.UserNamespace,
		})

		// Add UID/GID mappings
		uid := uint32(os.Getuid())
		gid := uint32(os.Getgid())
		s.Linux.UIDMappings = []spec.LinuxIDMapping{
			{ContainerID: 0, HostID: uid, Size: 1},
		}
		s.Linux.GIDMappings = []spec.LinuxIDMapping{
			{ContainerID: 0, HostID: gid, Size: 1},
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s)
}

func cmdInit() error {
	// This is called inside the container namespace
	return container.InitContainer()
}

func cmdExec() error {
	args := parseArgs(os.Args[2:])

	containerID := args.pos(0)
	if containerID == "" {
		return fmt.Errorf("container ID required")
	}

	root := globalRoot
	if root == "" {
		root = args.get("root")
	}

	opts := &container.ExecOptions{
		Tty:     args.has("tty") || args.has("t"),
		Cwd:     args.get("cwd"),
		Detach:  args.has("detach") || args.has("d"),
		PidFile: args.get("pid-file"),
	}

	// Parse environment variables
	if envVal := args.get("env"); envVal != "" {
		opts.Env = append(opts.Env, envVal)
	}

	// Check if --process flag is used (Docker/containerd style)
	processFile := args.get("process")
	if processFile != "" {
		return container.ExecWithProcessFile(containerID, root, processFile, opts)
	}

	// Get command to execute (remaining positional args after container ID)
	var execArgs []string
	for i := 1; ; i++ {
		arg := args.pos(i)
		if arg == "" {
			break
		}
		execArgs = append(execArgs, arg)
	}

	if len(execArgs) == 0 {
		return fmt.Errorf("command required")
	}

	return container.Exec(containerID, root, execArgs, opts)
}

func cmdExecInit() error {
	// This is called to join container namespaces and exec
	return container.ExecInit()
}

// args is a simple argument parser
type cliArgs struct {
	flags map[string]string
	positional []string
}

func parseArgs(args []string) *cliArgs {
	a := &cliArgs{
		flags: make(map[string]string),
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if len(arg) == 0 {
			i++
			continue
		}

		if arg[0] == '-' {
			// Flag
			key := arg
			for len(key) > 0 && key[0] == '-' {
				key = key[1:]
			}

			// Check for = in flag
			for j := 0; j < len(key); j++ {
				if key[j] == '=' {
					a.flags[key[:j]] = key[j+1:]
					key = ""
					break
				}
			}

			if key != "" {
				// Check if next arg is value
				if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
					a.flags[key] = args[i+1]
					i++
				} else {
					a.flags[key] = "true"
				}
			}
		} else {
			a.positional = append(a.positional, arg)
		}

		i++
	}

	return a
}

func (a *cliArgs) get(key string) string {
	if v, ok := a.flags[key]; ok {
		return v
	}
	return ""
}

func (a *cliArgs) has(key string) bool {
	_, ok := a.flags[key]
	return ok
}

func (a *cliArgs) pos(i int) string {
	if i < len(a.positional) {
		return a.positional[i]
	}
	return ""
}

// Helper for generating OCI bundle from container images
// This is not part of the OCI runtime spec but useful for testing
func cmdPrepare() error {
	args := parseArgs(os.Args[2:])

	rootfs := args.pos(0)
	if rootfs == "" {
		return fmt.Errorf("rootfs path required")
	}

	// Create rootfs directory structure
	dirs := []string{
		"bin", "sbin", "lib", "lib64",
		"usr/bin", "usr/sbin", "usr/lib",
		"etc", "var", "tmp", "root", "home",
		"proc", "sys", "dev", "dev/pts", "dev/shm",
	}

	for _, dir := range dirs {
		path := filepath.Join(rootfs, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Create minimal /etc files
	etc := filepath.Join(rootfs, "etc")
	os.WriteFile(filepath.Join(etc, "hostname"), []byte("container\n"), 0644)
	os.WriteFile(filepath.Join(etc, "hosts"), []byte("127.0.0.1 localhost\n"), 0644)
	os.WriteFile(filepath.Join(etc, "resolv.conf"), []byte("nameserver 8.8.8.8\n"), 0644)

	fmt.Printf("Prepared rootfs at %s\n", rootfs)
	fmt.Println("Copy binaries and libraries to complete the rootfs")

	return nil
}

// Ignore unused syscall import warning
var _ = syscall.SIGTERM
