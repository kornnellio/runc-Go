// Package errors provides typed error handling for the runc-go container runtime.
//
// This package defines domain-specific error types that enable better error
// classification, debugging, and user feedback. All errors support the standard
// errors.Is() and errors.As() functions for error inspection.
package errors

import (
	"errors"
	"fmt"
)

// ErrorKind represents the category of an error.
type ErrorKind int

const (
	// ErrNotFound indicates a resource was not found.
	ErrNotFound ErrorKind = iota
	// ErrAlreadyExists indicates a resource already exists.
	ErrAlreadyExists
	// ErrInvalidState indicates an operation was attempted in an invalid state.
	ErrInvalidState
	// ErrInvalidConfig indicates a configuration error.
	ErrInvalidConfig
	// ErrPermission indicates a permission error.
	ErrPermission
	// ErrResource indicates a resource allocation or access error.
	ErrResource
	// ErrNamespace indicates a namespace operation error.
	ErrNamespace
	// ErrCgroup indicates a cgroup operation error.
	ErrCgroup
	// ErrSeccomp indicates a seccomp filter error.
	ErrSeccomp
	// ErrCapability indicates a capability operation error.
	ErrCapability
	// ErrDevice indicates a device operation error.
	ErrDevice
	// ErrRootfs indicates a rootfs setup error.
	ErrRootfs
	// ErrInternal indicates an internal error.
	ErrInternal
)

// String returns a human-readable name for the error kind.
func (k ErrorKind) String() string {
	switch k {
	case ErrNotFound:
		return "not found"
	case ErrAlreadyExists:
		return "already exists"
	case ErrInvalidState:
		return "invalid state"
	case ErrInvalidConfig:
		return "invalid config"
	case ErrPermission:
		return "permission denied"
	case ErrResource:
		return "resource error"
	case ErrNamespace:
		return "namespace error"
	case ErrCgroup:
		return "cgroup error"
	case ErrSeccomp:
		return "seccomp error"
	case ErrCapability:
		return "capability error"
	case ErrDevice:
		return "device error"
	case ErrRootfs:
		return "rootfs error"
	case ErrInternal:
		return "internal error"
	default:
		return "unknown error"
	}
}

// ContainerError represents an error that occurred during a container operation.
type ContainerError struct {
	// Op is the operation that failed (e.g., "create", "start", "exec").
	Op string
	// Container is the container ID, if applicable.
	Container string
	// Err is the underlying error.
	Err error
	// Kind is the error classification.
	Kind ErrorKind
	// Detail provides additional context about the error.
	Detail string
}

// Error returns the error message.
func (e *ContainerError) Error() string {
	if e == nil {
		return "<nil>"
	}

	var msg string
	if e.Container != "" {
		msg = fmt.Sprintf("container %s: ", e.Container)
	}
	if e.Op != "" {
		msg += fmt.Sprintf("%s: ", e.Op)
	}
	if e.Detail != "" {
		msg += e.Detail
	} else {
		msg += e.Kind.String()
	}
	if e.Err != nil {
		msg += fmt.Sprintf(": %v", e.Err)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *ContainerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is reports whether the error matches the target.
// It matches if the target is a *ContainerError with the same Kind,
// or if the underlying error matches.
func (e *ContainerError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	if t, ok := target.(*ContainerError); ok {
		return e.Kind == t.Kind
	}
	return false
}

// New creates a new ContainerError with the given kind.
func New(kind ErrorKind, op string, detail string) *ContainerError {
	return &ContainerError{
		Op:     op,
		Kind:   kind,
		Detail: detail,
	}
}

// Wrap wraps an error with container context.
func Wrap(err error, kind ErrorKind, op string) *ContainerError {
	return &ContainerError{
		Op:   op,
		Err:  err,
		Kind: kind,
	}
}

// WrapWithContainer wraps an error with container context and ID.
func WrapWithContainer(err error, kind ErrorKind, op string, containerID string) *ContainerError {
	return &ContainerError{
		Op:        op,
		Container: containerID,
		Err:       err,
		Kind:      kind,
	}
}

// WrapWithDetail wraps an error with additional detail.
func WrapWithDetail(err error, kind ErrorKind, op string, detail string) *ContainerError {
	return &ContainerError{
		Op:     op,
		Err:    err,
		Kind:   kind,
		Detail: detail,
	}
}

// IsKind checks if an error is of a specific kind.
func IsKind(err error, kind ErrorKind) bool {
	var cerr *ContainerError
	if errors.As(err, &cerr) {
		return cerr.Kind == kind
	}
	return false
}

// GetKind returns the error kind if the error is a ContainerError.
func GetKind(err error) (ErrorKind, bool) {
	var cerr *ContainerError
	if errors.As(err, &cerr) {
		return cerr.Kind, true
	}
	return 0, false
}

// Re-export standard library functions for convenience.
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
)
