package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVersion(t *testing.T) {
	if Version != "1.0.2" {
		t.Errorf("expected version 1.0.2, got %s", Version)
	}
}

func TestDefaultSpec(t *testing.T) {
	spec := DefaultSpec()

	if spec == nil {
		t.Fatal("DefaultSpec returned nil")
	}

	if spec.Version != Version {
		t.Errorf("expected version %s, got %s", Version, spec.Version)
	}

	if spec.Root == nil {
		t.Fatal("Root is nil")
	}

	if spec.Root.Path != "rootfs" {
		t.Errorf("expected root path 'rootfs', got %s", spec.Root.Path)
	}

	if spec.Process == nil {
		t.Fatal("Process is nil")
	}

	if len(spec.Process.Args) == 0 {
		t.Error("Process.Args is empty")
	}

	if spec.Process.Args[0] != "/bin/sh" {
		t.Errorf("expected /bin/sh, got %s", spec.Process.Args[0])
	}

	if spec.Hostname != "container" {
		t.Errorf("expected hostname 'container', got %s", spec.Hostname)
	}

	if spec.Linux == nil {
		t.Fatal("Linux config is nil")
	}

	if len(spec.Linux.Namespaces) == 0 {
		t.Error("No namespaces configured")
	}

	// Check required namespaces are present
	namespaceTypes := make(map[LinuxNamespaceType]bool)
	for _, ns := range spec.Linux.Namespaces {
		namespaceTypes[ns.Type] = true
	}

	requiredNamespaces := []LinuxNamespaceType{
		PIDNamespace,
		NetworkNamespace,
		IPCNamespace,
		UTSNamespace,
		MountNamespace,
	}

	for _, ns := range requiredNamespaces {
		if !namespaceTypes[ns] {
			t.Errorf("missing required namespace: %s", ns)
		}
	}
}

func TestDefaultCapabilities(t *testing.T) {
	caps := defaultCapabilities()

	if len(caps) == 0 {
		t.Error("defaultCapabilities returned empty slice")
	}

	// Check for some expected capabilities
	expected := map[string]bool{
		"CAP_CHOWN":      false,
		"CAP_SETUID":     false,
		"CAP_SETGID":     false,
		"CAP_KILL":       false,
		"CAP_SYS_CHROOT": false,
	}

	for _, cap := range caps {
		if _, ok := expected[cap]; ok {
			expected[cap] = true
		}
	}

	for cap, found := range expected {
		if !found {
			t.Errorf("missing expected capability: %s", cap)
		}
	}
}

func TestLoadSpec(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test config.json
	spec := DefaultSpec()
	specPath := filepath.Join(tmpDir, "config.json")

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	if err := os.WriteFile(specPath, data, 0600); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	// Load the spec
	loaded, err := LoadSpec(specPath)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}

	if loaded.Version != spec.Version {
		t.Errorf("version mismatch: expected %s, got %s", spec.Version, loaded.Version)
	}

	if loaded.Hostname != spec.Hostname {
		t.Errorf("hostname mismatch: expected %s, got %s", spec.Hostname, loaded.Hostname)
	}
}

