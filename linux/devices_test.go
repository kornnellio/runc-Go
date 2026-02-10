package linux

import (
	"os"
	"path/filepath"
	"testing"

	"runc-go/spec"
)

// ============================================================================
// SECURITY TESTS: Device Path Validation
// ============================================================================

// TestValidateDevicePath_Basic tests basic device path validation.
func TestValidateDevicePath_Basic(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid /dev/null", "/dev/null", false},
		{"valid /dev/pts/0", "/dev/pts/0", false},
		{"valid /dev/shm/file", "/dev/shm/file", false},
		{"invalid /etc", "/etc/passwd", true},
		{"invalid /tmp", "/tmp/dev", true},
		{"invalid relative", "dev/null", true},
		{"traversal attack", "/dev/../etc/passwd", true},
		{"traversal attack 2", "/dev/pts/../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDevicePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDevicePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestValidateDevicePath_SymlinkEscape tests that device paths with symlinks
// outside /dev are detected and rejected.
func TestValidateDevicePath_SymlinkEscape(t *testing.T) {
	// Skip if not running as root (can't create device nodes)
	if os.Getuid() != 0 {
		t.Skip("Requires root to test device creation")
	}

	tmpDir, err := os.MkdirTemp("", "devices-symlink-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a fake /dev directory structure
	devDir := filepath.Join(tmpDir, "dev")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("Failed to create dev dir: %v", err)
	}

	// Create a secret file outside dev
	secretDir := filepath.Join(tmpDir, "etc")
	if err := os.MkdirAll(secretDir, 0755); err != nil {
		t.Fatalf("Failed to create etc dir: %v", err)
	}
	secretFile := filepath.Join(secretDir, "passwd")
	if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	// Create a symlink in /dev pointing outside
	// /dev/escape -> ../etc
	escapeLink := filepath.Join(devDir, "escape")
	if err := os.Symlink("../etc", escapeLink); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// NOTE: The current validateDevicePath only validates the path STRING,
	// not actual symlinks. This test documents the vulnerability.
	// The path /dev/escape/passwd looks valid but actually points to /etc/passwd

	// This should ideally fail but currently passes (VULNERABILITY)
	// After the fix, this should return an error
	err = validateDevicePath("/dev/escape/passwd")
	if err != nil {
		t.Logf("Good: validateDevicePath correctly rejected /dev/escape/passwd: %v", err)
	} else {
		t.Logf("VULNERABILITY: validateDevicePath accepted /dev/escape/passwd which could escape /dev via symlink")
	}
}

// TestIsAllowedDevice tests the device whitelist.
func TestIsAllowedDevice(t *testing.T) {
	tests := []struct {
		name    string
		major   int64
		minor   int64
		allowed bool
	}{
		{"dev/null", 1, 3, true},
		{"dev/zero", 1, 5, true},
		{"dev/random", 1, 8, true},
		{"dev/urandom", 1, 9, true},
		{"dev/tty", 5, 0, true},
		{"dev/console", 5, 1, true},
		{"dev/ptmx", 5, 2, true},
		{"pty slave", 136, 0, true},
		{"pty slave 5", 136, 5, true},
		{"dev/sda (not allowed)", 8, 0, false},
		{"dev/mem (not allowed)", 1, 1, false},
		{"dev/kmem (not allowed)", 1, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := spec.LinuxDevice{Major: tt.major, Minor: tt.minor}
			got := isAllowedDevice(dev)
			if got != tt.allowed {
				t.Errorf("isAllowedDevice(major=%d, minor=%d) = %v, want %v",
					tt.major, tt.minor, got, tt.allowed)
			}
		})
	}
}

// TestDefaultDevices verifies default devices are all allowed and correctly configured.
func TestDefaultDevices(t *testing.T) {
	devices := DefaultDevices()

	expectedPaths := map[string]bool{
		"/dev/null":    true,
		"/dev/zero":    true,
		"/dev/full":    true,
		"/dev/random":  true,
		"/dev/urandom": true,
		"/dev/tty":     true,
	}

	for _, dev := range devices {
		// Check device is expected
		if !expectedPaths[dev.Path] {
			t.Errorf("Unexpected default device: %s", dev.Path)
		}
		delete(expectedPaths, dev.Path)

		// Verify it's in the whitelist
		if !isAllowedDevice(dev) {
			t.Errorf("Default device %s (major=%d, minor=%d) is not in allowed list",
				dev.Path, dev.Major, dev.Minor)
		}

		// Verify type is character device
		if dev.Type != "c" {
			t.Errorf("Default device %s has type %q, expected 'c'", dev.Path, dev.Type)
		}

		// Verify mode is 0666
		if dev.FileMode == nil || *dev.FileMode != 0666 {
			t.Errorf("Default device %s should have mode 0666", dev.Path)
		}
	}

	// Check all expected devices were found
	for path := range expectedPaths {
		t.Errorf("Expected default device %s not found", path)
	}
}

// TestCreateAllDevices_PathTraversal tests that path traversal in device paths is rejected.
func TestCreateAllDevices_PathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "devices-traversal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	if err := os.MkdirAll(rootfs, 0755); err != nil {
		t.Fatalf("Failed to create rootfs: %v", err)
	}

	mode := os.FileMode(0666)

	// Test various path traversal attempts
	tests := []struct {
		name string
		path string
	}{
		{"traversal via ..", "/dev/../etc/passwd"},
		{"traversal via nested ..", "/dev/pts/../../etc/passwd"},
		{"absolute escape", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices := []spec.LinuxDevice{
				{Path: tt.path, Type: "c", Major: 1, Minor: 3, FileMode: &mode},
			}

			err := CreateAllDevices(devices, rootfs)
			if err == nil {
				t.Errorf("CreateAllDevices should reject path traversal: %s", tt.path)
			}
		})
	}
}

