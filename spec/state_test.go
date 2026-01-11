package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContainerStateSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	state := &ContainerState{
		State: State{
			Version: Version,
			ID:      "test-container",
			Status:  StatusRunning,
			Pid:     12345,
			Bundle:  "/path/to/bundle",
			Annotations: map[string]string{
				"key": "value",
			},
		},
		Created: time.Now(),
		Rootfs:  "/path/to/rootfs",
		Owner:   "root",
	}

	statePath := filepath.Join(tmpDir, "state.json")
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify content
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var loaded ContainerState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal state: %v", err)
	}

	if loaded.ID != state.ID {
		t.Errorf("ID mismatch: expected %s, got %s", state.ID, loaded.ID)
	}

	if loaded.Status != state.Status {
		t.Errorf("status mismatch: expected %s, got %s", state.Status, loaded.Status)
	}

	if loaded.Pid != state.Pid {
		t.Errorf("PID mismatch: expected %d, got %d", state.Pid, loaded.Pid)
	}
}

func TestLoadState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	state := &ContainerState{
		State: State{
			Version: Version,
			ID:      "load-test",
			Status:  StatusCreated,
			Pid:     9999,
			Bundle:  "/bundle",
		},
		Created: time.Now().Truncate(time.Second), // Truncate for comparison
		Rootfs:  "/rootfs",
	}

	statePath := filepath.Join(tmpDir, "state.json")
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if loaded.ID != state.ID {
		t.Errorf("ID mismatch")
	}

	if loaded.Status != state.Status {
		t.Errorf("status mismatch")
	}

	if loaded.Pid != state.Pid {
		t.Errorf("PID mismatch")
	}

	if loaded.Bundle != state.Bundle {
		t.Errorf("bundle mismatch")
	}

	if loaded.Rootfs != state.Rootfs {
		t.Errorf("rootfs mismatch")
	}
}

func TestLoadStateNotFound(t *testing.T) {
	_, err := LoadState("/nonexistent/state.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadStateInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "state.json")
	if err := os.WriteFile(statePath, []byte("not json"), 0600); err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	_, err = LoadState(statePath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestToOCIState(t *testing.T) {
	containerState := &ContainerState{
		State: State{
			Version: Version,
			ID:      "oci-test",
			Status:  StatusRunning,
			Pid:     5555,
			Bundle:  "/bundle",
			Annotations: map[string]string{
				"org.example.key": "value",
			},
		},
		Created: time.Now(),
		Rootfs:  "/rootfs",
		Owner:   "testuser",
	}

	ociState := containerState.ToOCIState()

	// Verify only OCI fields are present
	if ociState.Version != containerState.Version {
		t.Errorf("version mismatch")
	}

	if ociState.ID != containerState.ID {
		t.Errorf("ID mismatch")
	}

	if ociState.Status != containerState.Status {
		t.Errorf("status mismatch")
	}

	if ociState.Pid != containerState.Pid {
		t.Errorf("PID mismatch")
	}

	if ociState.Bundle != containerState.Bundle {
		t.Errorf("bundle mismatch")
	}

	if ociState.Annotations["org.example.key"] != "value" {
		t.Errorf("annotations mismatch")
	}
}

func TestStateJSONFormat(t *testing.T) {
	state := &State{
		Version: Version,
		ID:      "json-format-test",
		Status:  StatusRunning,
		Pid:     1234,
		Bundle:  "/path/to/bundle",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Check JSON field names
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	expectedFields := []string{"ociVersion", "id", "status", "pid", "bundle"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing expected field: %s", field)
		}
	}

	// Verify ociVersion is used instead of version
	if _, ok := raw["version"]; ok {
		t.Error("should use 'ociVersion' not 'version'")
	}
}

func TestContainerStateWithConfig(t *testing.T) {
	spec := DefaultSpec()
	state := &ContainerState{
		State: State{
			Version: Version,
			ID:      "with-config",
			Status:  StatusCreated,
			Bundle:  "/bundle",
		},
		Config: spec,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var loaded ContainerState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if loaded.Config == nil {
		t.Fatal("config was not preserved")
	}

	if loaded.Config.Version != spec.Version {
		t.Errorf("config version mismatch")
	}
}

func TestContainerStatusValues(t *testing.T) {
	tests := []struct {
		status   ContainerStatus
		expected string
	}{
		{StatusCreating, "creating"},
		{StatusCreated, "created"},
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
	}

	for _, tc := range tests {
		if string(tc.status) != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.status)
		}
	}
}

func TestStateEmptyAnnotations(t *testing.T) {
	state := &State{
		Version: Version,
		ID:      "no-annotations",
		Status:  StatusCreated,
		Bundle:  "/bundle",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Annotations should be omitted if empty
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := raw["annotations"]; ok {
		t.Error("empty annotations should be omitted")
	}
}

func TestStatePidOmitEmpty(t *testing.T) {
	state := &State{
		Version: Version,
		ID:      "no-pid",
		Status:  StatusCreated,
		Pid:     0,
		Bundle:  "/bundle",
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Pid 0 should be omitted
	if _, ok := raw["pid"]; ok {
		t.Error("pid 0 should be omitted")
	}
}
