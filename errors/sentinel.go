// Package errors provides predefined sentinel errors for common failure cases.
package errors

// Container lifecycle errors.
var (
	// ErrContainerNotFound indicates the container does not exist.
	ErrContainerNotFound = &ContainerError{
		Kind:   ErrNotFound,
		Detail: "container not found",
	}

	// ErrContainerExists indicates the container already exists.
	ErrContainerExists = &ContainerError{
		Kind:   ErrAlreadyExists,
		Detail: "container already exists",
	}

	// ErrContainerNotRunning indicates the container is not in running state.
	ErrContainerNotRunning = &ContainerError{
		Kind:   ErrInvalidState,
		Detail: "container is not running",
	}

	// ErrContainerNotStopped indicates the container is not in stopped state.
	ErrContainerNotStopped = &ContainerError{
		Kind:   ErrInvalidState,
		Detail: "container is not stopped",
	}

	// ErrContainerNotCreated indicates the container is not in created state.
	ErrContainerNotCreated = &ContainerError{
		Kind:   ErrInvalidState,
		Detail: "container is not in created state",
	}

	// ErrInvalidContainerID indicates the container ID is invalid.
	ErrInvalidContainerID = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "invalid container ID",
	}

	// ErrEmptyContainerID indicates the container ID is empty.
	ErrEmptyContainerID = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "container ID cannot be empty",
	}

	// ErrNoInitProcess indicates there is no init process.
	ErrNoInitProcess = &ContainerError{
		Kind:   ErrInvalidState,
		Detail: "no init process",
	}
)

// Configuration and validation errors.
var (
	// ErrInvalidBundlePath indicates the bundle path is invalid.
	ErrInvalidBundlePath = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "invalid bundle path",
	}

	// ErrMissingSpec indicates the config.json is missing.
	ErrMissingSpec = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "config.json not found",
	}

	// ErrInvalidSpec indicates the spec is invalid.
	ErrInvalidSpec = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "invalid OCI spec",
	}

	// ErrMissingRootfs indicates the rootfs is missing.
	ErrMissingRootfs = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "rootfs not found",
	}

	// ErrNoProcessArgs indicates no process arguments were specified.
	ErrNoProcessArgs = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "no process arguments specified",
	}
)

// Security-related errors.
var (
	// ErrPathTraversal indicates a path traversal attempt was detected.
	ErrPathTraversal = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "path traversal detected",
	}

	// ErrSeccompFilter indicates a seccomp filter error.
	ErrSeccompFilter = &ContainerError{
		Kind:   ErrSeccomp,
		Detail: "failed to apply seccomp filter",
	}

	// ErrCapabilityDrop indicates a capability drop error.
	ErrCapabilityDrop = &ContainerError{
		Kind:   ErrCapability,
		Detail: "failed to drop capabilities",
	}

	// ErrCapabilityUnknown indicates an unknown capability was specified.
	ErrCapabilityUnknown = &ContainerError{
		Kind:   ErrCapability,
		Detail: "unknown capability",
	}
)

// Namespace errors.
var (
	// ErrNamespaceSetup indicates a namespace setup error.
	ErrNamespaceSetup = &ContainerError{
		Kind:   ErrNamespace,
		Detail: "failed to setup namespace",
	}

	// ErrNamespaceJoin indicates a namespace join error.
	ErrNamespaceJoin = &ContainerError{
		Kind:   ErrNamespace,
		Detail: "failed to join namespace",
	}
)

// Cgroup errors.
var (
	// ErrCgroupSetup indicates a cgroup setup error.
	ErrCgroupSetup = &ContainerError{
		Kind:   ErrCgroup,
		Detail: "failed to setup cgroup",
	}

	// ErrCgroupNotFound indicates the cgroup was not found.
	ErrCgroupNotFound = &ContainerError{
		Kind:   ErrCgroup,
		Detail: "cgroup not found",
	}

	// ErrCgroupResource indicates a cgroup resource limit error.
	ErrCgroupResource = &ContainerError{
		Kind:   ErrCgroup,
		Detail: "failed to apply resource limits",
	}
)

// Device errors.
var (
	// ErrDeviceCreate indicates a device creation error.
	ErrDeviceCreate = &ContainerError{
		Kind:   ErrDevice,
		Detail: "failed to create device",
	}

	// ErrDeviceNotAllowed indicates a device is not in the whitelist.
	ErrDeviceNotAllowed = &ContainerError{
		Kind:   ErrDevice,
		Detail: "device not allowed",
	}

	// ErrInvalidDevicePath indicates an invalid device path.
	ErrInvalidDevicePath = &ContainerError{
		Kind:   ErrDevice,
		Detail: "invalid device path",
	}
)

// Rootfs errors.
var (
	// ErrRootfsSetup indicates a rootfs setup error.
	ErrRootfsSetup = &ContainerError{
		Kind:   ErrRootfs,
		Detail: "failed to setup rootfs",
	}

	// ErrPivotRoot indicates a pivot_root error.
	ErrPivotRoot = &ContainerError{
		Kind:   ErrRootfs,
		Detail: "failed to pivot_root",
	}

	// ErrMountFailed indicates a mount error.
	ErrMountFailed = &ContainerError{
		Kind:   ErrRootfs,
		Detail: "failed to mount",
	}
)

// Console/PTY errors.
var (
	// ErrConsoleSetup indicates a console setup error.
	ErrConsoleSetup = &ContainerError{
		Kind:   ErrResource,
		Detail: "failed to setup console",
	}

	// ErrInvalidSocketPath indicates an invalid socket path.
	ErrInvalidSocketPath = &ContainerError{
		Kind:   ErrInvalidConfig,
		Detail: "invalid socket path",
	}
)

// Process errors.
var (
	// ErrProcessStart indicates a process start error.
	ErrProcessStart = &ContainerError{
		Kind:   ErrInternal,
		Detail: "failed to start process",
	}

	// ErrProcessNotFound indicates the process was not found.
	ErrProcessNotFound = &ContainerError{
		Kind:   ErrNotFound,
		Detail: "process not found",
	}

	// ErrSignalFailed indicates a signal delivery error.
	ErrSignalFailed = &ContainerError{
		Kind:   ErrInternal,
		Detail: "failed to send signal",
	}
)
