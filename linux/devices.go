// Package linux provides device node management.
package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"runc-go/spec"
)

// allowedDevices is a whitelist of safe device major:minor numbers.
// This prevents container specs from creating arbitrary devices like /dev/sda.
var allowedDevices = map[string]bool{
	// Standard character devices
	"1:3":  true, // /dev/null
	"1:5":  true, // /dev/zero
	"1:7":  true, // /dev/full
	"1:8":  true, // /dev/random
	"1:9":  true, // /dev/urandom
	"5:0":  true, // /dev/tty
	"5:1":  true, // /dev/console
	"5:2":  true, // /dev/ptmx
	"1:11": true, // /dev/kmsg (read-only usually)
	// PTY devices (136:* for pts)
}

// isPTYDevice checks if a device is a PTY (major 136 for unix98 PTYs).
func isPTYDevice(major, minor int64) bool {
	return major == 136 // unix98 PTY slaves
}

// isAllowedDevice checks if a device is in the whitelist.
func isAllowedDevice(dev spec.LinuxDevice) bool {
	key := fmt.Sprintf("%d:%d", dev.Major, dev.Minor)
	if allowedDevices[key] {
		return true
	}
	// Also allow PTY devices
	if isPTYDevice(dev.Major, dev.Minor) {
		return true
	}
	return false
}

// validateDevicePath ensures a device path is safe (within /dev).
func validateDevicePath(path string) error {
	// Clean the path
	cleaned := filepath.Clean(path)

	// Must start with /dev/
	if !strings.HasPrefix(cleaned, "/dev/") && cleaned != "/dev" {
		return fmt.Errorf("device path %q must be under /dev", path)
	}

	// No path traversal after /dev
	if strings.Contains(cleaned[4:], "..") {
		return fmt.Errorf("device path %q contains path traversal", path)
	}

	return nil
}

// DefaultDevices returns the standard set of devices for a container.
func DefaultDevices() []spec.LinuxDevice {
	mode := os.FileMode(0666)
	return []spec.LinuxDevice{
		{Path: "/dev/null", Type: "c", Major: 1, Minor: 3, FileMode: &mode},
		{Path: "/dev/zero", Type: "c", Major: 1, Minor: 5, FileMode: &mode},
		{Path: "/dev/full", Type: "c", Major: 1, Minor: 7, FileMode: &mode},
		{Path: "/dev/random", Type: "c", Major: 1, Minor: 8, FileMode: &mode},
		{Path: "/dev/urandom", Type: "c", Major: 1, Minor: 9, FileMode: &mode},
		{Path: "/dev/tty", Type: "c", Major: 5, Minor: 0, FileMode: &mode},
	}
}

// CreateAllDevices creates all device nodes for the container.
func CreateAllDevices(devices []spec.LinuxDevice, rootfs string) error {
	for _, dev := range devices {
		// Validate device path format
		if err := validateDevicePath(dev.Path); err != nil {
			return fmt.Errorf("invalid device path: %w", err)
		}

		// Check if device is in whitelist (skip for default devices)
		if !isAllowedDevice(dev) {
			return fmt.Errorf("device %s (major:minor %d:%d) is not in allowed list",
				dev.Path, dev.Major, dev.Minor)
		}

		path := dev.Path
		if rootfs != "" {
			// Use SecureJoin to prevent path traversal
			var err error
			path, err = SecureJoin(rootfs, dev.Path)
			if err != nil {
				return fmt.Errorf("invalid device path %q: %w", dev.Path, err)
			}
		}

		if err := createDeviceNode(path, dev); err != nil {
			return fmt.Errorf("create device %s: %w", dev.Path, err)
		}
	}
	return nil
}

// createDeviceNode creates a single device node.
func createDeviceNode(path string, dev spec.LinuxDevice) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Calculate device type
	var devType uint32
	switch dev.Type {
	case "c", "u": // Character device
		devType = syscall.S_IFCHR
	case "b": // Block device
		devType = syscall.S_IFBLK
	case "p": // FIFO (named pipe)
		devType = syscall.S_IFIFO
	default:
		return fmt.Errorf("unknown device type: %s", dev.Type)
	}

	// Calculate file mode
	var mode uint32 = devType | 0666
	if dev.FileMode != nil {
		mode = devType | uint32(*dev.FileMode)
	}

	// Calculate device number (major << 8 | minor)
	devNum := int((dev.Major << 8) | dev.Minor)

	// Remove existing device if present
	os.Remove(path)

	// Create device node
	if err := syscall.Mknod(path, mode, devNum); err != nil {
		return fmt.Errorf("mknod: %w", err)
	}

	// Set ownership
	uid := 0
	gid := 0
	if dev.UID != nil {
		uid = int(*dev.UID)
	}
	if dev.GID != nil {
		gid = int(*dev.GID)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown: %w", err)
	}

	return nil
}

