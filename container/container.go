// Package container implements OCI container lifecycle management.
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"

	cerrors "runc-go/errors"
	"runc-go/logging"
	"runc-go/spec"
)

// containerIDRegex defines valid container ID format.
// Must be alphanumeric with dashes/underscores, no path separators or special chars.
var containerIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateContainerID checks that a container ID is safe and valid.
func ValidateContainerID(id string) error {
	if id == "" {
		return cerrors.ErrEmptyContainerID
	}
	if len(id) > 1024 {
		return cerrors.WrapWithDetail(nil, cerrors.ErrInvalidConfig, "validate",
			fmt.Sprintf("container ID too long (max 1024 characters): %d", len(id)))
	}
	if !containerIDRegex.MatchString(id) {
		return cerrors.WrapWithDetail(nil, cerrors.ErrInvalidConfig, "validate",
			fmt.Sprintf("container ID %q contains invalid characters (must be alphanumeric with _.-)", id))
	}
	// Explicitly check for path traversal attempts
	if id == "." || id == ".." || filepath.Clean(id) != id {
		return cerrors.WrapWithDetail(cerrors.ErrPathTraversal, cerrors.ErrInvalidConfig, "validate",
			fmt.Sprintf("container ID %q contains path traversal", id))
	}
	return nil
}

const (
	// DefaultStateDir is the default directory for container state.
	DefaultStateDir = "/run/runc-go"

	// ExecFifoName is the name of the FIFO used for create/start synchronization.
	ExecFifoName = "exec.fifo"

	// StateFileName is the name of the state file.
	StateFileName = "state.json"
)

// Container represents an OCI container.
type Container struct {
	// mu protects concurrent access to container state.
	mu sync.RWMutex

	// ID is the unique identifier for the container.
	ID string

	// Bundle is the path to the container bundle.
	Bundle string

	// StateDir is the directory containing container state.
	StateDir string

	// Spec is the OCI runtime specification.
	Spec *spec.Spec

	// State is the current container state.
	State *spec.ContainerState

	// InitProcess is the PID of the container's init process.
	InitProcess int

	// Cgroup is the cgroup for the container.
	CgroupPath string
}

// Load loads an existing container by ID.
func Load(ctx context.Context, id string, stateRoot string) (*Container, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate container ID to prevent path traversal
	if err := ValidateContainerID(id); err != nil {
		return nil, err
	}

	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	stateDir := filepath.Join(stateRoot, id)
	statePath := filepath.Join(stateDir, StateFileName)

	state, err := spec.LoadState(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, cerrors.WrapWithContainer(err, cerrors.ErrNotFound, "load", id)
		}
		return nil, cerrors.WrapWithContainer(err, cerrors.ErrInternal, "load state", id)
	}

	c := &Container{
		ID:          id,
		Bundle:      state.Bundle,
		StateDir:    stateDir,
		State:       state,
		InitProcess: state.Pid,
	}

	// Load spec if available (non-fatal if missing)
	specPath := filepath.Join(state.Bundle, "config.json")
	loadedSpec, err := spec.LoadSpec(specPath)
	if err != nil {
		// Log warning but don't fail - spec may not be needed for all operations
		logging.WarnContext(ctx, "could not load spec", "container_id", id, "path", specPath, "error", err)
	}
	c.Spec = loadedSpec

	return c, nil
}

// New creates a new container instance (doesn't start it yet).
func New(ctx context.Context, id, bundle, stateRoot string) (*Container, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Validate container ID to prevent path traversal
	if err := ValidateContainerID(id); err != nil {
		return nil, err
	}

	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	// Validate bundle
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.ErrInvalidConfig, "abs bundle path")
	}

	// Load OCI spec
	specPath := filepath.Join(bundle, "config.json")
	s, err := spec.LoadSpec(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, cerrors.Wrap(err, cerrors.ErrInvalidConfig, "load spec")
		}
		return nil, cerrors.Wrap(err, cerrors.ErrInvalidConfig, "parse spec")
	}

	// Create state directory
	stateDir := filepath.Join(stateRoot, id)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, cerrors.Wrap(err, cerrors.ErrPermission, "create state dir")
	}

	// Check if container already exists
	statePath := filepath.Join(stateDir, StateFileName)
	if _, err := os.Stat(statePath); err == nil {
		return nil, cerrors.WrapWithContainer(nil, cerrors.ErrAlreadyExists, "create", id)
	}

	c := &Container{
		ID:       id,
		Bundle:   bundle,
		StateDir: stateDir,
		Spec:     s,
		State: &spec.ContainerState{
			State: spec.State{
				Version:     spec.Version,
				ID:          id,
				Status:      spec.StatusCreating,
				Bundle:      bundle,
				Annotations: s.Annotations,
			},
			Created: time.Now(),
		},
	}

	// Set rootfs path
	if s.Root != nil {
		rootfs := s.Root.Path
		if !filepath.IsAbs(rootfs) {
			rootfs = filepath.Join(bundle, rootfs)
		}
		c.State.Rootfs = rootfs
	}

	return c, nil
}

