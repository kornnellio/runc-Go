// Package linux provides cgroup v2 resource management.
package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"runc-go/spec"
)

// validCgroupKey matches valid cgroup v2 controller file names.
// Valid keys are like: cpu.max, memory.max, pids.max, io.bfq.weight
var validCgroupKey = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)*$`)

const cgroupRoot = "/sys/fs/cgroup"

// Cgroup represents a cgroup v2 control group.
type Cgroup struct {
	path string
}

// NewCgroup creates or opens a cgroup at the given path.
// Path should be relative to /sys/fs/cgroup (e.g., "runc-go/container-id").
func NewCgroup(cgroupPath string) (*Cgroup, error) {
	// Handle absolute paths or OCI-style paths
	var fullPath string
	if strings.HasPrefix(cgroupPath, "/") {
		fullPath = filepath.Join(cgroupRoot, cgroupPath)
	} else {
		fullPath = filepath.Join(cgroupRoot, cgroupPath)
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return nil, fmt.Errorf("create cgroup directory: %w", err)
	}

	return &Cgroup{path: fullPath}, nil
}

// Path returns the filesystem path of the cgroup.
func (c *Cgroup) Path() string {
	return c.path
}

// AddProcess adds a process to this cgroup.
func (c *Cgroup) AddProcess(pid int) error {
	procsPath := filepath.Join(c.path, "cgroup.procs")
	return os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644)
}

// ApplyResources applies OCI resource limits to the cgroup.
func (c *Cgroup) ApplyResources(resources *spec.LinuxResources) error {
	if resources == nil {
		return nil
	}

	if err := c.applyMemory(resources.Memory); err != nil {
		return err
	}

	if err := c.applyCPU(resources.CPU); err != nil {
		return err
	}

	if err := c.applyPids(resources.Pids); err != nil {
		return err
	}

	// Apply unified cgroup v2 settings directly
	for key, value := range resources.Unified {
		// SECURITY: Validate cgroup key to prevent path traversal
		if err := validateCgroupKey(key); err != nil {
			return fmt.Errorf("invalid cgroup key %q: %w", key, err)
		}

		path := filepath.Join(c.path, key)
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return fmt.Errorf("write %s: %w", key, err)
		}
	}

	return nil
}

// applyMemory applies memory limits.
func (c *Cgroup) applyMemory(memory *spec.LinuxMemory) error {
	if memory == nil {
		return nil
	}

	// memory.max - hard limit
	if memory.Limit != nil && *memory.Limit > 0 {
		path := filepath.Join(c.path, "memory.max")
		if err := os.WriteFile(path, []byte(strconv.FormatInt(*memory.Limit, 10)), 0644); err != nil {
			return fmt.Errorf("set memory.max: %w", err)
		}
	}

	// memory.low - soft limit / reservation
	if memory.Reservation != nil && *memory.Reservation > 0 {
		path := filepath.Join(c.path, "memory.low")
		if err := os.WriteFile(path, []byte(strconv.FormatInt(*memory.Reservation, 10)), 0644); err != nil {
			return fmt.Errorf("set memory.low: %w", err)
		}
	}

	// memory.swap.max - swap limit
	if memory.Swap != nil {
		swapLimit := *memory.Swap
		// OCI spec: swap is memory+swap, cgroup v2 expects just swap
		if memory.Limit != nil {
			swapLimit = *memory.Swap - *memory.Limit
			if swapLimit < 0 {
				swapLimit = 0
			}
		}
		path := filepath.Join(c.path, "memory.swap.max")
		if err := os.WriteFile(path, []byte(strconv.FormatInt(swapLimit, 10)), 0644); err != nil {
			// Swap might not be enabled
			fmt.Printf("[cgroup] warning: set memory.swap.max: %v\n", err)
		}
	}

	return nil
}

// applyCPU applies CPU limits.
func (c *Cgroup) applyCPU(cpu *spec.LinuxCPU) error {
	if cpu == nil {
		return nil
	}

	// cpu.max - quota and period
	if cpu.Quota != nil || cpu.Period != nil {
		quota := "max"
		if cpu.Quota != nil && *cpu.Quota > 0 {
			quota = strconv.FormatInt(*cpu.Quota, 10)
		}
		period := uint64(100000) // Default 100ms
		if cpu.Period != nil && *cpu.Period > 0 {
			period = *cpu.Period
		}
		value := fmt.Sprintf("%s %d", quota, period)
		path := filepath.Join(c.path, "cpu.max")
		if err := os.WriteFile(path, []byte(value), 0644); err != nil {
			return fmt.Errorf("set cpu.max: %w", err)
		}
	}

	// cpu.weight (replaces cpu.shares)
	if cpu.Shares != nil && *cpu.Shares > 0 {
		// Convert shares to weight using the correct formula:
		// weight = 1 + (shares - 2) * 9999 / 262142
		// This maps shares (2-262144) to weight (1-10000)
		shares := *cpu.Shares
		var weight uint64 = 1
		if shares > 2 {
			weight = 1 + (shares-2)*9999/262142
		}
		if weight > 10000 {
			weight = 10000
		}
		path := filepath.Join(c.path, "cpu.weight")
		if err := os.WriteFile(path, []byte(strconv.FormatUint(weight, 10)), 0644); err != nil {
			return fmt.Errorf("set cpu.weight: %w", err)
		}
	}

	// cpuset.cpus
	if cpu.Cpus != "" {
		path := filepath.Join(c.path, "cpuset.cpus")
		if err := os.WriteFile(path, []byte(cpu.Cpus), 0644); err != nil {
			return fmt.Errorf("set cpuset.cpus: %w", err)
		}
	}

	// cpuset.mems
	if cpu.Mems != "" {
		path := filepath.Join(c.path, "cpuset.mems")
		if err := os.WriteFile(path, []byte(cpu.Mems), 0644); err != nil {
			return fmt.Errorf("set cpuset.mems: %w", err)
		}
	}

	return nil
}

// applyPids applies process count limits.
func (c *Cgroup) applyPids(pids *spec.LinuxPids) error {
	if pids == nil {
		return nil
	}

	if pids.Limit > 0 {
		path := filepath.Join(c.path, "pids.max")
		if err := os.WriteFile(path, []byte(strconv.FormatInt(pids.Limit, 10)), 0644); err != nil {
			return fmt.Errorf("set pids.max: %w", err)
		}
	}

	return nil
}

// Destroy removes the cgroup.
func (c *Cgroup) Destroy() error {
	// Cgroup must be empty to remove
	return os.Remove(c.path)
}

// GetMemoryCurrent returns current memory usage.
func (c *Cgroup) GetMemoryCurrent() (int64, error) {
	data, err := os.ReadFile(filepath.Join(c.path, "memory.current"))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// GetPidsCurrent returns current number of processes.
func (c *Cgroup) GetPidsCurrent() (int64, error) {
	data, err := os.ReadFile(filepath.Join(c.path, "pids.current"))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// Freeze freezes all processes in the cgroup.
func (c *Cgroup) Freeze() error {
	path := filepath.Join(c.path, "cgroup.freeze")
	return os.WriteFile(path, []byte("1"), 0644)
}

// Thaw unfreezes all processes in the cgroup.
func (c *Cgroup) Thaw() error {
	path := filepath.Join(c.path, "cgroup.freeze")
	return os.WriteFile(path, []byte("0"), 0644)
}

// EnsureParentControllers enables controllers on parent cgroups.
func EnsureParentControllers(cgroupPath string) error {
	// Walk up from cgroupPath and enable controllers at each level
	parts := strings.Split(strings.Trim(cgroupPath, "/"), "/")
	current := cgroupRoot

	controllers := "+cpu +memory +pids +cpuset"

	for _, part := range parts[:len(parts)] {
		controlFile := filepath.Join(current, "cgroup.subtree_control")
		if err := os.WriteFile(controlFile, []byte(controllers), 0644); err != nil {
			// Best effort - some controllers might not be available
		}
		current = filepath.Join(current, part)
	}

	return nil
}

// GetCgroupPath returns the default cgroup path for a container.
func GetCgroupPath(containerID string, specPath string) string {
	if specPath != "" {
		return specPath
	}
	return filepath.Join("runc-go", containerID)
}

// validateCgroupKey validates a cgroup controller file key.
// This prevents path traversal attacks via crafted unified keys.
func validateCgroupKey(key string) error {
	// Empty key is invalid
	if key == "" {
		return fmt.Errorf("empty key not allowed")
	}

	// Must not contain path separators
	if strings.ContainsAny(key, "/\\") {
		return fmt.Errorf("key contains path separator")
	}

	// Must not be . or ..
	if key == "." || key == ".." {
		return fmt.Errorf("key is relative path component")
	}

	// Must not start with .
	if strings.HasPrefix(key, ".") {
		return fmt.Errorf("key starts with dot")
	}

	// Must match valid cgroup key pattern (e.g., cpu.max, memory.swap.max)
	if !validCgroupKey.MatchString(key) {
		return fmt.Errorf("key does not match valid cgroup key pattern")
	}

	return nil
}
