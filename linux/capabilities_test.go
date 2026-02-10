package linux

import (
	"testing"

	"runc-go/spec"
)

func TestCapabilityMap_Complete(t *testing.T) {
	// Verify all standard capabilities are in the map
	expectedCaps := []struct {
		name string
		num  int
	}{
		{"CAP_CHOWN", CAP_CHOWN},
		{"CAP_DAC_OVERRIDE", CAP_DAC_OVERRIDE},
		{"CAP_DAC_READ_SEARCH", CAP_DAC_READ_SEARCH},
		{"CAP_FOWNER", CAP_FOWNER},
		{"CAP_FSETID", CAP_FSETID},
		{"CAP_KILL", CAP_KILL},
		{"CAP_SETGID", CAP_SETGID},
		{"CAP_SETUID", CAP_SETUID},
		{"CAP_SETPCAP", CAP_SETPCAP},
		{"CAP_NET_BIND_SERVICE", CAP_NET_BIND_SERVICE},
		{"CAP_NET_ADMIN", CAP_NET_ADMIN},
		{"CAP_NET_RAW", CAP_NET_RAW},
		{"CAP_SYS_MODULE", CAP_SYS_MODULE},
		{"CAP_SYS_CHROOT", CAP_SYS_CHROOT},
		{"CAP_SYS_PTRACE", CAP_SYS_PTRACE},
		{"CAP_SYS_ADMIN", CAP_SYS_ADMIN},
		{"CAP_MKNOD", CAP_MKNOD},
		{"CAP_AUDIT_WRITE", CAP_AUDIT_WRITE},
		{"CAP_SYSLOG", CAP_SYSLOG},
	}

	for _, cap := range expectedCaps {
		t.Run(cap.name, func(t *testing.T) {
			num, ok := capabilityMap[cap.name]
			if !ok {
				t.Errorf("Capability %s not found in capabilityMap", cap.name)
				return
			}
			if num != cap.num {
				t.Errorf("capabilityMap[%s] = %d, want %d", cap.name, num, cap.num)
			}
		})
	}
}

func TestCapabilityToName(t *testing.T) {
	tests := []struct {
		num  int
		want string
	}{
		{CAP_CHOWN, "CAP_CHOWN"},
		{CAP_DAC_OVERRIDE, "CAP_DAC_OVERRIDE"},
		{CAP_SETUID, "CAP_SETUID"},
		{CAP_SETGID, "CAP_SETGID"},
		{CAP_SYS_ADMIN, "CAP_SYS_ADMIN"},
		{CAP_NET_ADMIN, "CAP_NET_ADMIN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := CapabilityToName(tt.num)
			if got != tt.want {
				t.Errorf("CapabilityToName(%d) = %q, want %q", tt.num, got, tt.want)
			}
		})
	}
}

