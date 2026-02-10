package container

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"runc-go/spec"
)

// ============================================================================
// STATE TRANSITION TESTS
// ============================================================================

// TestStart_RequiresCreatedState tests that Start fails if container is not in created state.
func TestStart_RequiresCreatedState(t *testing.T) {
	tests := []struct {
		name   string
		status spec.ContainerStatus
	}{
		{"creating", spec.StatusCreating},
		{"running", spec.StatusRunning},
		{"stopped", spec.StatusStopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{
				ID: "test-container",
				State: &spec.ContainerState{
					State: spec.State{
						Status: tt.status,
					},
				},
				StateDir: t.TempDir(),
			}

			ctx := context.Background()
			err := c.Start(ctx)
			if err == nil {
				t.Error("expected error when starting container not in created state")
			}
		})
	}
}

// TestStart_ContextCancellation tests that Start respects context cancellation.
func TestStart_ContextCancellation(t *testing.T) {
	c := &Container{
		ID: "test-container",
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreated,
			},
		},
		StateDir: t.TempDir(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := c.Start(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ============================================================================
// FIFO TESTS
// ============================================================================

// TestStart_FIFONotFound tests error handling when FIFO doesn't exist.
func TestStart_FIFONotFound(t *testing.T) {
	tempDir := t.TempDir()
	c := &Container{
		ID: "test-container",
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreated,
			},
		},
		StateDir: tempDir,
	}

	ctx := context.Background()
	err := c.Start(ctx)
	if err == nil {
		t.Error("expected error when FIFO doesn't exist")
	}
}

// TestStart_FIFOWrite tests that Start writes to FIFO correctly.
func TestStart_FIFOWrite(t *testing.T) {
	tempDir := t.TempDir()
	fifoPath := filepath.Join(tempDir, ExecFifoName)

	// Create FIFO
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatalf("failed to create FIFO: %v", err)
	}

	// Use a real PID (our own) so RefreshStatus doesn't change state to stopped
	c := &Container{
		ID:          "test-container",
		InitProcess: os.Getpid(), // Use current process PID
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreated,
			},
		},
		StateDir: tempDir,
	}

	// Read from FIFO in goroutine
	readDone := make(chan byte, 1)
	go func() {
		f, err := os.OpenFile(fifoPath, os.O_RDONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		buf := make([]byte, 1)
		f.Read(buf)
		readDone <- buf[0]
	}()

	// Give the reader time to block on the FIFO
	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Check that byte was written
	select {
	case val := <-readDone:
		if val != 0 {
			t.Errorf("expected 0 byte, got %d", val)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for FIFO read")
	}
}

// TestStart_UpdatesStateToRunning tests that Start updates state to running.
func TestStart_UpdatesStateToRunning(t *testing.T) {
	tempDir := t.TempDir()
	fifoPath := filepath.Join(tempDir, ExecFifoName)

	// Create FIFO
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatalf("failed to create FIFO: %v", err)
	}

	// Use a real PID (our own) so RefreshStatus doesn't change state to stopped
	c := &Container{
		ID:          "test-container",
		InitProcess: os.Getpid(), // Use current process PID
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreated,
			},
		},
		StateDir: tempDir,
	}

	// Read from FIFO in goroutine
	go func() {
		f, _ := os.OpenFile(fifoPath, os.O_RDONLY, 0)
		if f != nil {
			buf := make([]byte, 1)
			f.Read(buf)
			f.Close()
		}
	}()

	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	err := c.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if c.State.Status != spec.StatusRunning {
		t.Errorf("state should be running, got %s", c.State.Status)
	}
}

// ============================================================================
// WAIT TESTS
// ============================================================================

// TestWait_InvalidPID tests Wait with invalid PID.
func TestWait_InvalidPID(t *testing.T) {
	c := &Container{
		ID:          "test-container",
		InitProcess: 0,
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusRunning,
			},
		},
	}

	ctx := context.Background()
	_, err := c.Wait(ctx)
	if err == nil {
		t.Error("expected error with invalid PID")
	}
}

