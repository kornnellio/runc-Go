// Package container implements the delete operation.
package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"runc-go/linux"
	"runc-go/spec"
)

// DeleteOptions contains options for container deletion.
type DeleteOptions struct {
	// Force kills the container if it's running.
	Force bool
}

// Delete removes a container.
func Delete(ctx context.Context, id, stateRoot string, opts *DeleteOptions) error {
	if opts == nil {
		opts = &DeleteOptions{}
	}

	c, err := Load(ctx, id, stateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("load container: %w", err)
	}

	// Refresh status
	c.RefreshStatus()

	// Check if running
	if c.IsRunning() {
		if !opts.Force {
			return fmt.Errorf("container is running, use --force to kill it")
		}

		// Force kill
		if err := c.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("kill container: %w", err)
		}

		// Wait for process to exit
		waitForExit(ctx, c.InitProcess, 5*time.Second)
	}

	// Clean up cgroup
	cgroupPath := linux.GetCgroupPath(c.ID, "")
	if c.CgroupPath != "" {
		cgroupPath = c.CgroupPath
	}
	cgroup, err := linux.NewCgroup(cgroupPath)
	if err == nil {
		cgroup.Destroy()
	}

	// Remove exec FIFO if it exists
	os.Remove(c.ExecFifoPath())

	// Remove state directory
	if err := os.RemoveAll(c.StateDir); err != nil {
		return fmt.Errorf("remove state dir: %w", err)
	}

	return nil
}

// waitForExit waits for a process to exit with a timeout.
func waitForExit(ctx context.Context, pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := syscall.Kill(pid, 0)
		if err != nil {
			return // Process exited
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Cleanup removes all state for containers that are no longer running.
func Cleanup(ctx context.Context, stateRoot string) error {
	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		c, err := Load(ctx, entry.Name(), stateRoot)
		if err != nil {
			// Remove invalid state
			os.RemoveAll(filepath.Join(stateRoot, entry.Name()))
			continue
		}

		c.RefreshStatus()
		if c.State.Status == spec.StatusStopped {
			Delete(ctx, c.ID, stateRoot, &DeleteOptions{Force: true})
		}
	}

	return nil
}
