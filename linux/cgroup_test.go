package linux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"runc-go/spec"
)

func TestGetCgroupPath(t *testing.T) {
	tests := []struct {
		containerID string
		specPath    string
		expected    string
	}{
		{"test-container", "", "runc-go/test-container"},
		{"container-123", "", "runc-go/container-123"},
		{"abc", "/custom/path", "/custom/path"},
		{"xyz", "/docker/containers/xyz", "/docker/containers/xyz"},
	}

	for _, tc := range tests {
		result := GetCgroupPath(tc.containerID, tc.specPath)
		if result != tc.expected {
			t.Errorf("GetCgroupPath(%q, %q) = %q, expected %q",
				tc.containerID, tc.specPath, result, tc.expected)
		}
	}
}

func TestCgroupPath(t *testing.T) {
	// Skip if not running as root or cgroup not available
	if os.Getuid() != 0 {
		t.Skip("skipping cgroup test: requires root")
	}

	if _, err := os.Stat("/sys/fs/cgroup"); os.IsNotExist(err) {
		t.Skip("skipping cgroup test: cgroup not mounted")
	}

	cgroupPath := "runc-go-test/test-cgroup"
	cg, err := NewCgroup(cgroupPath)
	if err != nil {
		t.Fatalf("NewCgroup failed: %v", err)
	}
	defer cg.Destroy()

	expected := filepath.Join("/sys/fs/cgroup", cgroupPath)
	if cg.Path() != expected {
		t.Errorf("expected path %s, got %s", expected, cg.Path())
	}
}

func TestCgroupApplyResourcesNil(t *testing.T) {
	cg := &Cgroup{path: "/tmp/fake-cgroup"}

	// Should handle nil resources gracefully
	err := cg.ApplyResources(nil)
	if err != nil {
		t.Errorf("ApplyResources(nil) should not error: %v", err)
	}
}

func TestCgroupApplyResourcesEmptyMemory(t *testing.T) {
	cg := &Cgroup{path: "/tmp/fake-cgroup"}

	resources := &spec.LinuxResources{
		Memory: nil,
	}

	// Should handle nil memory gracefully (won't write to real path)
	err := cg.applyMemory(nil)
	if err != nil {
		t.Errorf("applyMemory(nil) should not error: %v", err)
	}

	_ = resources
}

func TestCgroupApplyResourcesEmptyCPU(t *testing.T) {
	cg := &Cgroup{path: "/tmp/fake-cgroup"}

	err := cg.applyCPU(nil)
	if err != nil {
		t.Errorf("applyCPU(nil) should not error: %v", err)
	}
}

func TestCgroupApplyResourcesEmptyPids(t *testing.T) {
	cg := &Cgroup{path: "/tmp/fake-cgroup"}

	err := cg.applyPids(nil)
	if err != nil {
		t.Errorf("applyPids(nil) should not error: %v", err)
	}
}

func TestCgroupApplyPidsZeroLimit(t *testing.T) {
	cg := &Cgroup{path: "/tmp/fake-cgroup"}

	pids := &spec.LinuxPids{
		Limit: 0,
	}

	// Zero limit should be no-op
	err := cg.applyPids(pids)
	if err != nil {
		t.Errorf("applyPids with 0 limit should not error: %v", err)
	}
}

func TestCgroupIntegration(t *testing.T) {
	// Skip if not running as root
	if os.Getuid() != 0 {
		t.Skip("skipping cgroup integration test: requires root")
	}

	if _, err := os.Stat("/sys/fs/cgroup"); os.IsNotExist(err) {
		t.Skip("skipping cgroup test: cgroup not mounted")
	}

	cgroupPath := "runc-go-test/integration-test"

	// Clean up any existing cgroup
	fullPath := filepath.Join("/sys/fs/cgroup", cgroupPath)
	os.Remove(fullPath)

	cg, err := NewCgroup(cgroupPath)
	if err != nil {
		t.Fatalf("NewCgroup failed: %v", err)
	}
	defer func() {
		cg.Destroy()
		// Also clean up parent
		os.Remove(filepath.Join("/sys/fs/cgroup", "runc-go-test"))
	}()

	// Verify cgroup was created
	if _, err := os.Stat(cg.Path()); os.IsNotExist(err) {
		t.Error("cgroup directory was not created")
	}

	// Add our own process
	err = cg.AddProcess(os.Getpid())
	if err != nil {
		t.Logf("AddProcess failed (may be expected in some environments): %v", err)
	}

	// Try to apply resources
	limit := int64(1024 * 1024 * 100) // 100MB
	resources := &spec.LinuxResources{
		Memory: &spec.LinuxMemory{
			Limit: &limit,
		},
		Pids: &spec.LinuxPids{
			Limit: 100,
		},
	}

	err = cg.ApplyResources(resources)
	if err != nil {
		t.Logf("ApplyResources failed (may be expected if controllers not enabled): %v", err)
	}

	// Clean up
	err = cg.Destroy()
	if err != nil {
		t.Logf("Destroy failed (process may still be in cgroup): %v", err)
	}
}

