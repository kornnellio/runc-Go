// Package container implements OCI container lifecycle management.
package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"runc-go/spec"
)

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
func Load(id string, stateRoot string) (*Container, error) {
	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	stateDir := filepath.Join(stateRoot, id)
	statePath := filepath.Join(stateDir, StateFileName)

	state, err := spec.LoadState(statePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	c := &Container{
		ID:          id,
		Bundle:      state.Bundle,
		StateDir:    stateDir,
		State:       state,
		InitProcess: state.Pid,
	}

	// Load spec if available
	specPath := filepath.Join(state.Bundle, "config.json")
	c.Spec, _ = spec.LoadSpec(specPath)

	return c, nil
}

// New creates a new container instance (doesn't start it yet).
func New(id, bundle, stateRoot string) (*Container, error) {
	if stateRoot == "" {
		stateRoot = DefaultStateDir
	}

	// Validate bundle
	bundle, err := filepath.Abs(bundle)
	if err != nil {
		return nil, fmt.Errorf("abs bundle path: %w", err)
	}

	// Load OCI spec
	specPath := filepath.Join(bundle, "config.json")
	s, err := spec.LoadSpec(specPath)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}

	// Create state directory
	stateDir := filepath.Join(stateRoot, id)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	// Check if container already exists
	statePath := filepath.Join(stateDir, StateFileName)
	if _, err := os.Stat(statePath); err == nil {
		return nil, fmt.Errorf("container %s already exists", id)
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
func (c *Container) SaveState() error {
	statePath := filepath.Join(c.StateDir, StateFileName)
	return c.State.Save(statePath)
}

// GetState returns the OCI-compliant state.
func (c *Container) GetState() *spec.State {
	// Update PID from actual process if running
	if c.State.Status == spec.StatusRunning {
		c.State.Pid = c.InitProcess
	}
	return c.State.ToOCIState()
}

// UpdateStatus updates the container status.
func (c *Container) UpdateStatus(status spec.ContainerStatus) error {
	c.State.Status = status
	return c.SaveState()
}

// IsRunning checks if the container process is still running.
func (c *Container) IsRunning() bool {
	if c.InitProcess <= 0 {
		return false
	}

	// Check if process exists by sending signal 0
	err := syscall.Kill(c.InitProcess, 0)
	return err == nil
}

// RefreshStatus updates status based on actual process state.
func (c *Container) RefreshStatus() {
	switch c.State.Status {
	case spec.StatusRunning:
		if !c.IsRunning() {
			c.State.Status = spec.StatusStopped
		}
	case spec.StatusCreated:
		if !c.IsRunning() {
			c.State.Status = spec.StatusStopped
		}
	}
}

// Destroy removes all container state and resources.
func (c *Container) Destroy() error {
	// Remove state directory
	return os.RemoveAll(c.StateDir)
}

// ExecFifoPath returns the path to the exec FIFO.
func (c *Container) ExecFifoPath() string {
	return filepath.Join(c.StateDir, ExecFifoName)
}

// CreateExecFifo creates the FIFO used for create/start synchronization.
func (c *Container) CreateExecFifo() error {
	fifoPath := c.ExecFifoPath()
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return fmt.Errorf("mkfifo: %w", err)
	}
	return nil
}

// List returns all containers in the state directory.
func List(stateRoot string) ([]*Container, error) {
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
		if !entry.IsDir() {
			continue
		}

		c, err := Load(entry.Name(), stateRoot)
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
func (c *Container) StateJSON() ([]byte, error) {
	c.RefreshStatus()
	return json.MarshalIndent(c.GetState(), "", "  ")
}

// Signal sends a signal to the container's init process.
func (c *Container) Signal(sig syscall.Signal) error {
	if c.InitProcess <= 0 {
		return fmt.Errorf("no init process")
	}
	return syscall.Kill(c.InitProcess, sig)
}

// SignalAll sends a signal to all processes in the container.
func (c *Container) SignalAll(sig syscall.Signal) error {
	// Send to process group
	if c.InitProcess <= 0 {
		return fmt.Errorf("no init process")
	}
	return syscall.Kill(-c.InitProcess, sig)
}