// SaveState saves the container state to disk.
// This method is thread-safe.
func (c *Container) SaveState() error {
	c.mu.RLock()
	statePath := filepath.Join(c.StateDir, StateFileName)
	// Make a copy of state for safe serialization outside the lock
	stateCopy := *c.State
	c.mu.RUnlock()
	return stateCopy.Save(statePath)
}

// GetState returns the OCI-compliant state.
// This method is thread-safe. Returns a deep copy so callers can safely serialize.
func (c *Container) GetState() *spec.State {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update PID from actual process if running
	if c.State.Status == spec.StatusRunning {
		c.State.Pid = c.InitProcess
	}
	state := c.State.ToOCIState()
	// Return a deep copy to avoid race during serialization
	stateCopy := *state
	// Deep copy the Annotations map
	if state.Annotations != nil {
		stateCopy.Annotations = make(map[string]string, len(state.Annotations))
		for k, v := range state.Annotations {
			stateCopy.Annotations[k] = v
		}
	}
	return &stateCopy
}

// UpdateStatus updates the container status.
// This method is thread-safe.
func (c *Container) UpdateStatus(status spec.ContainerStatus) error {
	c.mu.Lock()
	c.State.Status = status
	statePath := filepath.Join(c.StateDir, StateFileName)
	// Make a copy of state for safe serialization outside the lock
	stateCopy := *c.State
	c.mu.Unlock()
	return stateCopy.Save(statePath)
}

// IsRunning checks if the container process is still running.
// This method is thread-safe.
func (c *Container) IsRunning() bool {
	c.mu.RLock()
	pid := c.InitProcess
	c.mu.RUnlock()

	if pid <= 0 {
		return false
	}

	// Check if process exists by sending signal 0
	err := syscall.Kill(pid, 0)
	return err == nil
}

// RefreshStatus updates status based on actual process state.
// This method is thread-safe.
func (c *Container) RefreshStatus() {
	// Check if process is running first (uses its own lock)
	isRunning := c.IsRunning()

	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.State.Status {
	case spec.StatusRunning:
		if !isRunning {
			c.State.Status = spec.StatusStopped
		}
	case spec.StatusCreated:
		if !isRunning {
			c.State.Status = spec.StatusStopped
		}
	}
}

// Destroy removes all container state and resources.
// This method is thread-safe.
func (c *Container) Destroy() error {
	c.mu.RLock()
	stateDir := c.StateDir
	c.mu.RUnlock()

	// Remove state directory
	return os.RemoveAll(stateDir)
}

// ExecFifoPath returns the path to the exec FIFO.
func (c *Container) ExecFifoPath() string {
	return filepath.Join(c.StateDir, ExecFifoName)
}

// CreateExecFifo creates the FIFO used for create/start synchronization.
func (c *Container) CreateExecFifo() error {
	fifoPath := c.ExecFifoPath()
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return cerrors.WrapWithContainer(err, cerrors.ErrResource, "create exec fifo", c.ID)
	}
	return nil
}

// List returns all containers in the state directory.
func List(ctx context.Context, stateRoot string) ([]*Container, error) {
	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var containers []*Container
	for _, entry := range entries {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		c, err := Load(ctx, entry.Name(), stateRoot)
		if err != nil {
			continue // Skip invalid containers
		}

		// Refresh status
		c.RefreshStatus()
		containers = append(containers, c)
	}

	return containers, nil
}

// StateJSON returns the container state as JSON.
// This method is thread-safe.
func (c *Container) StateJSON() ([]byte, error) {
	c.RefreshStatus()
	return json.MarshalIndent(c.GetState(), "", "  ")
}

// Signal sends a signal to the container's init process.
// This method is thread-safe.
func (c *Container) Signal(sig syscall.Signal) error {
	c.mu.RLock()
	pid := c.InitProcess
	id := c.ID
	c.mu.RUnlock()

	if pid <= 0 {
		return cerrors.WrapWithContainer(nil, cerrors.ErrInvalidState, "signal", id)
	}
	if err := syscall.Kill(pid, sig); err != nil {
		return cerrors.WrapWithContainer(err, cerrors.ErrInternal, "signal", id)
	}
	return nil
}

// SignalAll sends a signal to all processes in the container.
// This method is thread-safe.
func (c *Container) SignalAll(sig syscall.Signal) error {
	c.mu.RLock()
	pid := c.InitProcess
	id := c.ID
	c.mu.RUnlock()

	// Send to process group
	if pid <= 0 {
		return cerrors.WrapWithContainer(nil, cerrors.ErrInvalidState, "signal all", id)
	}
	if err := syscall.Kill(-pid, sig); err != nil {
		return cerrors.WrapWithContainer(err, cerrors.ErrInternal, "signal all", id)
	}
	return nil
}
