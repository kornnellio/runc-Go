// Package container implements the create operation.
package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	cerrors "runc-go/errors"
	"runc-go/linux"
	"runc-go/spec"
	"runc-go/utils"
)

// CreateOptions contains options for container creation.
type CreateOptions struct {
	// ConsoleSocket is the path to a unix socket for the console.
	ConsoleSocket string

	// PidFile is the path to write the container PID.
	PidFile string

	// NoPivot disables pivot_root (use chroot instead).
	NoPivot bool

	// NoNewKeyring disables creating a new session keyring.
	NoNewKeyring bool
}

// Create creates a container but doesn't start the user process.
// The container will be in "created" state, waiting for Start().
func (c *Container) Create(ctx context.Context, opts *CreateOptions) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if opts == nil {
		opts = &CreateOptions{}
	}

	// Create exec FIFO for synchronization
	if err := c.CreateExecFifo(); err != nil {
		return cerrors.Wrap(err, cerrors.ErrResource, "create exec fifo")
	}

	// Cleanup function to call on error after FIFO is created
	var cgroup *linux.Cgroup
	cleanup := func() {
		// Remove FIFO
		os.Remove(c.ExecFifoPath())
		// Destroy cgroup if created
		if cgroup != nil {
			cgroup.Destroy()
		}
	}

	// Setup cgroup
	cgroupPath := linux.GetCgroupPath(c.ID, "")
	if c.Spec.Linux != nil && c.Spec.Linux.CgroupsPath != "" {
		cgroupPath = c.Spec.Linux.CgroupsPath
	}
	c.CgroupPath = cgroupPath

	// Enable parent controllers
	linux.EnsureParentControllers(cgroupPath)

	// Create cgroup
	var err error
	cgroup, err = linux.NewCgroup(cgroupPath)
	if err != nil {
		cleanup()
		return fmt.Errorf("create cgroup: %w", err)
	}

	// Apply resource limits
	if c.Spec.Linux != nil && c.Spec.Linux.Resources != nil {
		if err := cgroup.ApplyResources(c.Spec.Linux.Resources); err != nil {
			cleanup()
			return fmt.Errorf("apply resources: %w", err)
		}
	}

	// Get path to our own executable
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}

	// Build command for init process
	// We re-exec ourselves with "init" command
	cmd := exec.Command(self, "init")
	cmd.Dir = c.Bundle

	// Setup namespace flags
	sysProcAttr, err := linux.BuildSysProcAttr(c.Spec)
	if err != nil {
		return fmt.Errorf("build sysprocattr: %w", err)
	}
	cmd.SysProcAttr = sysProcAttr

	// Setup environment for init
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("_RUNC_GO_INIT_BUNDLE=%s", c.Bundle),
		fmt.Sprintf("_RUNC_GO_INIT_FIFO=%s", c.ExecFifoPath()),
		fmt.Sprintf("_RUNC_GO_INIT_ID=%s", c.ID),
		fmt.Sprintf("_RUNC_GO_STATE_DIR=%s", c.StateDir),
	)

	// Setup stdin/stdout/stderr
	var console *utils.Console
	var consoleSlave *os.File
	if c.Spec.Process != nil && c.Spec.Process.Terminal && opts.ConsoleSocket != "" {
		// Console socket mode: create PTY and send master to socket
		var err error
		console, err = utils.NewConsole()
		if err != nil {
			return fmt.Errorf("create console: %w", err)
		}
		// Open slave PTY in parent and pass to child via inheritance
		consoleSlave, err = console.OpenSlave()
		if err != nil {
			console.Close()
			return fmt.Errorf("open console slave: %w", err)
		}
		// Connect child's stdio to slave PTY
		cmd.Stdin = consoleSlave
		cmd.Stdout = consoleSlave
		cmd.Stderr = consoleSlave
		// Note: Don't set Setctty here - it interferes with namespace creation
		// The controlling terminal is set up in InitContainer instead
	} else if c.Spec.Process != nil && c.Spec.Process.Terminal {
		// Direct terminal mode: inherit from parent
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		// Non-terminal mode
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Start the init process
	if err := cmd.Start(); err != nil {
		if console != nil {
			console.Close()
		}
		cleanup()
		return fmt.Errorf("start init: %w", err)
	}

	// Send PTY master to console socket (must be after cmd.Start)
	if console != nil {
		if err := utils.SendConsoleToSocket(opts.ConsoleSocket, console.Master()); err != nil {
			cmd.Process.Kill()
			console.Close()
			if consoleSlave != nil {
				consoleSlave.Close()
			}
			cleanup()
			return fmt.Errorf("send console to socket: %w", err)
		}
		console.Close() // Parent doesn't need master anymore
		if consoleSlave != nil {
			consoleSlave.Close() // Parent doesn't need slave anymore
		}
	}

	c.InitProcess = cmd.Process.Pid
	c.State.Pid = c.InitProcess

	// Add process to cgroup
	if err := cgroup.AddProcess(c.InitProcess); err != nil {
		cmd.Process.Kill()
		cleanup()
		return fmt.Errorf("add to cgroup: %w", err)
	}

	// Write PID file if requested
	if opts.PidFile != "" {
		if err := os.WriteFile(opts.PidFile, []byte(fmt.Sprintf("%d", c.InitProcess)), 0644); err != nil {
			cmd.Process.Kill()
			cleanup()
			return fmt.Errorf("write pid file: %w", err)
		}
	}

	// Update state to created
	c.State.Status = spec.StatusCreated
	if err := c.SaveState(); err != nil {
		cmd.Process.Kill()
		cleanup()
		return fmt.Errorf("save state: %w", err)
	}

	// Don't wait for cmd - the init process will block on the FIFO
	// waiting for Start() to be called

	return nil
}