// BindMountDevices bind-mounts devices from host instead of creating them.
// This is useful when mknod is not available (e.g., rootless containers).
func BindMountDevices(devices []spec.LinuxDevice, rootfs string) error {
	for _, dev := range devices {
		// Validate device path
		if err := validateDevicePath(dev.Path); err != nil {
			return fmt.Errorf("invalid device path: %w", err)
		}

		hostPath := dev.Path
		containerPath := dev.Path
		if rootfs != "" {
			// Use SecureJoin to prevent path traversal
			var err error
			containerPath, err = SecureJoin(rootfs, dev.Path)
			if err != nil {
				return fmt.Errorf("invalid device path %q: %w", dev.Path, err)
			}
		}

		// Check if host device exists
		if _, err := os.Stat(hostPath); err != nil {
			continue // Skip if host device doesn't exist
		}

		// Create mount point
		dir := filepath.Dir(containerPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}

		// Create empty file for bind mount
		f, err := os.OpenFile(containerPath, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return fmt.Errorf("create %s: %w", containerPath, err)
		}
		f.Close()

		// Bind mount from host
		if err := syscall.Mount(hostPath, containerPath, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind mount %s: %w", containerPath, err)
		}
	}
	return nil
}

// SetupDevTmpfs sets up /dev as a tmpfs with necessary devices and symlinks.
func SetupDevTmpfs(rootfs string, devices []spec.LinuxDevice) error {
	devPath := "/dev"
	if rootfs != "" {
		devPath = filepath.Join(rootfs, "dev")
	}

	// Create /dev directory
	if err := os.MkdirAll(devPath, 0755); err != nil {
		return fmt.Errorf("mkdir /dev: %w", err)
	}

	// Mount tmpfs on /dev
	if err := syscall.Mount("tmpfs", devPath, "tmpfs",
		syscall.MS_NOSUID|syscall.MS_STRICTATIME,
		"mode=755,size=65536k"); err != nil {
		return fmt.Errorf("mount tmpfs on /dev: %w", err)
	}

	// Create device nodes
	allDevices := devices
	if len(allDevices) == 0 {
		allDevices = DefaultDevices()
	}

	for _, dev := range allDevices {
		path := filepath.Join(devPath, filepath.Base(dev.Path))
		if err := createDeviceNode(path, dev); err != nil {
			// Non-fatal, try to continue
			fmt.Printf("[dev] warning: %v\n", err)
		}
	}

	// Create /dev/pts
	ptsPath := filepath.Join(devPath, "pts")
	if err := os.MkdirAll(ptsPath, 0755); err != nil {
		return fmt.Errorf("mkdir /dev/pts: %w", err)
	}

	// Mount devpts
	if err := syscall.Mount("devpts", ptsPath, "devpts",
		syscall.MS_NOSUID|syscall.MS_NOEXEC,
		"newinstance,ptmxmode=0666,mode=0620"); err != nil {
		fmt.Printf("[dev] warning: mount devpts: %v\n", err)
	}

	// Create /dev/ptmx symlink
	ptmxPath := filepath.Join(devPath, "ptmx")
	os.Remove(ptmxPath)
	if err := os.Symlink("pts/ptmx", ptmxPath); err != nil {
		// Try creating device node instead
		mode := os.FileMode(0666)
		dev := spec.LinuxDevice{Path: ptmxPath, Type: "c", Major: 5, Minor: 2, FileMode: &mode}
		createDeviceNode(ptmxPath, dev)
	}

	// Create /dev/shm
	shmPath := filepath.Join(devPath, "shm")
	if err := os.MkdirAll(shmPath, 1777); err != nil {
		return fmt.Errorf("mkdir /dev/shm: %w", err)
	}
	if err := syscall.Mount("shm", shmPath, "tmpfs",
		syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV,
		"mode=1777,size=65536k"); err != nil {
		fmt.Printf("[dev] warning: mount shm: %v\n", err)
	}

	// Create /dev/mqueue
	mqPath := filepath.Join(devPath, "mqueue")
	if err := os.MkdirAll(mqPath, 0755); err == nil {
		syscall.Mount("mqueue", mqPath, "mqueue",
			syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
	}

	// Create standard symlinks
	symlinks := map[string]string{
		"fd":     "/proc/self/fd",
		"stdin":  "/proc/self/fd/0",
		"stdout": "/proc/self/fd/1",
		"stderr": "/proc/self/fd/2",
	}

	for name, target := range symlinks {
		linkPath := filepath.Join(devPath, name)
		os.Remove(linkPath)
		os.Symlink(target, linkPath)
	}

	// Create /dev/console placeholder
	consolePath := filepath.Join(devPath, "console")
	f, _ := os.OpenFile(consolePath, os.O_CREATE|os.O_WRONLY, 0600)
	if f != nil {
		f.Close()
	}

	return nil
}

// MakeDevicesCgroupRules creates cgroup device rules from OCI config.
func MakeDevicesCgroupRules(devices []spec.LinuxDeviceCgroup) string {
	// Format: TYPE MAJOR:MINOR ACCESS
	// Example: "c 1:3 rwm" (allow read/write/mknod on /dev/null)
	var rules string
	for _, dev := range devices {
		var devType string
		switch dev.Type {
		case "a":
			devType = "a"
		case "c":
			devType = "c"
		case "b":
			devType = "b"
		default:
			continue
		}

		major := "*"
		if dev.Major != nil {
			major = fmt.Sprintf("%d", *dev.Major)
		}

		minor := "*"
		if dev.Minor != nil {
			minor = fmt.Sprintf("%d", *dev.Minor)
		}

		access := dev.Access
		if access == "" {
			access = "rwm"
		}

		allow := "deny"
		if dev.Allow {
			allow = "allow"
		}

		rules += fmt.Sprintf("%s %s %s:%s %s\n", allow, devType, major, minor, access)
	}
	return rules
}
