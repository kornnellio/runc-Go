// Package spec defines the OCI Runtime Specification structures.
// These types represent the config.json format defined by the OCI Runtime Spec.
// Reference: https://github.com/opencontainers/runtime-spec/blob/main/config.md
package spec

import (
	"encoding/json"
	"os"
)

// Version is the OCI Runtime Specification version this implementation targets.
const Version = "1.0.2"

// Spec is the base configuration for the container.
type Spec struct {
	// Version is the OCI Runtime Specification version.
	Version string `json:"ociVersion"`

	// Process configures the container process.
	Process *Process `json:"process,omitempty"`

	// Root configures the container's root filesystem.
	Root *Root `json:"root,omitempty"`

	// Hostname configures the container's hostname.
	Hostname string `json:"hostname,omitempty"`

	// Domainname configures the container's domainname.
	Domainname string `json:"domainname,omitempty"`

	// Mounts configures additional mounts (on top of Root).
	Mounts []Mount `json:"mounts,omitempty"`

	// Hooks configures callbacks for container lifecycle events.
	Hooks *Hooks `json:"hooks,omitempty"`

	// Annotations contains arbitrary metadata for the container.
	Annotations map[string]string `json:"annotations,omitempty"`

	// Linux is platform-specific configuration for Linux based containers.
	Linux *Linux `json:"linux,omitempty"`
}

// Process contains information to start a specific application inside the container.
type Process struct {
	// Terminal creates an interactive terminal for the container.
	Terminal bool `json:"terminal,omitempty"`

	// ConsoleSize specifies the size of the console.
	ConsoleSize *Box `json:"consoleSize,omitempty"`

	// User specifies user information for the process.
	User User `json:"user"`

	// Args specifies the binary and arguments for the application to execute.
	Args []string `json:"args,omitempty"`

	// CommandLine specifies the full command line for the application (Windows).
	CommandLine string `json:"commandLine,omitempty"`

	// Env populates the process environment for the process.
	Env []string `json:"env,omitempty"`

	// Cwd is the current working directory for the process.
	Cwd string `json:"cwd"`

	// Capabilities are Linux capabilities that are kept for the process.
	Capabilities *LinuxCapabilities `json:"capabilities,omitempty"`

	// Rlimits specifies rlimit options to apply to the process.
	Rlimits []POSIXRlimit `json:"rlimits,omitempty"`

	// NoNewPrivileges controls whether additional privileges could be gained by processes.
	NoNewPrivileges bool `json:"noNewPrivileges,omitempty"`

	// ApparmorProfile specifies the apparmor profile for the container.
	ApparmorProfile string `json:"apparmorProfile,omitempty"`

	// OOMScoreAdj specifies the OOM killer score adjustment.
	OOMScoreAdj *int `json:"oomScoreAdj,omitempty"`

	// SelinuxLabel specifies the selinux context for the process.
	SelinuxLabel string `json:"selinuxLabel,omitempty"`
}

// Box specifies dimensions of a rectangle (for console size).
type Box struct {
	Height uint `json:"height"`
	Width  uint `json:"width"`
}

