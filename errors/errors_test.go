package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorKind_String(t *testing.T) {
	tests := []struct {
		kind     ErrorKind
		expected string
	}{
		{ErrNotFound, "not found"},
		{ErrAlreadyExists, "already exists"},
		{ErrInvalidState, "invalid state"},
		{ErrInvalidConfig, "invalid config"},
		{ErrPermission, "permission denied"},
		{ErrResource, "resource error"},
		{ErrNamespace, "namespace error"},
		{ErrCgroup, "cgroup error"},
		{ErrSeccomp, "seccomp error"},
		{ErrCapability, "capability error"},
		{ErrDevice, "device error"},
		{ErrRootfs, "rootfs error"},
		{ErrInternal, "internal error"},
		{ErrorKind(999), "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.expected {
				t.Errorf("ErrorKind.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestContainerError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ContainerError
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "<nil>",
		},
		{
			name: "full error",
			err: &ContainerError{
				Op:        "create",
				Container: "test-container",
				Kind:      ErrNotFound,
				Detail:    "config.json not found",
				Err:       fmt.Errorf("file not found"),
			},
			expected: "container test-container: create: config.json not found: file not found",
		},
		{
			name: "without container",
			err: &ContainerError{
				Op:     "setup",
				Kind:   ErrRootfs,
				Detail: "pivot_root failed",
			},
			expected: "setup: pivot_root failed",
		},
		{
			name: "kind only",
			err: &ContainerError{
				Kind: ErrPermission,
			},
			expected: "permission denied",
		},
		{
			name: "with underlying error",
			err: &ContainerError{
				Op:   "mount",
				Kind: ErrRootfs,
				Err:  fmt.Errorf("device busy"),
			},
			expected: "mount: rootfs error: device busy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("ContainerError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestContainerError_Unwrap(t *testing.T) {
	underlying := fmt.Errorf("underlying error")
	err := &ContainerError{
		Op:   "test",
		Kind: ErrInternal,
		Err:  underlying,
	}

	if got := err.Unwrap(); got != underlying {
		t.Errorf("Unwrap() = %v, want %v", got, underlying)
	}

	// Test nil error
	var nilErr *ContainerError
	if got := nilErr.Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v, want nil", got)
	}
}

func TestContainerError_Is(t *testing.T) {
	err1 := &ContainerError{Kind: ErrNotFound, Op: "test1"}
	err2 := &ContainerError{Kind: ErrNotFound, Op: "test2"}
	err3 := &ContainerError{Kind: ErrPermission, Op: "test3"}

	// Same kind should match
	if !err1.Is(err2) {
		t.Error("err1.Is(err2) should be true (same kind)")
	}

	// Different kind should not match
	if err1.Is(err3) {
		t.Error("err1.Is(err3) should be false (different kind)")
	}

	// Non-ContainerError should not match
	if err1.Is(fmt.Errorf("some error")) {
		t.Error("err1.Is(fmt.Errorf(...)) should be false")
	}

	// Nil handling
	var nilErr *ContainerError
	if !nilErr.Is(nil) {
		t.Error("nil.Is(nil) should be true")
	}
}

func TestNew(t *testing.T) {
	err := New(ErrInvalidConfig, "validate", "container ID is empty")

	if err.Kind != ErrInvalidConfig {
		t.Errorf("Kind = %v, want %v", err.Kind, ErrInvalidConfig)
	}
	if err.Op != "validate" {
		t.Errorf("Op = %q, want %q", err.Op, "validate")
	}
	if err.Detail != "container ID is empty" {
		t.Errorf("Detail = %q, want %q", err.Detail, "container ID is empty")
	}
}

func TestWrap(t *testing.T) {
	underlying := fmt.Errorf("permission denied")
	err := Wrap(underlying, ErrPermission, "open file")

	if err.Err != underlying {
		t.Error("Wrapped error should preserve underlying error")
	}
	if err.Kind != ErrPermission {
		t.Errorf("Kind = %v, want %v", err.Kind, ErrPermission)
	}
	if err.Op != "open file" {
		t.Errorf("Op = %q, want %q", err.Op, "open file")
	}
}

func TestWrapWithContainer(t *testing.T) {
	underlying := fmt.Errorf("not found")
	err := WrapWithContainer(underlying, ErrNotFound, "load", "my-container")

	if err.Container != "my-container" {
		t.Errorf("Container = %q, want %q", err.Container, "my-container")
	}
}

func TestWrapWithDetail(t *testing.T) {
	underlying := fmt.Errorf("syscall failed")
	err := WrapWithDetail(underlying, ErrSeccomp, "filter", "invalid architecture")

	if err.Detail != "invalid architecture" {
		t.Errorf("Detail = %q, want %q", err.Detail, "invalid architecture")
	}
}

func TestIsKind(t *testing.T) {
	err := &ContainerError{Kind: ErrNotFound}
	wrapped := fmt.Errorf("wrapped: %w", err)

	if !IsKind(err, ErrNotFound) {
		t.Error("IsKind(err, ErrNotFound) should be true")
	}
	if !IsKind(wrapped, ErrNotFound) {
		t.Error("IsKind(wrapped, ErrNotFound) should be true")
	}
	if IsKind(err, ErrPermission) {
		t.Error("IsKind(err, ErrPermission) should be false")
	}
	if IsKind(fmt.Errorf("plain error"), ErrNotFound) {
		t.Error("IsKind(plain error, ErrNotFound) should be false")
	}
}

func TestGetKind(t *testing.T) {
	err := &ContainerError{Kind: ErrCgroup}
	wrapped := fmt.Errorf("wrapped: %w", err)

	kind, ok := GetKind(err)
	if !ok || kind != ErrCgroup {
		t.Errorf("GetKind(err) = (%v, %v), want (%v, true)", kind, ok, ErrCgroup)
	}

	kind, ok = GetKind(wrapped)
	if !ok || kind != ErrCgroup {
		t.Errorf("GetKind(wrapped) = (%v, %v), want (%v, true)", kind, ok, ErrCgroup)
	}

	_, ok = GetKind(fmt.Errorf("plain error"))
	if ok {
		t.Error("GetKind(plain error) should return false")
	}
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  *ContainerError
		kind ErrorKind
	}{
		{"ErrContainerNotFound", ErrContainerNotFound, ErrNotFound},
		{"ErrContainerExists", ErrContainerExists, ErrAlreadyExists},
		{"ErrContainerNotRunning", ErrContainerNotRunning, ErrInvalidState},
		{"ErrContainerNotStopped", ErrContainerNotStopped, ErrInvalidState},
		{"ErrInvalidContainerID", ErrInvalidContainerID, ErrInvalidConfig},
		{"ErrPathTraversal", ErrPathTraversal, ErrInvalidConfig},
		{"ErrSeccompFilter", ErrSeccompFilter, ErrSeccomp},
		{"ErrCapabilityDrop", ErrCapabilityDrop, ErrCapability},
		{"ErrNamespaceSetup", ErrNamespaceSetup, ErrNamespace},
		{"ErrCgroupSetup", ErrCgroupSetup, ErrCgroup},
		{"ErrDeviceCreate", ErrDeviceCreate, ErrDevice},
		{"ErrRootfsSetup", ErrRootfsSetup, ErrRootfs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Kind != tt.kind {
				t.Errorf("%s.Kind = %v, want %v", tt.name, tt.err.Kind, tt.kind)
			}
			// Ensure Is() works with sentinel errors
			wrapped := Wrap(fmt.Errorf("underlying"), tt.kind, "test")
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("errors.Is(wrapped, %s) should be true", tt.name)
			}
		})
	}
}

func TestErrorChain(t *testing.T) {
	// Test that error chains work correctly with errors.Is and errors.As
	underlying := fmt.Errorf("file not found")
	err1 := Wrap(underlying, ErrNotFound, "load spec")
	err2 := fmt.Errorf("container operation failed: %w", err1)

	// errors.Is should find the ContainerError in the chain
	if !errors.Is(err2, ErrContainerNotFound) {
		t.Error("errors.Is should find ErrContainerNotFound in chain")
	}

	// errors.As should extract the ContainerError
	var cerr *ContainerError
	if !errors.As(err2, &cerr) {
		t.Error("errors.As should find ContainerError in chain")
	}
	if cerr.Op != "load spec" {
		t.Errorf("cerr.Op = %q, want %q", cerr.Op, "load spec")
	}

	// Unwrap should work through the chain
	if errors.Unwrap(err1) != underlying {
		t.Error("Unwrap should return underlying error")
	}
}