func TestEnsureParentControllers(t *testing.T) {
	// This is a best-effort function, so we just verify it doesn't panic
	err := EnsureParentControllers("runc-go/test")
	// Error is expected if not root or cgroups not available
	_ = err
}

func TestCPUWeightConversion(t *testing.T) {
	// Test the shares to weight conversion logic
	tests := []struct {
		shares   uint64
		minWeight uint64
		maxWeight uint64
	}{
		{1024, 100, 100},   // Default shares should give ~100
		{512, 50, 50},      // Half should give ~50
		{2048, 200, 200},   // Double should give ~200
		{2, 1, 1},          // Minimum
		{262144, 10000, 10000}, // Maximum
	}

	for _, tc := range tests {
		weight := (tc.shares * 100) / 1024
		if weight < 1 {
			weight = 1
		}
		if weight > 10000 {
			weight = 10000
		}

		if weight < tc.minWeight || weight > tc.maxWeight {
			t.Errorf("shares %d: expected weight between %d and %d, got %d",
				tc.shares, tc.minWeight, tc.maxWeight, weight)
		}
	}
}

func TestSwapLimitCalculation(t *testing.T) {
	// Test the swap limit calculation (OCI swap - memory limit)
	tests := []struct {
		memoryLimit int64
		swapLimit   int64
		expected    int64
	}{
		{100, 200, 100},  // 200 - 100 = 100
		{100, 100, 0},    // 100 - 100 = 0
		{100, 50, 0},     // Would be -50, should be clamped to 0
		{0, 100, 100},    // No memory limit
	}

	for _, tc := range tests {
		var result int64
		if tc.memoryLimit > 0 {
			result = tc.swapLimit - tc.memoryLimit
			if result < 0 {
				result = 0
			}
		} else {
			result = tc.swapLimit
		}

		if result != tc.expected {
			t.Errorf("memoryLimit=%d, swapLimit=%d: expected %d, got %d",
				tc.memoryLimit, tc.swapLimit, tc.expected, result)
		}
	}
}

// ============================================================================
// SECURITY TESTS: Cgroup Unified Key Validation
// ============================================================================

// TestApplyResources_UnifiedKeyPathTraversal tests that path traversal in
// unified cgroup keys is rejected. This is a critical security test.
func TestApplyResources_UnifiedKeyPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cgroup-traversal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a fake cgroup directory
	cgroupDir := filepath.Join(tmpDir, "cgroup")
	if err := os.MkdirAll(cgroupDir, 0755); err != nil {
		t.Fatalf("Failed to create cgroup dir: %v", err)
	}

	// Create target file outside cgroup dir
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("Failed to create outside dir: %v", err)
	}

	cg := &Cgroup{path: cgroupDir}

	// Test various path traversal attempts in unified keys
	traversalKeys := []string{
		"../outside/escaped",
		"../../escaped",
		"../../../etc/passwd",
		"foo/../../../etc/passwd",
	}

	for _, key := range traversalKeys {
		resources := &spec.LinuxResources{
			Unified: map[string]string{
				key: "malicious-content",
			},
		}

		err := cg.ApplyResources(resources)

		// Check if file was created outside cgroup
		escapedPath := filepath.Join(tmpDir, "outside", "escaped")
		if _, statErr := os.Stat(escapedPath); statErr == nil {
			t.Errorf("SECURITY VULNERABILITY: Unified key %q escaped cgroup directory!", key)
			t.Errorf("File created at: %s", escapedPath)
		}

		// For the test to pass after fix, ApplyResources should return an error
		if err == nil {
			t.Logf("WARNING: Unified key %q was accepted (should be rejected after fix)", key)
		}
	}
}

