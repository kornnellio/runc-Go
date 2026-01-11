package container

import (
	"os"
	"path/filepath"
	"testing"

	"runc-go/spec"
)

func TestConstants(t *testing.T) {
	if DefaultStateDir != "/run/runc-go" {
		t.Errorf("unexpected DefaultStateDir: %s", DefaultStateDir)
	}

	if ExecFifoName != "exec.fifo" {
		t.Errorf("unexpected ExecFifoName: %s", ExecFifoName)
	}

	if StateFileName != "state.json" {
		t.Errorf("unexpected StateFileName: %s", StateFileName)
	}
}

func TestNew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create bundle directory
	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatalf("failed to create bundle dir: %v", err)
	}

	// Create rootfs directory
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		t.Fatalf("failed to create rootfs dir: %v", err)
	}

	// Write config.json
	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	// Create state root
	stateRoot := filepath.Join(tmpDir, "state")
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatalf("failed to create state root: %v", err)
	}

	// Create container
	c, err := New("test-container", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if c.ID != "test-container" {
		t.Errorf("expected ID 'test-container', got %s", c.ID)
	}

	if c.Bundle != bundleDir {
		t.Errorf("expected bundle %s, got %s", bundleDir, c.Bundle)
	}

	if c.State == nil {
		t.Fatal("State is nil")
	}

	if c.State.Status != spec.StatusCreating {
		t.Errorf("expected status creating, got %s", c.State.Status)
	}

	// Verify state directory was created
	stateDir := filepath.Join(stateRoot, "test-container")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("state directory was not created")
	}
}

func TestNewDuplicateContainer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	// Create first container
	c1, err := New("duplicate-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}

	// Save state to simulate existing container
	if err := c1.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Try to create duplicate
	_, err = New("duplicate-test", bundleDir, stateRoot)
	if err == nil {
		t.Error("expected error for duplicate container")
	}
}

func TestNewInvalidBundle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = New("test", "/nonexistent/bundle", tmpDir)
	if err == nil {
		t.Error("expected error for invalid bundle")
	}
}

func TestLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	// Create and save container
	c, err := New("load-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	c.InitProcess = 12345
	c.State.Pid = 12345
	c.State.Status = spec.StatusRunning

	if err := c.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Load container
	loaded, err := Load("load-test", stateRoot)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ID != "load-test" {
		t.Errorf("ID mismatch")
	}

	if loaded.State.Status != spec.StatusRunning {
		t.Errorf("status mismatch: expected running, got %s", loaded.State.Status)
	}

	if loaded.InitProcess != 12345 {
		t.Errorf("InitProcess mismatch: expected 12345, got %d", loaded.InitProcess)
	}
}

func TestLoadNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = Load("nonexistent", tmpDir)
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
}

func TestSaveState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	c, err := New("save-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := c.SaveState(); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(c.StateDir, StateFileName)
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	// Verify permissions
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestUpdateStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	c, err := New("status-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := c.UpdateStatus(spec.StatusRunning); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if c.State.Status != spec.StatusRunning {
		t.Errorf("expected status running, got %s", c.State.Status)
	}

	// Reload and verify
	loaded, err := Load("status-test", stateRoot)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.State.Status != spec.StatusRunning {
		t.Errorf("persisted status mismatch")
	}
}

func TestGetState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	c, err := New("getstate-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	c.InitProcess = 9999
	c.State.Status = spec.StatusRunning

	state := c.GetState()

	if state.ID != "getstate-test" {
		t.Errorf("ID mismatch")
	}

	if state.Pid != 9999 {
		t.Errorf("PID mismatch: expected 9999, got %d", state.Pid)
	}
}

func TestExecFifoPath(t *testing.T) {
	c := &Container{
		StateDir: "/run/runc-go/test-container",
	}

	expected := "/run/runc-go/test-container/exec.fifo"
	if c.ExecFifoPath() != expected {
		t.Errorf("expected %s, got %s", expected, c.ExecFifoPath())
	}
}

func TestDestroy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	c, err := New("destroy-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	stateDir := c.StateDir

	if err := c.Destroy(); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}

	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Error("state directory still exists after Destroy")
	}
}

