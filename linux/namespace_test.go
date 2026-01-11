package linux

import (
	"syscall"
	"testing"

	"runc-go/spec"
)

func TestNamespaceConstants(t *testing.T) {
	// Verify clone flags match syscall package
	if CLONE_NEWNS != syscall.CLONE_NEWNS {
		t.Errorf("CLONE_NEWNS mismatch")
	}

	if CLONE_NEWUTS != syscall.CLONE_NEWUTS {
		t.Errorf("CLONE_NEWUTS mismatch")
	}

	if CLONE_NEWIPC != syscall.CLONE_NEWIPC {
		t.Errorf("CLONE_NEWIPC mismatch")
	}

	if CLONE_NEWPID != syscall.CLONE_NEWPID {
		t.Errorf("CLONE_NEWPID mismatch")
	}

	if CLONE_NEWNET != syscall.CLONE_NEWNET {
		t.Errorf("CLONE_NEWNET mismatch")
	}

	if CLONE_NEWUSER != syscall.CLONE_NEWUSER {
		t.Errorf("CLONE_NEWUSER mismatch")
	}

	// CLONE_NEWCGROUP is not in syscall package
	if CLONE_NEWCGROUP != 0x02000000 {
		t.Errorf("CLONE_NEWCGROUP should be 0x02000000")
	}
}

func TestNamespaceTypeToFlag(t *testing.T) {
	tests := []struct {
		nsType   spec.LinuxNamespaceType
		expected uintptr
	}{
		{spec.PIDNamespace, CLONE_NEWPID},
		{spec.NetworkNamespace, CLONE_NEWNET},
		{spec.MountNamespace, CLONE_NEWNS},
		{spec.IPCNamespace, CLONE_NEWIPC},
		{spec.UTSNamespace, CLONE_NEWUTS},
		{spec.UserNamespace, CLONE_NEWUSER},
		{spec.CgroupNamespace, CLONE_NEWCGROUP},
	}

	for _, tc := range tests {
		flag, ok := namespaceTypeToFlag[tc.nsType]
		if !ok {
			t.Errorf("missing mapping for %s", tc.nsType)
			continue
		}
		if flag != tc.expected {
			t.Errorf("expected 0x%x for %s, got 0x%x", tc.expected, tc.nsType, flag)
		}
	}
}

func TestNamespaceFlags(t *testing.T) {
	namespaces := []spec.LinuxNamespace{
		{Type: spec.PIDNamespace},
		{Type: spec.NetworkNamespace},
		{Type: spec.MountNamespace},
	}

	flags := NamespaceFlags(namespaces)

	expected := uintptr(CLONE_NEWPID | CLONE_NEWNET | CLONE_NEWNS)
	if flags != expected {
		t.Errorf("expected 0x%x, got 0x%x", expected, flags)
	}
}

func TestNamespaceFlagsWithPath(t *testing.T) {
	// When path is set, namespace should not be created (flag not set)
	namespaces := []spec.LinuxNamespace{
		{Type: spec.PIDNamespace},
		{Type: spec.NetworkNamespace, Path: "/var/run/netns/test"},
		{Type: spec.MountNamespace},
	}

	flags := NamespaceFlags(namespaces)

	// Should not include CLONE_NEWNET since it has a path
	expected := uintptr(CLONE_NEWPID | CLONE_NEWNS)
	if flags != expected {
		t.Errorf("expected 0x%x, got 0x%x", expected, flags)
	}
}

func TestNamespaceFlagsEmpty(t *testing.T) {
	flags := NamespaceFlags(nil)
	if flags != 0 {
		t.Errorf("expected 0 for empty namespaces, got 0x%x", flags)
	}
}

func TestHasNamespace(t *testing.T) {
	namespaces := []spec.LinuxNamespace{
		{Type: spec.PIDNamespace},
		{Type: spec.NetworkNamespace},
	}

	if !HasNamespace(namespaces, spec.PIDNamespace) {
		t.Error("should have PID namespace")
	}

	if !HasNamespace(namespaces, spec.NetworkNamespace) {
		t.Error("should have network namespace")
	}

	if HasNamespace(namespaces, spec.MountNamespace) {
		t.Error("should not have mount namespace")
	}

	if HasNamespace(namespaces, spec.UserNamespace) {
		t.Error("should not have user namespace")
	}
}

func TestHasNamespaceEmpty(t *testing.T) {
	if HasNamespace(nil, spec.PIDNamespace) {
		t.Error("empty list should not have any namespace")
	}
}

func TestGetNamespacePath(t *testing.T) {
	namespaces := []spec.LinuxNamespace{
		{Type: spec.PIDNamespace},
		{Type: spec.NetworkNamespace, Path: "/var/run/netns/test"},
	}

	path := GetNamespacePath(namespaces, spec.NetworkNamespace)
	if path != "/var/run/netns/test" {
		t.Errorf("expected /var/run/netns/test, got %s", path)
	}

	path = GetNamespacePath(namespaces, spec.PIDNamespace)
	if path != "" {
		t.Errorf("expected empty path, got %s", path)
	}

	path = GetNamespacePath(namespaces, spec.MountNamespace)
	if path != "" {
		t.Errorf("expected empty path for missing namespace, got %s", path)
	}
}

