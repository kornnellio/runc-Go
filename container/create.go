// Package container implements the create operation.
package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"runc-go/linux"
	"runc-go/spec"
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
func (c *Container) Create(opts *CreateOptions) error {
	if opts == nil {
		opts = &CreateOptions{}
	}

	// Create exec FIFO for synchronization
	if err := c.CreateExecFifo(); err != nil {
		return fmt.Errorf("create exec fifo: %w", err)
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
	cgroup, err := linux.NewCgroup(cgroupPath)
	if err != nil {
		return fmt.Errorf("create cgroup: %w", err)
	}

	// Apply resource limits
	if c.Spec.Linux != nil && c.Spec.Linux.Resources != nil {
		if err := cgroup.ApplyResources(c.Spec.Linux.Resources); err != nil {
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
	// For now, inherit from parent (todo: console socket)
	if c.Spec.Process != nil && c.Spec.Process.Terminal {
		// Terminal mode - we'd setup console socket here
		// For now, just use parent's terminal
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Start the init process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start init: %w", err)
	}

	c.InitProcess = cmd.Process.Pid
	c.State.Pid = c.InitProcess

	// Add process to cgroup
	if err := cgroup.AddProcess(c.InitProcess); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("add to cgroup: %w", err)
	}

	// Write PID file if requested
	if opts.PidFile != "" {
		if err := os.WriteFile(opts.PidFile, []byte(fmt.Sprintf("%d", c.InitProcess)), 0644); err != nil {
			cmd.Process.Kill()
			return fmt.Errorf("write pid file: %w", err)
		}
	}

	// Update state to created
	c.State.Status = spec.StatusCreated
	if err := c.SaveState(); err != nil {
		cmd.Process.Kill()
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

	args := s.Process.Args
	path, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("lookup %s: %w", args[0], err)
	}

	// Exec replaces this process with the user's command
	return execProcess(path, args, os.Environ())
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
		// Note: setgroups might fail in user namespaces
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
