// Package linux provides Linux-specific container primitives.
package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"runc-go/spec"
)

// Linux namespace clone flags
const (
	CLONE_NEWNS     = syscall.CLONE_NEWNS     // Mount namespace
	CLONE_NEWUTS    = syscall.CLONE_NEWUTS    // UTS namespace (hostname)
	CLONE_NEWIPC    = syscall.CLONE_NEWIPC    // IPC namespace
	CLONE_NEWPID    = syscall.CLONE_NEWPID    // PID namespace
	CLONE_NEWNET    = syscall.CLONE_NEWNET    // Network namespace
	CLONE_NEWUSER   = syscall.CLONE_NEWUSER   // User namespace
	CLONE_NEWCGROUP = 0x02000000              // Cgroup namespace (not in syscall pkg)
)

// namespaceTypeToFlag maps OCI namespace types to clone flags.
var namespaceTypeToFlag = map[spec.LinuxNamespaceType]uintptr{
	spec.PIDNamespace:     CLONE_NEWPID,
	spec.NetworkNamespace: CLONE_NEWNET,
	spec.MountNamespace:   CLONE_NEWNS,
	spec.IPCNamespace:     CLONE_NEWIPC,
	spec.UTSNamespace:     CLONE_NEWUTS,
	spec.UserNamespace:    CLONE_NEWUSER,
	spec.CgroupNamespace:  CLONE_NEWCGROUP,
}

// NamespaceFlags builds clone flags from OCI namespace configuration.
func NamespaceFlags(namespaces []spec.LinuxNamespace) uintptr {
	var flags uintptr
	for _, ns := range namespaces {
		// Only add flag if path is empty (create new namespace)
		// If path is set, we'll join that namespace later with setns()
		if ns.Path == "" {
			if flag, ok := namespaceTypeToFlag[ns.Type]; ok {
				flags |= flag
			}
		}
	}
	return flags
}

// HasNamespace checks if a namespace type is in the list.
func HasNamespace(namespaces []spec.LinuxNamespace, nsType spec.LinuxNamespaceType) bool {
	for _, ns := range namespaces {
		if ns.Type == nsType {
			return true
		}
	}
	return false
}

// GetNamespacePath returns the path for a namespace type, empty if creating new.
func GetNamespacePath(namespaces []spec.LinuxNamespace, nsType spec.LinuxNamespaceType) string {
	for _, ns := range namespaces {
		if ns.Type == nsType {
			return ns.Path
		}
	}
	return ""
}

// SetNamespaces joins existing namespaces specified by path.
// This is called after fork but before exec.
func SetNamespaces(namespaces []spec.LinuxNamespace) error {
	for _, ns := range namespaces {
		if ns.Path != "" {
			if err := setns(ns.Path, ns.Type); err != nil {
				return fmt.Errorf("setns %s (%s): %w", ns.Type, ns.Path, err)
			}
		}
	}
	return nil
}

// setns joins an existing namespace.
func setns(path string, nsType spec.LinuxNamespaceType) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer syscall.Close(fd)

	flag := namespaceTypeToFlag[nsType]
	// Use unix.SYS_SETNS which is architecture-independent
	_, _, errno := syscall.Syscall(unix.SYS_SETNS, uintptr(fd), flag, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// BuildSysProcAttr creates SysProcAttr from OCI spec.
func BuildSysProcAttr(s *spec.Spec) (*syscall.SysProcAttr, error) {
	if s.Linux == nil {
		// Default namespaces if not specified
		return &syscall.SysProcAttr{
			Cloneflags: CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWUTS | CLONE_NEWIPC | CLONE_NEWNET,
			Setsid:     true,
		}, nil
	}

	flags := NamespaceFlags(s.Linux.Namespaces)
	hasUserNS := HasNamespace(s.Linux.Namespaces, spec.UserNamespace)

	attr := &syscall.SysProcAttr{
		Cloneflags: flags,
		Setsid:     true,
	}

	// Don't set Unshareflags with user namespace - causes EPERM
	if !hasUserNS {
		attr.Unshareflags = syscall.CLONE_NEWNS
	}

	// Setup UID/GID mappings for user namespace
	if hasUserNS {
		attr.UidMappings = buildIDMappings(s.Linux.UIDMappings)
		attr.GidMappings = buildIDMappings(s.Linux.GIDMappings)
		attr.GidMappingsEnableSetgroups = false
	}

	return attr, nil
}

// buildIDMappings converts OCI ID mappings to syscall format.
func buildIDMappings(mappings []spec.LinuxIDMapping) []syscall.SysProcIDMap {
	result := make([]syscall.SysProcIDMap, len(mappings))
	for i, m := range mappings {
		result[i] = syscall.SysProcIDMap{
			ContainerID: int(m.ContainerID),
			HostID:      int(m.HostID),
			Size:        int(m.Size),
		}
	}
	return result
}

// WriteIDMappings writes UID/GID mappings to /proc/pid/{uid,gid}_map.
// Used when setting up user namespaces externally.
func WriteIDMappings(pid int, uidMappings, gidMappings []spec.LinuxIDMapping) error {
	// Write uid_map
	if len(uidMappings) > 0 {
		path := filepath.Join("/proc", fmt.Sprint(pid), "uid_map")
		content := formatIDMap(uidMappings)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write uid_map: %w", err)
		}
	}

	// Must disable setgroups before writing gid_map (unless we have CAP_SETGID)
	if len(gidMappings) > 0 {
		setgroupsPath := filepath.Join("/proc", fmt.Sprint(pid), "setgroups")
		if err := os.WriteFile(setgroupsPath, []byte("deny"), 0644); err != nil {
			// Best effort - might not exist on older kernels
		}

		path := filepath.Join("/proc", fmt.Sprint(pid), "gid_map")
		content := formatIDMap(gidMappings)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write gid_map: %w", err)
		}
	}

	return nil
}

// formatIDMap formats ID mappings for /proc/pid/{uid,gid}_map.
func formatIDMap(mappings []spec.LinuxIDMapping) string {
	var result string
	for _, m := range mappings {
		result += fmt.Sprintf("%d %d %d\n", m.ContainerID, m.HostID, m.Size)
	}
	return result
}

// SetHostname sets the hostname in the UTS namespace.
func SetHostname(hostname string) error {
	if hostname == "" {
		return nil
	}
	return syscall.Sethostname([]byte(hostname))
}

// SetDomainname sets the domain name in the UTS namespace.
func SetDomainname(domainname string) error {
	if domainname == "" {
		return nil
	}
	return syscall.Setdomainname([]byte(domainname))
}
