// Package linux provides rootfs and mount handling.
package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"runc-go/spec"
)

// Mount propagation flags
const (
	MS_PRIVATE     = syscall.MS_PRIVATE
	MS_SHARED      = syscall.MS_SHARED
	MS_SLAVE       = syscall.MS_SLAVE
	MS_UNBINDABLE  = syscall.MS_UNBINDABLE
	MS_REC         = syscall.MS_REC
	MS_BIND        = syscall.MS_BIND
	MS_MOVE        = syscall.MS_MOVE
	MS_RDONLY      = syscall.MS_RDONLY
	MS_NOSUID      = syscall.MS_NOSUID
	MS_NODEV       = syscall.MS_NODEV
	MS_NOEXEC      = syscall.MS_NOEXEC
	MS_REMOUNT     = syscall.MS_REMOUNT
	MS_STRICTATIME = syscall.MS_STRICTATIME
	MS_RELATIME    = syscall.MS_RELATIME
	MS_NOATIME     = syscall.MS_NOATIME
)

// mountOptionFlags maps mount option strings to flags.
var mountOptionFlags = map[string]uintptr{
	"ro":         MS_RDONLY,
	"rw":         0,
	"nosuid":     MS_NOSUID,
	"suid":       0,
	"nodev":      MS_NODEV,
	"dev":        0,
	"noexec":     MS_NOEXEC,
	"exec":       0,
	"sync":       syscall.MS_SYNCHRONOUS,
	"async":      0,
	"remount":    MS_REMOUNT,
	"bind":       MS_BIND,
	"rbind":      MS_BIND | MS_REC,
	"private":    MS_PRIVATE,
	"rprivate":   MS_PRIVATE | MS_REC,
	"shared":     MS_SHARED,
	"rshared":    MS_SHARED | MS_REC,
	"slave":      MS_SLAVE,
	"rslave":     MS_SLAVE | MS_REC,
	"unbindable": MS_UNBINDABLE,
	"runbindable": MS_UNBINDABLE | MS_REC,
	"relatime":   MS_RELATIME,
	"norelatime": 0,
	"strictatime": MS_STRICTATIME,
	"noatime":    MS_NOATIME,
}

// SetupRootfs sets up the container's root filesystem.
func SetupRootfs(s *spec.Spec, bundlePath string) error {
	if s.Root == nil {
		return fmt.Errorf("no root filesystem specified")
	}

	// Get absolute rootfs path
	rootfs := s.Root.Path
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(bundlePath, rootfs)
	}
	rootfs, err := filepath.Abs(rootfs)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}

	// Make mount tree private to prevent propagation
	if err := makePrivate("/"); err != nil {
		// Non-fatal, might work anyway
		fmt.Printf("[rootfs] warning: make private: %v\n", err)
	}

	// Bind mount rootfs to itself (make it a mount point for pivot_root)
	if err := syscall.Mount(rootfs, rootfs, "", MS_BIND|MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount rootfs: %w", err)
	}

	// Setup mounts before pivot_root
	if err := setupMounts(s.Mounts, rootfs); err != nil {
		return fmt.Errorf("setup mounts: %w", err)
	}

	// Pivot root
	if err := pivotRoot(rootfs); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// Make rootfs readonly if specified
	if s.Root.Readonly {
		if err := syscall.Mount("", "/", "", MS_REMOUNT|MS_BIND|MS_RDONLY, ""); err != nil {
			return fmt.Errorf("remount readonly: %w", err)
		}
	}

	// Apply rootfs propagation
	if s.Linux != nil && s.Linux.RootfsPropagation != "" {
		if err := applyPropagation("/", s.Linux.RootfsPropagation); err != nil {
			fmt.Printf("[rootfs] warning: propagation: %v\n", err)
		}
	}

	// Mask paths
	if s.Linux != nil {
		for _, path := range s.Linux.MaskedPaths {
			if err := maskPath(path); err != nil {
				fmt.Printf("[rootfs] warning: mask %s: %v\n", path, err)
			}
		}
		for _, path := range s.Linux.ReadonlyPaths {
			if err := readonlyPath(path); err != nil {
				fmt.Printf("[rootfs] warning: readonly %s: %v\n", path, err)
			}
		}
	}

	return nil
}

// makePrivate makes the mount tree private.
func makePrivate(path string) error {
	return syscall.Mount("", path, "", MS_REC|MS_PRIVATE, "")
}