// InitContainer is called inside the container namespace to complete setup.
// This is executed by the re-exec'd process.
func InitContainer() error {
	// Get init parameters from environment
	bundle := os.Getenv("_RUNC_GO_INIT_BUNDLE")
	fifoPath := os.Getenv("_RUNC_GO_INIT_FIFO")
	// containerID := os.Getenv("_RUNC_GO_INIT_ID")
	// stateDir := os.Getenv("_RUNC_GO_STATE_DIR")

	if bundle == "" || fifoPath == "" {
		return fmt.Errorf("missing init environment")
	}

	// Load spec
	specPath := filepath.Join(bundle, "config.json")
	s, err := spec.LoadSpec(specPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	// Join namespaces if paths specified
	if s.Linux != nil {
		if err := linux.SetNamespaces(s.Linux.Namespaces); err != nil {
			return fmt.Errorf("set namespaces: %w", err)
		}
	}

	// Set hostname
	if s.Hostname != "" {
		if err := linux.SetHostname(s.Hostname); err != nil {
			return fmt.Errorf("set hostname: %w", err)
		}
	}

	// Set domainname
	if s.Domainname != "" {
		if err := linux.SetDomainname(s.Domainname); err != nil {
			return fmt.Errorf("set domainname: %w", err)
		}
	}

	// IMPORTANT: Open FIFO BEFORE pivot_root, as it won't be accessible after
	fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open fifo: %w", err)
	}

	// Setup rootfs (pivot_root, mounts, etc.)
	if err := linux.SetupRootfs(s, bundle); err != nil {
		fifo.Close()
		return fmt.Errorf("setup rootfs: %w", err)
	}

	// Setup devices
	if s.Linux != nil && len(s.Linux.Devices) > 0 {
		if err := linux.CreateDevices(s.Linux.Devices); err != nil {
			fmt.Printf("[init] warning: create devices: %v\n", err)
		}
	}

	// Setup default devices
	linux.SetupDefaultDevices()
	linux.SetupDevSymlinks()
	linux.SetupDevPts()

	// Change to working directory
	if s.Process != nil && s.Process.Cwd != "" {
		if err := os.Chdir(s.Process.Cwd); err != nil {
			fifo.Close()
			return fmt.Errorf("chdir %s: %w", s.Process.Cwd, err)
		}
	}

	// Now wait on FIFO - this blocks until Start() is called
	// Read from FIFO (blocks until writer connects)
	buf := make([]byte, 1)
	_, err = fifo.Read(buf)
	fifo.Close()

	if err != nil {
		return fmt.Errorf("read fifo: %w", err)
	}

	// Create /dev/console if stdin is a PTY (character device)
	// Go's Setctty flag handles setsid() and TIOCSCTTY automatically
	var stat syscall.Stat_t
	if err := syscall.Fstat(0, &stat); err == nil {
		if stat.Mode&syscall.S_IFCHR != 0 {
			os.Remove("/dev/console")
			if err := syscall.Mknod("/dev/console", syscall.S_IFCHR|0600, int(stat.Rdev)); err != nil {
				fmt.Printf("[init] warning: failed to create /dev/console: %v\n", err)
			}
		}
	}

	// Apply capabilities
	if s.Process != nil && s.Process.Capabilities != nil {
		if err := linux.ApplyCapabilities(s.Process.Capabilities); err != nil {
			return fmt.Errorf("apply capabilities: %w", err)
		}
	}

	// Apply seccomp
	if s.Linux != nil && s.Linux.Seccomp != nil {
		if err := linux.SetupSeccomp(s.Linux.Seccomp); err != nil {
			return fmt.Errorf("setup seccomp: %w", err)
		}
	}

	// Set user
	if s.Process != nil {
		if err := setUser(s.Process.User); err != nil {
			return fmt.Errorf("set user: %w", err)
		}
	}

	// Setup environment
	if s.Process != nil {
		for _, env := range s.Process.Env {
			parts := splitEnv(env)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}

	// Exec the user process
	if s.Process == nil || len(s.Process.Args) == 0 {
		return fmt.Errorf("no process args specified")
	}

	// If stdin is a TTY, ensure it's the controlling terminal
	// This is needed because Go's Setctty doesn't work reliably with Cloneflags
	if s.Process.Terminal {
		// Try to become session leader (may already be one, which is fine)
		syscall.Setsid()
		// Set stdin as controlling terminal
		utils.SetControllingTerminal(os.Stdin)
		// Enable signal generation and set foreground process group
		utils.SetupTerminalSignals(os.Stdin)
	}

	args := s.Process.Args
	path, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("lookup %s: %w", args[0], err)
	}

	// Instead of exec'ing directly (which would make user command PID 1),
	// fork/exec and forward signals. PID 1 in Linux ignores signals without handlers.
	cmd := exec.Command(path, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	// Start the user process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start user process: %w", err)
	}

	// Forward signals to the child process
	// PID 1 in Linux ignores signals without handlers, so we must catch and forward them
	sigChan := make(chan os.Signal, 10)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	// Signal forwarding goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range sigChan {
			// Ignore errors - process may have exited
			_ = cmd.Process.Signal(sig)
		}
	}()

	// Wait for child to exit and propagate its exit code
	waitErr := cmd.Wait()

	// Stop signal forwarding and clean up
	signal.Stop(sigChan)
	close(sigChan)
	<-done // Wait for goroutine to finish

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return waitErr
	}
	os.Exit(0)
	return nil // unreachable
}

// splitEnv splits an environment variable string into key and value.
func splitEnv(env string) []string {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

// setUser sets the user ID and group ID.
func setUser(user spec.User) error {
	// Set supplementary groups
	if len(user.AdditionalGids) > 0 {
		gids := make([]int, len(user.AdditionalGids))
		for i, g := range user.AdditionalGids {
			gids[i] = int(g)
		}
		// setgroups might fail in user namespaces, log warning but don't fail
		if err := setGroups(gids); err != nil {
			fmt.Printf("[init] warning: setgroups failed (expected in user namespaces): %v\n", err)
		}
	}

	// Set GID first (must be before UID)
	if user.GID != 0 {
		if err := setGid(int(user.GID)); err != nil {
			return fmt.Errorf("setgid: %w", err)
		}
	}

	// Set UID
	if user.UID != 0 {
		if err := setUid(int(user.UID)); err != nil {
			return fmt.Errorf("setuid: %w", err)
		}
	}

	// Set umask
	if user.Umask != nil {
		oldMask := setUmask(int(*user.Umask))
		_ = oldMask // Ignore old mask
	}

	return nil
}