// User specifies specific user (and group) information for the container process.
type User struct {
	// UID is the user id.
	UID uint32 `json:"uid"`

	// GID is the group id.
	GID uint32 `json:"gid"`

	// Umask is the umask for the init process.
	Umask *uint32 `json:"umask,omitempty"`

	// AdditionalGids are additional group ids set for the container's process.
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`

	// Username is the user name (Windows).
	Username string `json:"username,omitempty"`
}

// LinuxCapabilities specifies the capabilities to keep for the container process.
type LinuxCapabilities struct {
	// Bounding is the set of capabilities checked by the kernel.
	Bounding []string `json:"bounding,omitempty"`

	// Effective is the set of capabilities checked by the kernel for permission checks.
	Effective []string `json:"effective,omitempty"`

	// Inheritable is the set of capabilities preserved across an execve.
	Inheritable []string `json:"inheritable,omitempty"`

	// Permitted is the limiting superset for the effective capabilities.
	Permitted []string `json:"permitted,omitempty"`

	// Ambient is the set of capabilities that are preserved across execve for unprivileged programs.
	Ambient []string `json:"ambient,omitempty"`
}

// POSIXRlimit type and target details for rlimit.
type POSIXRlimit struct {
	// Type is the type of the rlimit (e.g., RLIMIT_NOFILE).
	Type string `json:"type"`

	// Hard is the hard limit for the specified type.
	Hard uint64 `json:"hard"`

	// Soft is the soft limit for the specified type.
	Soft uint64 `json:"soft"`
}

// Root contains information about the container's root filesystem.
type Root struct {
	// Path is the path to the root filesystem.
	Path string `json:"path"`

	// Readonly makes the root filesystem for the container readonly before the process is executed.
	Readonly bool `json:"readonly,omitempty"`
}

// Mount specifies a mount for a container.
type Mount struct {
	// Destination is the path inside the container.
	Destination string `json:"destination"`

	// Type specifies the mount type.
	Type string `json:"type,omitempty"`

	// Source specifies the source path of the mount.
	Source string `json:"source,omitempty"`

	// Options are fstab-style mount options.
	Options []string `json:"options,omitempty"`

	// UIDMappings specifies the user mappings for the mount's user namespace.
	UIDMappings []LinuxIDMapping `json:"uidMappings,omitempty"`

	// GIDMappings specifies the group mappings for the mount's user namespace.
	GIDMappings []LinuxIDMapping `json:"gidMappings,omitempty"`
}

// Hook specifies a command that is run at a particular event in the container lifecycle.
type Hook struct {
	// Path to the executable.
	Path string `json:"path"`

	// Arguments including the path itself.
	Args []string `json:"args,omitempty"`

	// Additional environment variables.
	Env []string `json:"env,omitempty"`

	// Timeout is the number of seconds before aborting the hook.
	Timeout *int `json:"timeout,omitempty"`
}

// Hooks specifies hooks for container lifecycle events.
type Hooks struct {
	// Prestart is DEPRECATED. Use CreateRuntime, CreateContainer, and StartContainer instead.
	Prestart []Hook `json:"prestart,omitempty"`

	// CreateRuntime is called during create after the runtime environment is created
	// but before pivot_root or any equivalent operation has been called.
	CreateRuntime []Hook `json:"createRuntime,omitempty"`

	// CreateContainer is called during create after pivot_root or equivalent
	// has been called, but before the user-specified process is executed.
	CreateContainer []Hook `json:"createContainer,omitempty"`

	// StartContainer is called during start before the user-specified process is executed.
	StartContainer []Hook `json:"startContainer,omitempty"`

	// Poststart is called after the user-specified process is executed
	// but before the start operation returns.
	Poststart []Hook `json:"poststart,omitempty"`

	// Poststop is called after the container is deleted but before delete returns.
	Poststop []Hook `json:"poststop,omitempty"`
}

// Linux contains platform-specific configuration for Linux based containers.
type Linux struct {
	// UIDMappings specifies user mappings for user namespaces.
	UIDMappings []LinuxIDMapping `json:"uidMappings,omitempty"`

	// GIDMappings specifies group mappings for user namespaces.
	GIDMappings []LinuxIDMapping `json:"gidMappings,omitempty"`

	// Sysctl are kernel parameters to set in the container.
	Sysctl map[string]string `json:"sysctl,omitempty"`

	// Resources contains cgroup resource restrictions.
	Resources *LinuxResources `json:"resources,omitempty"`

	// CgroupsPath specifies the path to cgroups that are created/joined by the container.
	CgroupsPath string `json:"cgroupsPath,omitempty"`

	// Namespaces contains the namespaces that are created/joined by the container.
	Namespaces []LinuxNamespace `json:"namespaces,omitempty"`

	// Devices are a list of device nodes to create in the container.
	Devices []LinuxDevice `json:"devices,omitempty"`

	// Seccomp specifies the seccomp security settings for the container.
	Seccomp *LinuxSeccomp `json:"seccomp,omitempty"`

	// RootfsPropagation is the rootfs mount propagation mode.
	RootfsPropagation string `json:"rootfsPropagation,omitempty"`

	// MaskedPaths masks over the provided paths inside the container.
	MaskedPaths []string `json:"maskedPaths,omitempty"`

	// ReadonlyPaths sets the provided paths as readonly inside the container.
	ReadonlyPaths []string `json:"readonlyPaths,omitempty"`

	// MountLabel specifies the selinux mount label for the container's filesystem.
	MountLabel string `json:"mountLabel,omitempty"`

	// IntelRdt contains Intel Resource Director Technology (RDT) information.
	IntelRdt *LinuxIntelRdt `json:"intelRdt,omitempty"`

	// Personality contains configuration for the Linux personality syscall.
	Personality *LinuxPersonality `json:"personality,omitempty"`
}

// LinuxIDMapping specifies UID/GID mappings.
type LinuxIDMapping struct {
	// ContainerID is the starting uid/gid in the container.
	ContainerID uint32 `json:"containerID"`

	// HostID is the starting uid/gid on the host to be mapped to containerID.
	HostID uint32 `json:"hostID"`

	// Size is the number of ids to be mapped.
	Size uint32 `json:"size"`
}

// LinuxNamespace is the configuration for a Linux namespace.
type LinuxNamespace struct {
	// Type is the type of namespace.
	Type LinuxNamespaceType `json:"type"`

	// Path is a path to an existing namespace to join.
	Path string `json:"path,omitempty"`
}

// LinuxNamespaceType is one of the Linux namespaces.
type LinuxNamespaceType string

// Namespace types
const (
	PIDNamespace     LinuxNamespaceType = "pid"
	NetworkNamespace LinuxNamespaceType = "network"
	MountNamespace   LinuxNamespaceType = "mount"
	IPCNamespace     LinuxNamespaceType = "ipc"
	UTSNamespace     LinuxNamespaceType = "uts"
	UserNamespace    LinuxNamespaceType = "user"
	CgroupNamespace  LinuxNamespaceType = "cgroup"
	TimeNamespace    LinuxNamespaceType = "time"
)

// LinuxDevice represents a device node.
type LinuxDevice struct {
	// Path to the device.
	Path string `json:"path"`

	// Type is the device type, block, char, etc.
	Type string `json:"type"`

	// Major is the device's major number.
	Major int64 `json:"major"`

	// Minor is the device's minor number.
	Minor int64 `json:"minor"`

	// FileMode permission bits for the device.
	FileMode *os.FileMode `json:"fileMode,omitempty"`

	// UID of the device.
	UID *uint32 `json:"uid,omitempty"`

	// GID of the device.
	GID *uint32 `json:"gid,omitempty"`
}

// LinuxResources has container runtime resource constraints.
type LinuxResources struct {
	// Devices configures the device allowlist.
	Devices []LinuxDeviceCgroup `json:"devices,omitempty"`

	// Memory restriction configuration.
	Memory *LinuxMemory `json:"memory,omitempty"`

	// CPU resource restriction configuration.
	CPU *LinuxCPU `json:"cpu,omitempty"`

	// Pids restricts the number of pids.
	Pids *LinuxPids `json:"pids,omitempty"`

	// BlockIO restriction configuration.
	BlockIO *LinuxBlockIO `json:"blockIO,omitempty"`

	// HugepageLimits are a list of limits on the size and number of hugepages.
	HugepageLimits []LinuxHugepageLimit `json:"hugepageLimits,omitempty"`

	// Network restriction configuration.
	Network *LinuxNetwork `json:"network,omitempty"`

	// Rdma resource restriction configuration.
	Rdma map[string]LinuxRdma `json:"rdma,omitempty"`

	// Unified contains values for unified cgroup v2 controllers.
	Unified map[string]string `json:"unified,omitempty"`
}

// LinuxDeviceCgroup represents a device rule for the device cgroup controller.
type LinuxDeviceCgroup struct {
	// Allow or deny.
	Allow bool `json:"allow"`

	// Type is the device type: c, b, or a (all).
	Type string `json:"type,omitempty"`

	// Major is the device's major number.
	Major *int64 `json:"major,omitempty"`

	// Minor is the device's minor number.
	Minor *int64 `json:"minor,omitempty"`

	// Access is a combination of r (read), w (write), and m (mknod).
	Access string `json:"access,omitempty"`
}

// LinuxMemory for Linux cgroup 'memory' resource management.
type LinuxMemory struct {
	// Limit is the memory limit in bytes.
	Limit *int64 `json:"limit,omitempty"`

	// Reservation is the soft limit in bytes.
	Reservation *int64 `json:"reservation,omitempty"`

	// Swap is memory+swap limit in bytes.
	Swap *int64 `json:"swap,omitempty"`

	// Kernel is the hard limit for kernel memory in bytes.
	Kernel *int64 `json:"kernel,omitempty"`

	// KernelTCP is the hard limit for kernel TCP buffer memory in bytes.
	KernelTCP *int64 `json:"kernelTCP,omitempty"`

	// Swappiness is the swappiness value (0-100).
	Swappiness *uint64 `json:"swappiness,omitempty"`

	// DisableOOMKiller disables the OOM killer for out of memory conditions.
	DisableOOMKiller *bool `json:"disableOOMKiller,omitempty"`

	// UseHierarchy enables memory.use_hierarchy.
	UseHierarchy *bool `json:"useHierarchy,omitempty"`

	// CheckBeforeUpdate checks memory limits before setting, if true.
	CheckBeforeUpdate *bool `json:"checkBeforeUpdate,omitempty"`
}

// LinuxCPU for Linux cgroup 'cpu' resource management.
type LinuxCPU struct {
	// Shares is the CPU shares (relative weight).
	Shares *uint64 `json:"shares,omitempty"`

	// Quota is the CPU hardcap limit (in usecs). 0 means no limit.
	Quota *int64 `json:"quota,omitempty"`

	// Burst is the CPU burst limit.
	Burst *uint64 `json:"burst,omitempty"`

	// Period is the CPU period to be used in usecs.
	Period *uint64 `json:"period,omitempty"`

	// RealtimeRuntime is the allowable realtime runtime in usecs.
	RealtimeRuntime *int64 `json:"realtimeRuntime,omitempty"`

	// RealtimePeriod is the realtime period in usecs.
	RealtimePeriod *uint64 `json:"realtimePeriod,omitempty"`

	// Cpus is the list of CPUs the container will run on (comma-separated list or ranges).
	Cpus string `json:"cpus,omitempty"`

	// Mems is the list of memory nodes the container will run on (comma-separated list or ranges).
	Mems string `json:"mems,omitempty"`

	// Idle cgroup idleness value.
	Idle *int64 `json:"idle,omitempty"`
}

// LinuxPids for Linux cgroup 'pids' resource management.
type LinuxPids struct {
	// Limit is the maximum number of PIDs.
	Limit int64 `json:"limit"`
}

// LinuxBlockIO for Linux cgroup 'blkio' resource management.
type LinuxBlockIO struct {
	// Weight is the per cgroup weight.
	Weight *uint16 `json:"weight,omitempty"`

	// LeafWeight is the cgroup leaf weight.
	LeafWeight *uint16 `json:"leafWeight,omitempty"`

	// WeightDevice specifies per device weight.
	WeightDevice []LinuxWeightDevice `json:"weightDevice,omitempty"`

	// ThrottleReadBpsDevice specifies per device read rate limits in bytes per second.
	ThrottleReadBpsDevice []LinuxThrottleDevice `json:"throttleReadBpsDevice,omitempty"`

	// ThrottleWriteBpsDevice specifies per device write rate limits in bytes per second.
	ThrottleWriteBpsDevice []LinuxThrottleDevice `json:"throttleWriteBpsDevice,omitempty"`

	// ThrottleReadIOPSDevice specifies per device read rate limits in IOPS.
	ThrottleReadIOPSDevice []LinuxThrottleDevice `json:"throttleReadIOPSDevice,omitempty"`

	// ThrottleWriteIOPSDevice specifies per device write rate limits in IOPS.
	ThrottleWriteIOPSDevice []LinuxThrottleDevice `json:"throttleWriteIOPSDevice,omitempty"`
}

// LinuxWeightDevice specifies per device weight for blkio cgroup.
type LinuxWeightDevice struct {
	// Major is the device's major number.
	Major int64 `json:"major"`

	// Minor is the device's minor number.
	Minor int64 `json:"minor"`

	// Weight is the bandwidth rate weight for the device.
	Weight *uint16 `json:"weight,omitempty"`

	// LeafWeight is the bandwidth rate leaf weight for the device.
	LeafWeight *uint16 `json:"leafWeight,omitempty"`
}

// LinuxThrottleDevice specifies per device throttle limits.
type LinuxThrottleDevice struct {
	// Major is the device's major number.
	Major int64 `json:"major"`

	// Minor is the device's minor number.
	Minor int64 `json:"minor"`

	// Rate is the IO rate limit per second.
	Rate uint64 `json:"rate"`
}

// LinuxHugepageLimit specifies hugepage limits.
type LinuxHugepageLimit struct {
	// Pagesize is the hugepage size (e.g., "2MB", "1GB").
	Pagesize string `json:"pageSize"`

	// Limit is the limit of allocatable bytes for hugepages.
	Limit uint64 `json:"limit"`
}

// LinuxNetwork contains network cgroup limits.
type LinuxNetwork struct {
	// ClassID is the class identifier for container's network packets.
	ClassID *uint32 `json:"classID,omitempty"`

	// Priorities specifies per interface priorities.
	Priorities []LinuxInterfacePriority `json:"priorities,omitempty"`
}

// LinuxInterfacePriority specifies per interface priority.
type LinuxInterfacePriority struct {
	// Name is the interface name.
	Name string `json:"name"`

	// Priority is the priority applied to the interface.
	Priority uint32 `json:"priority"`
}

// LinuxRdma contains RDMA cgroup limits.
type LinuxRdma struct {
	// HcaHandles is the number of HCA handles.
	HcaHandles *uint32 `json:"hcaHandles,omitempty"`

	// HcaObjects is the number of HCA objects.
	HcaObjects *uint32 `json:"hcaObjects,omitempty"`
}

// LinuxSeccomp represents syscall filtering configuration.
type LinuxSeccomp struct {
	// DefaultAction is the default action when no rules match.
	DefaultAction LinuxSeccompAction `json:"defaultAction"`

	// Architectures specifies the architectures this configuration applies to.
	Architectures []Arch `json:"architectures,omitempty"`

	// Flags are seccomp flags (e.g., SECCOMP_FILTER_FLAG_LOG).
	Flags []LinuxSeccompFlag `json:"flags,omitempty"`

	// ListenerPath is a path to a socket to receive seccomp notifications.
	ListenerPath string `json:"listenerPath,omitempty"`

	// ListenerMetadata is opaque data to pass to the seccomp agent.
	ListenerMetadata string `json:"listenerMetadata,omitempty"`

	// Syscalls specifies syscall filtering rules.
	Syscalls []LinuxSyscall `json:"syscalls,omitempty"`
}

// LinuxSeccompAction is the action to take when a syscall matches.
type LinuxSeccompAction string

// Seccomp actions
const (
	ActKill         LinuxSeccompAction = "SCMP_ACT_KILL"
	ActKillProcess  LinuxSeccompAction = "SCMP_ACT_KILL_PROCESS"
	ActKillThread   LinuxSeccompAction = "SCMP_ACT_KILL_THREAD"
	ActTrap         LinuxSeccompAction = "SCMP_ACT_TRAP"
	ActErrno        LinuxSeccompAction = "SCMP_ACT_ERRNO"
	ActTrace        LinuxSeccompAction = "SCMP_ACT_TRACE"
	ActAllow        LinuxSeccompAction = "SCMP_ACT_ALLOW"
	ActLog          LinuxSeccompAction = "SCMP_ACT_LOG"
	ActNotify       LinuxSeccompAction = "SCMP_ACT_NOTIFY"
)

// Arch is the architecture type.
type Arch string

// Architecture types
const (
	ArchX86         Arch = "SCMP_ARCH_X86"
	ArchX86_64      Arch = "SCMP_ARCH_X86_64"
	ArchX32         Arch = "SCMP_ARCH_X32"
	ArchARM         Arch = "SCMP_ARCH_ARM"
	ArchAARCH64     Arch = "SCMP_ARCH_AARCH64"
	ArchMIPS        Arch = "SCMP_ARCH_MIPS"
	ArchMIPS64      Arch = "SCMP_ARCH_MIPS64"
	ArchMIPS64N32   Arch = "SCMP_ARCH_MIPS64N32"
	ArchMIPSEL      Arch = "SCMP_ARCH_MIPSEL"
	ArchMIPSEL64    Arch = "SCMP_ARCH_MIPSEL64"
	ArchMIPSEL64N32 Arch = "SCMP_ARCH_MIPSEL64N32"
	ArchPPC         Arch = "SCMP_ARCH_PPC"
	ArchPPC64       Arch = "SCMP_ARCH_PPC64"
	ArchPPC64LE     Arch = "SCMP_ARCH_PPC64LE"
	ArchS390        Arch = "SCMP_ARCH_S390"
	ArchS390X       Arch = "SCMP_ARCH_S390X"
	ArchRISCV64     Arch = "SCMP_ARCH_RISCV64"
)

// LinuxSeccompFlag is a flag for seccomp.
type LinuxSeccompFlag string

// Seccomp flags
const (
	SeccompFlagLog        LinuxSeccompFlag = "SECCOMP_FILTER_FLAG_LOG"
	SeccompFlagSpecAllow  LinuxSeccompFlag = "SECCOMP_FILTER_FLAG_SPEC_ALLOW"
	SeccompFlagWaitKill   LinuxSeccompFlag = "SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV"
)

// LinuxSyscall specifies a syscall filter rule.
type LinuxSyscall struct {
	// Names specifies the names of the syscalls.
	Names []string `json:"names"`

	// Action is the action to take when the syscall is matched.
	Action LinuxSeccompAction `json:"action"`

	// ErrnoRet is the errno return value when action is SCMP_ACT_ERRNO.
	ErrnoRet *uint `json:"errnoRet,omitempty"`

	// Args specifies conditions on syscall arguments.
	Args []LinuxSeccompArg `json:"args,omitempty"`
}

// LinuxSeccompArg specifies a condition on a syscall argument.
type LinuxSeccompArg struct {
	// Index is the argument index (0-5).
	Index uint `json:"index"`

	// Value is the value to compare against.
	Value uint64 `json:"value"`

	// ValueTwo is the second value for range comparisons.
	ValueTwo uint64 `json:"valueTwo,omitempty"`

	// Op is the comparison operator.
	Op LinuxSeccompOperator `json:"op"`
}

// LinuxSeccompOperator is the comparison operator for seccomp argument checks.
type LinuxSeccompOperator string

// Seccomp operators
const (
	OpNotEqual     LinuxSeccompOperator = "SCMP_CMP_NE"
	OpLessThan     LinuxSeccompOperator = "SCMP_CMP_LT"
	OpLessEqual    LinuxSeccompOperator = "SCMP_CMP_LE"
	OpEqualTo      LinuxSeccompOperator = "SCMP_CMP_EQ"
	OpGreaterEqual LinuxSeccompOperator = "SCMP_CMP_GE"
	OpGreaterThan  LinuxSeccompOperator = "SCMP_CMP_GT"
	OpMaskedEqual  LinuxSeccompOperator = "SCMP_CMP_MASKED_EQ"
)

// LinuxIntelRdt contains Intel Resource Director Technology (RDT) information.
type LinuxIntelRdt struct {
	// ClosID is the identity of the RDT Class of Service.
	ClosID string `json:"closID,omitempty"`

	// L3CacheSchema specifies the L3 cache schema.
	L3CacheSchema string `json:"l3CacheSchema,omitempty"`

	// MemBwSchema specifies the memory bandwidth schema.
	MemBwSchema string `json:"memBwSchema,omitempty"`

	// EnableCMT enables Cache Monitoring Technology.
	EnableCMT bool `json:"enableCMT,omitempty"`

	// EnableMBM enables Memory Bandwidth Monitoring.
	EnableMBM bool `json:"enableMBM,omitempty"`
}

// LinuxPersonality specifies the Linux personality to set.
type LinuxPersonality struct {
	// Domain is the execution domain.
	Domain LinuxPersonalityDomain `json:"domain"`

	// Flags are additional flags to modify personality behavior.
	Flags []LinuxPersonalityFlag `json:"flags,omitempty"`
}

// LinuxPersonalityDomain is the execution domain.
type LinuxPersonalityDomain string

// Personality domains
const (
	PerLinux   LinuxPersonalityDomain = "LINUX"
	PerLinux32 LinuxPersonalityDomain = "LINUX32"
)

// LinuxPersonalityFlag is a flag for personality.
type LinuxPersonalityFlag string

// LoadSpec loads an OCI spec from a config.json file.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// SaveSpec saves an OCI spec to a file.
func (s *Spec) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// DefaultSpec returns a minimal default OCI spec suitable for most containers.
func DefaultSpec() *Spec {
	return &Spec{
		Version: Version,
		Root: &Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Process: &Process{
			Terminal: true,
			User: User{
				UID: 0,
				GID: 0,
			},
			Args: []string{"/bin/sh"},
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"TERM=xterm",
			},
			Cwd:             "/",
			NoNewPrivileges: true,
			Capabilities: &LinuxCapabilities{
				Bounding: defaultCapabilities(),
				Effective: defaultCapabilities(),
				Permitted: defaultCapabilities(),
			},
			Rlimits: []POSIXRlimit{
				{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
			},
		},
		Hostname: "container",
		Mounts: []Mount{
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			},
			{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			},
		},
		Linux: &Linux{
			Resources: &LinuxResources{
				Devices: []LinuxDeviceCgroup{
					{Allow: false, Access: "rwm"}, // Deny all by default
					{Allow: true, Type: "c", Major: intPtr(1), Minor: intPtr(3), Access: "rwm"},  // /dev/null
					{Allow: true, Type: "c", Major: intPtr(1), Minor: intPtr(5), Access: "rwm"},  // /dev/zero
					{Allow: true, Type: "c", Major: intPtr(1), Minor: intPtr(7), Access: "rwm"},  // /dev/full
					{Allow: true, Type: "c", Major: intPtr(1), Minor: intPtr(8), Access: "rwm"},  // /dev/random
					{Allow: true, Type: "c", Major: intPtr(1), Minor: intPtr(9), Access: "rwm"},  // /dev/urandom
					{Allow: true, Type: "c", Major: intPtr(5), Minor: intPtr(0), Access: "rwm"},  // /dev/tty
					{Allow: true, Type: "c", Major: intPtr(5), Minor: intPtr(1), Access: "rwm"},  // /dev/console
					{Allow: true, Type: "c", Major: intPtr(5), Minor: intPtr(2), Access: "rwm"},  // /dev/ptmx
					{Allow: true, Type: "c", Major: intPtr(136), Minor: nil, Access: "rwm"},       // /dev/pts/*
				},
			},
			Namespaces: []LinuxNamespace{
				{Type: PIDNamespace},
				{Type: NetworkNamespace},
				{Type: IPCNamespace},
				{Type: UTSNamespace},
				{Type: MountNamespace},
			},
			MaskedPaths: []string{
				"/proc/acpi",
				"/proc/asound",
				"/proc/kcore",
				"/proc/keys",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_debug",
				"/proc/scsi",
				"/sys/firmware",
			},
			ReadonlyPaths: []string{
				"/proc/bus",
				"/proc/fs",
				"/proc/irq",
				"/proc/sys",
				"/proc/sysrq-trigger",
			},
		},
	}
}

// defaultCapabilities returns the default capability set.
func defaultCapabilities() []string {
	return []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_FSETID",
		"CAP_FOWNER",
		"CAP_MKNOD",
		"CAP_NET_RAW",
		"CAP_SETGID",
		"CAP_SETUID",
		"CAP_SETFCAP",
		"CAP_SETPCAP",
		"CAP_NET_BIND_SERVICE",
		"CAP_SYS_CHROOT",
		"CAP_KILL",
		"CAP_AUDIT_WRITE",
	}
}

// intPtr returns a pointer to an int64.
func intPtr(i int64) *int64 {
	return &i
}
