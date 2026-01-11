// Package spec provides OCI state types.
package spec

import (
	"encoding/json"
	"os"
	"time"
)

// ContainerStatus is the running status of a container.
type ContainerStatus string

// Container statuses as defined by OCI Runtime Spec.
const (
	// StatusCreating indicates the container is being created.
	StatusCreating ContainerStatus = "creating"

	// StatusCreated indicates the container has been created but not started.
	StatusCreated ContainerStatus = "created"

	// StatusRunning indicates the container process has been started and is running.
	StatusRunning ContainerStatus = "running"

	// StatusStopped indicates the container process has exited.
	StatusStopped ContainerStatus = "stopped"
)

// State holds information about the runtime state of the container.
// This is the format returned by the "state" operation as per OCI spec.
type State struct {
	// Version is the OCI specification version used by the runtime.
	Version string `json:"ociVersion"`

	// ID is the container's ID.
	ID string `json:"id"`

	// Status is the runtime status of the container.
	Status ContainerStatus `json:"status"`

	// Pid is the ID of the container process (as seen by the host).
	// This is the pid of the init process in the container.
	Pid int `json:"pid,omitempty"`

	// Bundle is the absolute path to the container's bundle directory.
	Bundle string `json:"bundle"`

	// Annotations are key-value pairs associated with the container.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ContainerState extends State with additional internal runtime information.
// This is stored in the state directory and includes more details than
// what the OCI "state" command outputs.
type ContainerState struct {
	State

	// Created is the time the container was created.
	Created time.Time `json:"created"`

	// Rootfs is the absolute path to the root filesystem.
	Rootfs string `json:"rootfs"`

	// Owner is the user who created the container.
	Owner string `json:"owner,omitempty"`

	// Config holds the original spec (optional, for debugging/introspection).
	Config *Spec `json:"config,omitempty"`
}

// LoadState loads container state from a JSON file.
func LoadState(path string) (*ContainerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state ContainerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save writes the container state to a JSON file.
func (s *ContainerState) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ToOCIState returns just the OCI-compliant state portion.
func (s *ContainerState) ToOCIState() *State {
	return &s.State
}
