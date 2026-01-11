// Package container implements the start operation.
package container

import (
	"fmt"
	"os"
	"syscall"

	"runc-go/spec"
)

// Start starts a created container by signaling the init process to exec.
func (c *Container) Start() error {
	// Verify container is in created state
	c.RefreshStatus()
	if c.State.Status != spec.StatusCreated {
		return fmt.Errorf("container is not in created state (current: %s)", c.State.Status)
	}

	// Open FIFO for writing - this signals the init process to continue
	fifoPath := c.ExecFifoPath()
	fifo, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open fifo: %w", err)
	}

	// Write to FIFO to unblock the init process
	_, err = fifo.Write([]byte{0})
	fifo.Close()

	if err != nil {
		return fmt.Errorf("write fifo: %w", err)
	}

	// Remove FIFO - it's no longer needed
	os.Remove(fifoPath)

	// Update state to running
	c.State.Status = spec.StatusRunning
	if err := c.SaveState(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}

// Run creates and starts a container in one operation.
func (c *Container) Run(opts *CreateOptions) error {
	// Create the container
	if err := c.Create(opts); err != nil {
		return err
	}

	// Start the container
	return c.Start()
}

// Wait waits for the container process to exit and returns the exit code.
func (c *Container) Wait() (int, error) {
	if c.InitProcess <= 0 {
		return -1, fmt.Errorf("no init process")
	}

	// Wait for the process
	var wstatus syscall.WaitStatus
	_, err := syscall.Wait4(c.InitProcess, &wstatus, 0, nil)
	if err != nil {
		return -1, fmt.Errorf("wait4: %w", err)
	}

	// Update state
	c.State.Status = spec.StatusStopped
	c.SaveState()

	// Return exit code
	if wstatus.Exited() {
		return wstatus.ExitStatus(), nil
	}
	if wstatus.Signaled() {
		return 128 + int(wstatus.Signal()), nil
	}

	return -1, nil
}