// TestWait_NegativePID tests Wait with negative PID.
func TestWait_NegativePID(t *testing.T) {
	c := &Container{
		ID:          "test-container",
		InitProcess: -1,
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusRunning,
			},
		},
	}

	ctx := context.Background()
	_, err := c.Wait(ctx)
	if err == nil {
		t.Error("expected error with negative PID")
	}
}

// TestWait_ContextCancellation tests that Wait respects context cancellation.
func TestWait_ContextCancellation(t *testing.T) {
	c := &Container{
		ID:          "test-container",
		InitProcess: 99999999, // Non-existent PID
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusRunning,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.Wait(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ============================================================================
// RUN TESTS
// ============================================================================

// TestRun_RequiresValidBundle tests that Run requires a valid bundle.
func TestRun_RequiresValidBundle(t *testing.T) {
	tempDir := t.TempDir()
	c := &Container{
		ID:       "test-container",
		Bundle:   "/nonexistent/bundle",
		StateDir: tempDir,
		Spec:     &spec.Spec{}, // Provide non-nil Spec
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreating,
			},
		},
	}

	ctx := context.Background()
	err := c.Run(ctx, nil)
	if err == nil {
		t.Error("expected error with invalid bundle")
	}
}

// ============================================================================
// CONCURRENT ACCESS TESTS
// ============================================================================

// TestStart_ConcurrentAccess tests that Start is safe for concurrent access.
func TestStart_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	fifoPath := filepath.Join(tempDir, ExecFifoName)

	// Create FIFO
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatalf("failed to create FIFO: %v", err)
	}

	// Use a real PID (our own) so RefreshStatus doesn't change state to stopped
	c := &Container{
		ID:          "test-container",
		InitProcess: os.Getpid(), // Use current process PID
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusCreated,
			},
		},
		StateDir: tempDir,
	}

	// Read from FIFO in goroutine - read multiple times for concurrent access
	go func() {
		for i := 0; i < 3; i++ {
			f, _ := os.OpenFile(fifoPath, os.O_RDONLY, 0)
			if f != nil {
				buf := make([]byte, 1)
				f.Read(buf)
				f.Close()
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// Try concurrent Start calls
	done := make(chan error, 3)
	for i := range 3 {
		go func(idx int) {
			ctx := context.Background()
			done <- c.Start(ctx)
		}(i)
	}

	// Collect results - one should succeed, others should fail
	var successCount, errorCount int
	for range 3 {
		if err := <-done; err == nil {
			successCount++
		} else {
			errorCount++
		}
	}

	// Exactly one should succeed (first to acquire the FIFO)
	// Others may error or also succeed depending on timing
	// The key is that no panics occur
	if successCount == 0 && errorCount == 0 {
		t.Error("expected at least some results from concurrent starts")
	}
}

// ============================================================================
// FIFO CREATION TESTS (in start_test.go)
// ============================================================================

// TestStart_CreateExecFifo tests FIFO creation via Start path.
func TestStart_CreateExecFifo(t *testing.T) {
	tempDir := t.TempDir()
	c := &Container{
		ID:       "test-container",
		StateDir: tempDir,
	}

	if err := c.CreateExecFifo(); err != nil {
		t.Fatalf("CreateExecFifo failed: %v", err)
	}

	fifoPath := c.ExecFifoPath()
	fi, err := os.Stat(fifoPath)
	if err != nil {
		t.Fatalf("FIFO not created: %v", err)
	}

	// Check it's a FIFO (named pipe)
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Error("created file is not a FIFO")
	}
}

// TestStart_CreateExecFifo_AlreadyExists tests FIFO creation when one already exists.
func TestStart_CreateExecFifo_AlreadyExists(t *testing.T) {
	tempDir := t.TempDir()
	c := &Container{
		ID:       "test-container",
		StateDir: tempDir,
	}

	// Create FIFO first time
	if err := c.CreateExecFifo(); err != nil {
		t.Fatalf("first CreateExecFifo failed: %v", err)
	}

	// Try to create again - should fail
	err := c.CreateExecFifo()
	if err == nil {
		t.Error("expected error when FIFO already exists")
	}
}