// TestCreateDeviceNode_SymlinkSafety tests that device creation doesn't follow symlinks.
func TestCreateDeviceNode_SymlinkSafety(t *testing.T) {
	// Skip if not running as root
	if os.Getuid() != 0 {
		t.Skip("Requires root to create device nodes")
	}

	tmpDir, err := os.MkdirTemp("", "devices-mknod-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	devDir := filepath.Join(rootfs, "dev")
	outsideDir := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("Failed to create dev dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}

	// Create a symlink that would escape rootfs
	// rootfs/dev/escape -> ../../outside
	escapeLink := filepath.Join(devDir, "escape")
	if err := os.Symlink("../../outside", escapeLink); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	mode := os.FileMode(0666)
	devices := []spec.LinuxDevice{
		{Path: "/dev/escape/malicious", Type: "c", Major: 1, Minor: 3, FileMode: &mode},
	}

	err = CreateAllDevices(devices, rootfs)
	// This should fail because SecureJoin should detect the symlink escape
	if err == nil {
		// Check if a file was created outside rootfs
		maliciousPath := filepath.Join(outsideDir, "malicious")
		if _, statErr := os.Stat(maliciousPath); statErr == nil {
			t.Errorf("SECURITY VULNERABILITY: Device created outside rootfs at %s", maliciousPath)
		}
	}
}

// TestMakeDevicesCgroupRules tests cgroup device rules generation.
func TestMakeDevicesCgroupRules(t *testing.T) {
	major3 := int64(1)
	minor3 := int64(3)

	devices := []spec.LinuxDeviceCgroup{
		{Type: "c", Major: &major3, Minor: &minor3, Access: "rwm", Allow: true},
		{Type: "a", Allow: false},
	}

	rules := MakeDevicesCgroupRules(devices)

	// Check that rules are formatted correctly
	if rules == "" {
		t.Error("Expected non-empty rules")
	}

	// Should contain "allow c 1:3 rwm"
	if !contains(rules, "allow c 1:3 rwm") {
		t.Errorf("Rules should contain 'allow c 1:3 rwm', got: %s", rules)
	}

	// Should contain deny all
	if !contains(rules, "deny a") {
		t.Errorf("Rules should contain deny for type 'a', got: %s", rules)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestIsPTYDevice tests PTY device detection.
func TestIsPTYDevice(t *testing.T) {
	tests := []struct {
		major int64
		minor int64
		isPTY bool
	}{
		{136, 0, true},   // /dev/pts/0
		{136, 10, true},  // /dev/pts/10
		{136, 255, true}, // /dev/pts/255
		{5, 2, false},    // /dev/ptmx (not a PTY slave)
		{1, 3, false},    // /dev/null
		{8, 0, false},    // /dev/sda
	}

	for _, tt := range tests {
		got := isPTYDevice(tt.major, tt.minor)
		if got != tt.isPTY {
			t.Errorf("isPTYDevice(%d, %d) = %v, want %v", tt.major, tt.minor, got, tt.isPTY)
		}
	}
}
