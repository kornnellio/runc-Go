// Package container implements the start operation.
package container

import (
	"context"
	"fmt"
	"os"
	"syscall"

	cerrors "runc-go/errors"
	"runc-go/spec"
)

// Start starts a created container by signaling the init process to exec.
func (c *Container) Start(ctx context.Context) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Verify container is in created state (thread-safe)
	c.RefreshStatus()
	c.mu.RLock()
	currentStatus := c.State.Status
	c.mu.RUnlock()
	if currentStatus != spec.StatusCreated {
		return cerrors.WrapWithDetail(nil, cerrors.ErrInvalidState, "start",
			fmt.Sprintf("container is not in created state (current: %s)", currentStatus))
	}

	// Open FIFO for writing - this signals the init process to continue
	fifoPath := c.ExecFifoPath()
	fifo, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
	if err != nil {
		return cerrors.Wrap(err, cerrors.ErrResource, "open fifo")
	}

	// Write to FIFO to unblock the init process
	_, err = fifo.Write([]byte{0})
	fifo.Close()

	if err != nil {
		return cerrors.Wrap(err, cerrors.ErrResource, "write fifo")
	}

	// Remove FIFO - it's no longer needed
	// Log error but don't fail - FIFO removal is non-critical
	if rmErr := os.Remove(fifoPath); rmErr != nil && !os.IsNotExist(rmErr) {
		fmt.Printf("[start] warning: failed to remove fifo: %v\n", rmErr)
	}

	// Update state to running (thread-safe via UpdateStatus)
	if err := c.UpdateStatus(spec.StatusRunning); err != nil {
		return cerrors.Wrap(err, cerrors.ErrInternal, "save state")
	}

	return nil
}

// Run creates and starts a container in one operation.
func (c *Container) Run(ctx context.Context, opts *CreateOptions) error {
	// Create the container
	if err := c.Create(ctx, opts); err != nil {
		return err
	}

	// Start the container
	return c.Start(ctx)
}

// Wait waits for the container process to exit and returns the exit code.
func (c *Container) Wait(ctx context.Context) (int, error) {
	if c.InitProcess <= 0 {
		return -1, cerrors.WrapWithContainer(nil, cerrors.ErrInvalidState, "wait", c.ID)
	}

	// Wait for the process (with context cancellation check)
	waitCh := make(chan struct {
		wstatus syscall.WaitStatus
		err     error
	}, 1)

	go func() {
		var wstatus syscall.WaitStatus
		_, err := syscall.Wait4(c.InitProcess, &wstatus, 0, nil)
		waitCh <- struct {
			wstatus syscall.WaitStatus
			err     error
		}{wstatus, err}
	}()

	select {
	case <-ctx.Done():
		return -1, ctx.Err()
	case result := <-waitCh:
		if result.err != nil {
			return -1, cerrors.Wrap(result.err, cerrors.ErrInternal, "wait4")
		}

		// Update state
		c.State.Status = spec.StatusStopped
		if saveErr := c.SaveState(); saveErr != nil {
			// Log error but still return exit code - state save is non-critical for Wait()
			fmt.Printf("[wait] warning: failed to save state: %v\n", saveErr)
		}

		// Return exit code
		if result.wstatus.Exited() {
			return result.wstatus.ExitStatus(), nil
		}
		if result.wstatus.Signaled() {
			return 128 + int(result.wstatus.Signal()), nil
		}

		return -1, nil
	}
}