func TestLoadSpecNotFound(t *testing.T) {
	_, err := LoadSpec("/nonexistent/path/config.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadSpecInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	specPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(specPath, []byte("invalid json"), 0600); err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	_, err = LoadSpec(specPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveSpec(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runc-go-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	spec := DefaultSpec()
	spec.Hostname = "test-container"
	specPath := filepath.Join(tmpDir, "config.json")

	if err := spec.Save(specPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(specPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Load and verify
	loaded, err := LoadSpec(specPath)
	if err != nil {
		t.Fatalf("failed to reload spec: %v", err)
	}

	if loaded.Hostname != "test-container" {
		t.Errorf("hostname mismatch after reload: expected test-container, got %s", loaded.Hostname)
	}
}

func TestSpecJSONSerialization(t *testing.T) {
	spec := &Spec{
		Version:  Version,
		Hostname: "test",
		Root: &Root{
			Path:     "rootfs",
			Readonly: true,
		},
		Process: &Process{
			Terminal: true,
			Args:     []string{"/bin/sh", "-c", "echo hello"},
			Env:      []string{"PATH=/bin", "HOME=/root"},
			Cwd:      "/",
			User: User{
				UID: 1000,
				GID: 1000,
			},
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Spec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Version != spec.Version {
		t.Errorf("version mismatch")
	}

	if decoded.Hostname != spec.Hostname {
		t.Errorf("hostname mismatch")
	}

	if decoded.Root.Readonly != spec.Root.Readonly {
		t.Errorf("root readonly mismatch")
	}

	if decoded.Process.User.UID != spec.Process.User.UID {
		t.Errorf("user UID mismatch")
	}
}

func TestNamespaceTypes(t *testing.T) {
	tests := []struct {
		nsType   LinuxNamespaceType
		expected string
	}{
		{PIDNamespace, "pid"},
		{NetworkNamespace, "network"},
		{MountNamespace, "mount"},
		{IPCNamespace, "ipc"},
		{UTSNamespace, "uts"},
		{UserNamespace, "user"},
		{CgroupNamespace, "cgroup"},
		{TimeNamespace, "time"},
	}

	for _, tc := range tests {
		if string(tc.nsType) != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.nsType)
		}
	}
}

func TestSeccompActions(t *testing.T) {
	actions := []LinuxSeccompAction{
		ActKill,
		ActKillProcess,
		ActKillThread,
		ActTrap,
		ActErrno,
		ActTrace,
		ActAllow,
		ActLog,
		ActNotify,
	}

	for _, action := range actions {
		if action == "" {
			t.Error("empty seccomp action")
		}
	}
}

func TestContainerStatuses(t *testing.T) {
	statuses := []ContainerStatus{
		StatusCreating,
		StatusCreated,
		StatusRunning,
		StatusStopped,
	}

	expected := []string{"creating", "created", "running", "stopped"}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], status)
		}
	}
}

func TestMountSerialization(t *testing.T) {
	mount := Mount{
		Destination: "/data",
		Type:        "bind",
		Source:      "/host/data",
		Options:     []string{"rbind", "rw"},
	}

	data, err := json.Marshal(mount)
	if err != nil {
		t.Fatalf("failed to marshal mount: %v", err)
	}

	var decoded Mount
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal mount: %v", err)
	}

	if decoded.Destination != mount.Destination {
		t.Errorf("destination mismatch")
	}

	if len(decoded.Options) != len(mount.Options) {
		t.Errorf("options length mismatch")
	}
}

func TestLinuxResourcesSerialization(t *testing.T) {
	limit := int64(1024 * 1024 * 100) // 100MB
	resources := &LinuxResources{
		Memory: &LinuxMemory{
			Limit: &limit,
		},
		Pids: &LinuxPids{
			Limit: 100,
		},
	}

	data, err := json.Marshal(resources)
	if err != nil {
		t.Fatalf("failed to marshal resources: %v", err)
	}

	var decoded LinuxResources
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal resources: %v", err)
	}

	if decoded.Memory == nil || decoded.Memory.Limit == nil {
		t.Fatal("memory limit not preserved")
	}

	if *decoded.Memory.Limit != limit {
		t.Errorf("memory limit mismatch: expected %d, got %d", limit, *decoded.Memory.Limit)
	}

	if decoded.Pids == nil || decoded.Pids.Limit != 100 {
		t.Error("pids limit not preserved")
	}
}

func TestIntPtr(t *testing.T) {
	val := int64(42)
	ptr := intPtr(val)

	if ptr == nil {
		t.Fatal("intPtr returned nil")
	}

	if *ptr != val {
		t.Errorf("expected %d, got %d", val, *ptr)
	}
}