func TestNameToCapability(t *testing.T) {
	tests := []struct {
		name    string
		want    int
		wantOk  bool
	}{
		{"CAP_CHOWN", CAP_CHOWN, true},
		{"CAP_SYS_ADMIN", CAP_SYS_ADMIN, true},
		{"CAP_NET_ADMIN", CAP_NET_ADMIN, true},
		{"INVALID_CAP", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NameToCapability(tt.name)
			if ok != tt.wantOk {
				t.Errorf("NameToCapability(%q) ok = %v, wantOk %v", tt.name, ok, tt.wantOk)
				return
			}
			if tt.wantOk && got != tt.want {
				t.Errorf("NameToCapability(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetLastCap(t *testing.T) {
	lastCap := getLastCap()

	// Last capability should be at least CAP_CHECKPOINT_RESTORE (40)
	if lastCap < 40 {
		t.Errorf("getLastCap() = %d, expected at least 40", lastCap)
	}

	// Should not exceed reasonable maximum
	if lastCap > 63 {
		t.Errorf("getLastCap() = %d, expected at most 63", lastCap)
	}
}

func TestAllCapabilities(t *testing.T) {
	caps := AllCapabilities()

	// Should have at least the standard set of capabilities
	if len(caps) < 40 {
		t.Errorf("AllCapabilities() returned %d caps, expected at least 40", len(caps))
	}

	// Verify some specific capabilities are included
	expectedCaps := []string{
		"CAP_CHOWN",
		"CAP_DAC_OVERRIDE",
		"CAP_SETUID",
		"CAP_SETGID",
		"CAP_SYS_ADMIN",
		"CAP_NET_ADMIN",
	}

	for _, expected := range expectedCaps {
		found := false
		for _, cap := range caps {
			if cap == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllCapabilities() missing capability %s", expected)
		}
	}
}

func TestMakeCapSet(t *testing.T) {
	tests := []struct {
		name     string
		capNames []string
		wantLen  int
	}{
		{
			name:     "empty set",
			capNames: []string{},
			wantLen:  0,
		},
		{
			name:     "single capability",
			capNames: []string{"CAP_NET_ADMIN"},
			wantLen:  1,
		},
		{
			name:     "multiple capabilities",
			capNames: []string{"CAP_CHOWN", "CAP_SETUID", "CAP_SETGID"},
			wantLen:  3,
		},
		{
			name:     "with invalid capability (ignored)",
			capNames: []string{"CAP_CHOWN", "CAP_INVALID"},
			wantLen:  1, // Only valid cap is counted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capSet := makeCapSet(tt.capNames)
			if len(capSet) != tt.wantLen {
				t.Errorf("makeCapSet() returned %d caps, want %d", len(capSet), tt.wantLen)
			}
		})
	}
}

func TestCapSetContents(t *testing.T) {
	capNames := []string{"CAP_CHOWN", "CAP_SETUID", "CAP_NET_ADMIN"}
	capSet := makeCapSet(capNames)

	// Verify all requested capabilities are in the set
	for _, name := range capNames {
		capNum, ok := NameToCapability(name)
		if !ok {
			t.Errorf("Invalid capability name: %s", name)
			continue
		}
		if !capSet[capNum] {
			t.Errorf("makeCapSet() missing %s", name)
		}
	}
}

func TestLinuxCapabilitiesSpec(t *testing.T) {
	// Test parsing of OCI capabilities spec
	caps := &spec.LinuxCapabilities{
		Bounding:    []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_KILL"},
		Effective:   []string{"CAP_CHOWN"},
		Permitted:   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE"},
		Inheritable: []string{},
		Ambient:     []string{},
	}

	// Parse bounding set
	boundingSet := makeCapSet(caps.Bounding)
	if len(boundingSet) != 3 {
		t.Errorf("Bounding set has %d caps, expected 3", len(boundingSet))
	}

	// Parse effective set
	effectiveSet := makeCapSet(caps.Effective)
	if len(effectiveSet) != 1 {
		t.Errorf("Effective set has %d caps, expected 1", len(effectiveSet))
	}

	// Parse permitted set
	permittedSet := makeCapSet(caps.Permitted)
	if len(permittedSet) != 2 {
		t.Errorf("Permitted set has %d caps, expected 2", len(permittedSet))
	}
}

func TestDangerousCapabilities(t *testing.T) {
	// These capabilities should be considered dangerous and not granted by default
	dangerousCaps := []string{
		"CAP_SYS_ADMIN",    // Extremely powerful
		"CAP_SYS_MODULE",   // Load kernel modules
		"CAP_SYS_RAWIO",    // Raw I/O access
		"CAP_SYS_PTRACE",   // Debug processes
		"CAP_NET_ADMIN",    // Network configuration
		"CAP_SYS_BOOT",     // Reboot system
		"CAP_MAC_ADMIN",    // MAC configuration
		"CAP_MAC_OVERRIDE", // Override MAC
	}

	for _, capName := range dangerousCaps {
		t.Run(capName, func(t *testing.T) {
			capNum, ok := NameToCapability(capName)
			if !ok {
				t.Errorf("Dangerous capability %s not found", capName)
				return
			}
			// Just verify it exists and has the right number
			if capNum < 0 || capNum > 63 {
				t.Errorf("Invalid capability number for %s: %d", capName, capNum)
			}
		})
	}
}

func TestCapabilityConstants(t *testing.T) {
	// Verify capability constants match expected values from Linux kernel
	tests := []struct {
		name     string
		constant int
		expected int
	}{
		{"CAP_CHOWN", CAP_CHOWN, 0},
		{"CAP_DAC_OVERRIDE", CAP_DAC_OVERRIDE, 1},
		{"CAP_KILL", CAP_KILL, 5},
		{"CAP_SETUID", CAP_SETUID, 7},
		{"CAP_NET_BIND_SERVICE", CAP_NET_BIND_SERVICE, 10},
		{"CAP_SYS_ADMIN", CAP_SYS_ADMIN, 21},
		{"CAP_MKNOD", CAP_MKNOD, 27},
		{"CAP_AUDIT_WRITE", CAP_AUDIT_WRITE, 29},
		{"CAP_SYSLOG", CAP_SYSLOG, 34},
		{"CAP_CHECKPOINT_RESTORE", CAP_CHECKPOINT_RESTORE, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