// pivotRoot performs pivot_root to change the root filesystem.
func pivotRoot(rootfs string) error {
	// Create directory for old root
	oldRoot := filepath.Join(rootfs, ".old_root")
	if err := os.MkdirAll(oldRoot, 0700); err != nil {
		return fmt.Errorf("mkdir old_root: %w", err)
	}

	// Pivot root
	if err := syscall.PivotRoot(rootfs, oldRoot); err != nil {
		// Try chroot fallback for rootless containers
		return chrootFallback(rootfs)
	}

	// Change to new root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}

	// Unmount old root
	oldRoot = "/.old_root"
	if err := syscall.Unmount(oldRoot, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}

	// Remove old root directory
	os.RemoveAll(oldRoot)

	return nil
}

// chrootFallback uses chroot when pivot_root fails (e.g., rootless).
func chrootFallback(rootfs string) error {
	if err := syscall.Chroot(rootfs); err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}
	return nil
}

// setupMounts performs all mounts specified in the OCI config.
func setupMounts(mounts []spec.Mount, rootfs string) error {
	for _, m := range mounts {
		dest := filepath.Join(rootfs, m.Destination)

		// Parse mount options
		flags, data := parseMountOptions(m.Options)

		// Handle special mount types
		source := m.Source
		isBind := m.Type == "bind" || hasOption(m.Options, "bind") || hasOption(m.Options, "rbind")

		if isBind {
			// Bind mount - check if source is file or directory
			if !filepath.IsAbs(source) {
				source = filepath.Join(rootfs, source)
			}

			// Stat the source to determine if it's a file or directory
			srcInfo, err := os.Stat(source)
			if err != nil {
				// Source doesn't exist, skip this mount
				fmt.Printf("[rootfs] warning: bind source %s not found: %v\n", source, err)
				continue
			}

			// Create mount point based on source type
			if srcInfo.IsDir() {
				if err := os.MkdirAll(dest, 0755); err != nil {
					return fmt.Errorf("mkdir %s: %w", dest, err)
				}
			} else {
				// Source is a file - create parent dir and empty file
				if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
					return fmt.Errorf("mkdir parent %s: %w", filepath.Dir(dest), err)
				}
				// Create empty file if it doesn't exist
				if _, err := os.Stat(dest); os.IsNotExist(err) {
					f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0644)
					if err != nil {
						return fmt.Errorf("create file %s: %w", dest, err)
					}
					f.Close()
				}
			}

			if err := syscall.Mount(source, dest, "", flags|MS_BIND, data); err != nil {
				return fmt.Errorf("bind mount %s: %w", dest, err)
			}
		} else {
			// Regular mount - create directory
			if err := os.MkdirAll(dest, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dest, err)
			}
			if err := syscall.Mount(source, dest, m.Type, flags, data); err != nil {
				// Non-fatal for optional mounts
				fmt.Printf("[rootfs] warning: mount %s (%s): %v\n", dest, m.Type, err)
			}
		}
	}
	return nil
}

// parseMountOptions parses OCI mount options into flags and data string.
func parseMountOptions(options []string) (uintptr, string) {
	var flags uintptr
	var dataOpts []string

	for _, opt := range options {
		if flag, ok := mountOptionFlags[opt]; ok {
			flags |= flag
		} else if strings.Contains(opt, "=") || !isKnownOption(opt) {
			// Data options passed to filesystem
			dataOpts = append(dataOpts, opt)
		}
	}

	return flags, strings.Join(dataOpts, ",")
}

// hasOption checks if an option is in the list.
func hasOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}

// isKnownOption checks if an option is a known mount flag.
func isKnownOption(opt string) bool {
	_, ok := mountOptionFlags[opt]
	return ok
}

// applyPropagation sets mount propagation.
func applyPropagation(path, propagation string) error {
	var flag uintptr
	switch propagation {
	case "private":
		flag = MS_PRIVATE
	case "rprivate":
		flag = MS_PRIVATE | MS_REC
	case "shared":
		flag = MS_SHARED
	case "rshared":
		flag = MS_SHARED | MS_REC
	case "slave":
		flag = MS_SLAVE
	case "rslave":
		flag = MS_SLAVE | MS_REC
	case "unbindable":
		flag = MS_UNBINDABLE
	case "runbindable":
		flag = MS_UNBINDABLE | MS_REC
	default:
		return fmt.Errorf("unknown propagation: %s", propagation)
	}
	return syscall.Mount("", path, "", flag, "")
}

