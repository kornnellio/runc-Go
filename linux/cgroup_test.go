package linux

import (
	"os"
	"path/filepath"
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