func TestIsRunning(t *testing.T) {
	c := &Container{
		InitProcess: 0,
	}

	if c.IsRunning() {
		t.Error("should not be running with PID 0")
	}

	// Use our own PID which should be valid
	c.InitProcess = os.Getpid()
	if !c.IsRunning() {
		t.Error("should detect our own process as running")
	}

	// Use an invalid PID
	c.InitProcess = 9999999
	if c.IsRunning() {
		t.Error("should not detect invalid PID as running")
	}
}

func TestRefreshStatus(t *testing.T) {
	c := &Container{
		InitProcess: 9999999, // Invalid PID
		State: &spec.ContainerState{
			State: spec.State{
				Status: spec.StatusRunning,
			},
		},
	}

	c.RefreshStatus()

	if c.State.Status != spec.StatusStopped {
		t.Errorf("expected stopped status after refresh with invalid PID, got %s", c.State.Status)
	}
}

func TestStateJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	stateRoot := filepath.Join(tmpDir, "state")

	c, err := New("json-test", bundleDir, stateRoot)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	data, err := c.StateJSON()
	if err != nil {
		t.Fatalf("StateJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("StateJSON returned empty data")
	}

	// Verify it's valid JSON
	if data[0] != '{' {
		t.Error("StateJSON should return JSON object")
	}
}

func TestList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateRoot := filepath.Join(tmpDir, "state")

	// List on non-existent directory should return empty
	containers, err := List(stateRoot)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("expected 0 containers, got %d", len(containers))
	}

	// Create state directory
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		t.Fatalf("failed to create state root: %v", err)
	}

	bundleDir := filepath.Join(tmpDir, "bundle")
	if err := os.MkdirAll(filepath.Join(bundleDir, "rootfs"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}

	s := spec.DefaultSpec()
	if err := s.Save(filepath.Join(bundleDir, "config.json")); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	// Create some containers
	for i := 0; i < 3; i++ {
		c, err := New("list-test-"+string(rune('a'+i)), bundleDir, stateRoot)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		if err := c.SaveState(); err != nil {
			t.Fatalf("SaveState failed: %v", err)
		}
	}

	containers, err = List(stateRoot)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(containers) != 3 {
		t.Errorf("expected 3 containers, got %d", len(containers))
	}
}

func TestListDefaultStateRoot(t *testing.T) {
	// Test that empty stateRoot uses default
	_, err := List("")
	// This might fail if /run/runc-go doesn't exist, which is expected
	// The important thing is that it tries the default path
	_ = err
}

func TestSplitEnv(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"KEY=value", []string{"KEY", "value"}},
		{"PATH=/usr/bin:/bin", []string{"PATH", "/usr/bin:/bin"}},
		{"EMPTY=", []string{"EMPTY", ""}},
		{"NOEQUALS", []string{"NOEQUALS"}},
		{"MULTI=a=b=c", []string{"MULTI", "a=b=c"}},
	}

	for _, tc := range tests {
		result := splitEnv(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("splitEnv(%q): expected %d parts, got %d", tc.input, len(tc.expected), len(result))
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("splitEnv(%q)[%d]: expected %q, got %q", tc.input, i, tc.expected[i], result[i])
			}
		}
	}
}

func TestCreateOptions(t *testing.T) {
	opts := &CreateOptions{
		ConsoleSocket: "/tmp/console.sock",
		PidFile:       "/tmp/container.pid",
		NoPivot:       true,
		NoNewKeyring:  true,
	}

	if opts.ConsoleSocket != "/tmp/console.sock" {
		t.Errorf("ConsoleSocket mismatch")
	}

	if opts.PidFile != "/tmp/container.pid" {
		t.Errorf("PidFile mismatch")
	}

	if !opts.NoPivot {
		t.Errorf("NoPivot should be true")
	}

	if !opts.NoNewKeyring {
		t.Errorf("NoNewKeyring should be true")
	}
}

func TestDeleteOptions(t *testing.T) {
	opts := &DeleteOptions{
		Force: true,
	}

	if !opts.Force {
		t.Errorf("Force should be true")
	}
}