// maskPath masks a path by bind-mounting /dev/null over it.
func maskPath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Check if it's a file or directory
	fi, err := os.Stat(path)
	if err != nil {
		return nil // Best effort
	}

	if fi.IsDir() {
		// Bind mount empty tmpfs
		return syscall.Mount("tmpfs", path, "tmpfs", MS_RDONLY, "size=0")
	}

	// Bind mount /dev/null
	return syscall.Mount("/dev/null", path, "", MS_BIND, "")
}

// readonlyPath makes a path read-only by remounting it.
func readonlyPath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// Bind mount to itself first
	if err := syscall.Mount(path, path, "", MS_BIND|MS_REC, ""); err != nil {
		return err
	}

	// Remount read-only
	return syscall.Mount(path, path, "", MS_BIND|MS_REMOUNT|MS_RDONLY|MS_REC, "")
}

// MountProc mounts procfs at /proc.
func MountProc() error {
	if err := os.MkdirAll("/proc", 0755); err != nil {
		return err
	}
	return syscall.Mount("proc", "/proc", "proc", MS_NOSUID|MS_NOEXEC|MS_NODEV, "")
}

// CreateDevices creates device nodes specified in the config.
func CreateDevices(devices []spec.LinuxDevice) error {
	for _, dev := range devices {
		if err := createDevice(dev); err != nil {
			return fmt.Errorf("create device %s: %w", dev.Path, err)
		}
	}
	return nil
}

// createDevice creates a single device node.
func createDevice(dev spec.LinuxDevice) error {
	// Ensure parent directory exists
	dir := filepath.Dir(dev.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Calculate device type
	var devType uint32
	switch dev.Type {
	case "c", "u":
		devType = syscall.S_IFCHR
	case "b":
		devType = syscall.S_IFBLK
	case "p":
		devType = syscall.S_IFIFO
	default:
		return fmt.Errorf("unknown device type: %s", dev.Type)
	}

	// Calculate mode
	mode := devType
	if dev.FileMode != nil {
		mode |= uint32(*dev.FileMode)
	} else {
		mode |= 0666
	}

	// Calculate device number
	devNum := int((dev.Major << 8) | dev.Minor)

	// Create device
	if err := syscall.Mknod(dev.Path, mode, devNum); err != nil {
		if !os.IsExist(err) {
			return err
		}
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
	if err := os.Chown(dev.Path, uid, gid); err != nil {
		return err
	}

	return nil
}

// SetupDefaultDevices creates the standard container device nodes.
func SetupDefaultDevices() error {
	devices := []spec.LinuxDevice{
		{Path: "/dev/null", Type: "c", Major: 1, Minor: 3},
		{Path: "/dev/zero", Type: "c", Major: 1, Minor: 5},
		{Path: "/dev/full", Type: "c", Major: 1, Minor: 7},
		{Path: "/dev/random", Type: "c", Major: 1, Minor: 8},
		{Path: "/dev/urandom", Type: "c", Major: 1, Minor: 9},
		{Path: "/dev/tty", Type: "c", Major: 5, Minor: 0},
	}

	mode := os.FileMode(0666)
	for i := range devices {
		devices[i].FileMode = &mode
	}

	return CreateDevices(devices)
}

// SetupDevSymlinks creates standard /dev symlinks.
func SetupDevSymlinks() error {
	symlinks := map[string]string{
		"/dev/fd":     "/proc/self/fd",
		"/dev/stdin":  "/proc/self/fd/0",
		"/dev/stdout": "/proc/self/fd/1",
		"/dev/stderr": "/proc/self/fd/2",
	}

	for link, target := range symlinks {
		os.Remove(link) // Remove if exists
		if err := os.Symlink(target, link); err != nil {
			fmt.Printf("[dev] warning: symlink %s: %v\n", link, err)
		}
	}

	return nil
}

// SetupDevPts mounts devpts at /dev/pts.
func SetupDevPts() error {
	if err := os.MkdirAll("/dev/pts", 0755); err != nil {
		return err
	}
	return syscall.Mount("devpts", "/dev/pts", "devpts",
		MS_NOSUID|MS_NOEXEC,
		"newinstance,ptmxmode=0666,mode=0620")
}