func TestBuildIDMappings(t *testing.T) {
	mappings := []spec.LinuxIDMapping{
		{ContainerID: 0, HostID: 1000, Size: 1},
		{ContainerID: 1, HostID: 100000, Size: 65536},
	}

	result := buildIDMappings(mappings)

	if len(result) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(result))
	}

	if result[0].ContainerID != 0 || result[0].HostID != 1000 || result[0].Size != 1 {
		t.Errorf("first mapping incorrect: %+v", result[0])
	}

	if result[1].ContainerID != 1 || result[1].HostID != 100000 || result[1].Size != 65536 {
		t.Errorf("second mapping incorrect: %+v", result[1])
	}
}

func TestBuildIDMappingsEmpty(t *testing.T) {
	result := buildIDMappings(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d mappings", len(result))
	}
}

func TestFormatIDMap(t *testing.T) {
	mappings := []spec.LinuxIDMapping{
		{ContainerID: 0, HostID: 1000, Size: 1},
		{ContainerID: 1, HostID: 100000, Size: 65536},
	}

	result := formatIDMap(mappings)
	expected := "0 1000 1\n1 100000 65536\n"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatIDMapEmpty(t *testing.T) {
	result := formatIDMap(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildSysProcAttr(t *testing.T) {
	s := &spec.Spec{
		Linux: &spec.Linux{
			Namespaces: []spec.LinuxNamespace{
				{Type: spec.PIDNamespace},
				{Type: spec.MountNamespace},
				{Type: spec.UTSNamespace},
			},
		},
	}

	attr, err := BuildSysProcAttr(s)
	if err != nil {
		t.Fatalf("BuildSysProcAttr failed: %v", err)
	}

	if attr.Cloneflags&CLONE_NEWPID == 0 {
		t.Error("should have CLONE_NEWPID")
	}

	if attr.Cloneflags&CLONE_NEWNS == 0 {
		t.Error("should have CLONE_NEWNS")
	}

	if attr.Cloneflags&CLONE_NEWUTS == 0 {
		t.Error("should have CLONE_NEWUTS")
	}

	if !attr.Setsid {
		t.Error("Setsid should be true")
	}
}

func TestBuildSysProcAttrNoLinux(t *testing.T) {
	s := &spec.Spec{}

	attr, err := BuildSysProcAttr(s)
	if err != nil {
		t.Fatalf("BuildSysProcAttr failed: %v", err)
	}

	// Should have default namespaces
	expected := uintptr(CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWUTS | CLONE_NEWIPC | CLONE_NEWNET)
	if attr.Cloneflags != expected {
		t.Errorf("expected default flags 0x%x, got 0x%x", expected, attr.Cloneflags)
	}
}

func TestBuildSysProcAttrWithUserNamespace(t *testing.T) {
	s := &spec.Spec{
		Linux: &spec.Linux{
			Namespaces: []spec.LinuxNamespace{
				{Type: spec.PIDNamespace},
				{Type: spec.UserNamespace},
			},
			UIDMappings: []spec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 1},
			},
			GIDMappings: []spec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 1},
			},
		},
	}

	attr, err := BuildSysProcAttr(s)
	if err != nil {
		t.Fatalf("BuildSysProcAttr failed: %v", err)
	}

	if attr.Cloneflags&CLONE_NEWUSER == 0 {
		t.Error("should have CLONE_NEWUSER")
	}

	if len(attr.UidMappings) != 1 {
		t.Errorf("expected 1 UID mapping, got %d", len(attr.UidMappings))
	}

	if len(attr.GidMappings) != 1 {
		t.Errorf("expected 1 GID mapping, got %d", len(attr.GidMappings))
	}

	// Unshareflags should not be set with user namespace
	if attr.Unshareflags != 0 {
		t.Error("Unshareflags should be 0 with user namespace")
	}

	if attr.GidMappingsEnableSetgroups {
		t.Error("GidMappingsEnableSetgroups should be false")
	}
}

func TestSYS_SETNS(t *testing.T) {
	// Verify the syscall number for x86_64
	if SYS_SETNS != 308 {
		t.Errorf("SYS_SETNS should be 308 for x86_64, got %d", SYS_SETNS)
	}
}

func TestSetNamespacesEmpty(t *testing.T) {
	// Empty namespaces should succeed
	err := SetNamespaces(nil)
	if err != nil {
		t.Errorf("SetNamespaces with nil should succeed: %v", err)
	}

	err = SetNamespaces([]spec.LinuxNamespace{})
	if err != nil {
		t.Errorf("SetNamespaces with empty slice should succeed: %v", err)
	}
}

func TestSetNamespacesNoPath(t *testing.T) {
	// Namespaces without paths should be skipped
	namespaces := []spec.LinuxNamespace{
		{Type: spec.PIDNamespace},
		{Type: spec.NetworkNamespace},
	}

	err := SetNamespaces(namespaces)
	if err != nil {
		t.Errorf("SetNamespaces with no paths should succeed: %v", err)
	}
}

func TestSetHostnameEmpty(t *testing.T) {
	// Empty hostname should be no-op
	err := SetHostname("")
	if err != nil {
		t.Errorf("SetHostname with empty string should succeed: %v", err)
	}
}

func TestSetDomainnameEmpty(t *testing.T) {
	// Empty domainname should be no-op
	err := SetDomainname("")
	if err != nil {
		t.Errorf("SetDomainname with empty string should succeed: %v", err)
	}
}