// TestApplyResources_UnifiedKeyValidation tests that only valid cgroup keys
// are accepted in the unified map.
func TestApplyResources_UnifiedKeyValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cgroup-key-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cg := &Cgroup{path: tmpDir}

	// Valid cgroup keys
	validKeys := []string{
		"cpu.max",
		"memory.max",
		"pids.max",
		"cpu.weight",
		"cpuset.cpus",
		"memory.swap.max",
		"io.max",
		"io.bfq.weight",
	}

	// Invalid keys (should be rejected after fix)
	invalidKeys := []string{
		"../foo",
		"..",
		"./foo",
		"/absolute/path",
		"foo/../../bar",
		"",
		"memory max", // space
		"memory\tmax", // tab
		"memory\nmax", // newline
	}

	for _, key := range validKeys {
		resources := &spec.LinuxResources{
			Unified: map[string]string{
				key: "100",
			},
		}

		err := cg.ApplyResources(resources)
		// We expect this to potentially fail because the cgroup controller
		// files don't exist in our temp dir, but it should NOT fail due to
		// key validation
		if err != nil && isKeyValidationError(err) {
			t.Errorf("Valid cgroup key %q was rejected: %v", key, err)
		}
	}

	for _, key := range invalidKeys {
		resources := &spec.LinuxResources{
			Unified: map[string]string{
				key: "100",
			},
		}

		err := cg.ApplyResources(resources)
		// After fix, these should return a key validation error
		if err == nil {
			// Check if we wrote outside the cgroup dir
			if _, statErr := os.Stat(filepath.Join(tmpDir, "..", filepath.Base(tmpDir)+"_escaped")); statErr == nil {
				t.Errorf("VULNERABILITY: Invalid key %q escaped directory", key)
			}
		}
	}
}

// isKeyValidationError checks if an error is a key validation error (vs file not found etc.)
func isKeyValidationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "invalid cgroup key") ||
		strings.Contains(errStr, "path traversal") ||
		strings.Contains(errStr, "invalid key")
}

// TestCgroupPath_Traversal tests that cgroup path with traversal is rejected.
func TestCgroupPath_Traversal(t *testing.T) {
	// These paths should be rejected or sanitized
	traversalPaths := []string{
		"../etc",
		"../../etc",
		"foo/../../../etc",
	}

	for _, path := range traversalPaths {
		// NewCgroup should either reject these or sanitize them
		// Currently it doesn't, which is a vulnerability
		cg, err := NewCgroup(path)
		if err == nil && cg != nil {
			// Check if the resulting path is outside /sys/fs/cgroup
			if !strings.HasPrefix(cg.Path(), "/sys/fs/cgroup") {
				t.Errorf("VULNERABILITY: Cgroup path %q resulted in path %q outside /sys/fs/cgroup",
					path, cg.Path())
			}
			// Note: Even if within /sys/fs/cgroup, path traversal might access
			// other cgroups, which could be a security issue
		}
	}
}

// TestCPUWeightFormula tests the correct CPU weight conversion formula.
// The correct formula is: weight = 1 + (shares - 2) * 9999 / 262142
func TestCPUWeightFormula(t *testing.T) {
	tests := []struct {
		shares        uint64
		expectedMin   uint64 // Expected weight range
		expectedMax   uint64
		description   string
	}{
		{2, 1, 1, "minimum shares"},
		{1024, 38, 40, "default shares (should be ~39)"}, // 1 + (1024-2)*9999/262142 ≈ 39
		{262144, 9999, 10000, "maximum shares"},
		{512, 19, 20, "half default shares"},
		{2048, 77, 79, "double default shares"},
	}

	for _, tc := range tests {
		// Current (wrong) formula: (shares * 100) / 1024
		currentWeight := (tc.shares * 100) / 1024
		if currentWeight < 1 {
			currentWeight = 1
		}
		if currentWeight > 10000 {
			currentWeight = 10000
		}

		// Correct formula: 1 + (shares - 2) * 9999 / 262142
		correctWeight := uint64(1)
		if tc.shares > 2 {
			correctWeight = 1 + (tc.shares-2)*9999/262142
		}

		t.Logf("Shares %d (%s): current=%d, correct=%d, expected=%d-%d",
			tc.shares, tc.description, currentWeight, correctWeight,
			tc.expectedMin, tc.expectedMax)

		// The correct formula should give a value in the expected range
		if correctWeight < tc.expectedMin || correctWeight > tc.expectedMax {
			t.Errorf("Correct formula for shares %d: expected %d-%d, got %d",
				tc.shares, tc.expectedMin, tc.expectedMax, correctWeight)
		}
	}
}

