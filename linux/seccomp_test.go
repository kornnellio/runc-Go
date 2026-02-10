package linux

import (
	"testing"

	"runc-go/spec"
)

// ============================================================================
// ARCHITECTURE TESTS
// ============================================================================

// TestArchToAudit_ValidArches tests that all supported architectures map correctly.
func TestArchToAudit_ValidArches(t *testing.T) {
	tests := []struct {
		arch     spec.Arch
		expected uint32
	}{
		{spec.ArchX86_64, AUDIT_ARCH_X86_64},
		{spec.ArchX86, AUDIT_ARCH_I386},
		{spec.ArchAARCH64, AUDIT_ARCH_AARCH64},
		{spec.ArchARM, AUDIT_ARCH_ARM},
	}

	for _, tt := range tests {
		t.Run(string(tt.arch), func(t *testing.T) {
			got, ok := archToAudit[tt.arch]
			if !ok {
				t.Errorf("arch %s not found in archToAudit", tt.arch)
				return
			}
			if got != tt.expected {
				t.Errorf("archToAudit[%s] = 0x%x, want 0x%x", tt.arch, got, tt.expected)
			}
		})
	}
}

// TestArchToAudit_UnknownArch tests that unknown architectures are not in the map.
func TestArchToAudit_UnknownArch(t *testing.T) {
	unknownArches := []spec.Arch{
		"SCMP_ARCH_UNKNOWN",
		"invalid",
		"",
	}

	for _, arch := range unknownArches {
		if _, ok := archToAudit[arch]; ok {
			t.Errorf("unknown arch %q should not be in archToAudit", arch)
		}
	}
}

// ============================================================================
// ACTION TESTS
// ============================================================================

// TestActionToRet_AllActions tests that all OCI actions map to seccomp return values.
func TestActionToRet_AllActions(t *testing.T) {
	tests := []struct {
		action   spec.LinuxSeccompAction
		expected uint32
	}{
		{spec.ActKill, SECCOMP_RET_KILL_THREAD},
		{spec.ActKillProcess, SECCOMP_RET_KILL_PROCESS},
		{spec.ActKillThread, SECCOMP_RET_KILL_THREAD},
		{spec.ActTrap, SECCOMP_RET_TRAP},
		{spec.ActErrno, SECCOMP_RET_ERRNO},
		{spec.ActTrace, SECCOMP_RET_TRACE},
		{spec.ActAllow, SECCOMP_RET_ALLOW},
		{spec.ActLog, SECCOMP_RET_LOG},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got, ok := actionToRet[tt.action]
			if !ok {
				t.Errorf("action %s not found in actionToRet", tt.action)
				return
			}
			if got != tt.expected {
				t.Errorf("actionToRet[%s] = 0x%x, want 0x%x", tt.action, got, tt.expected)
			}
		})
	}
}

// TestActionToRet_UnknownAction tests that unknown actions are not in the map.
func TestActionToRet_UnknownAction(t *testing.T) {
	unknownActions := []spec.LinuxSeccompAction{
		"SCMP_ACT_UNKNOWN",
		"invalid",
		"",
	}

	for _, action := range unknownActions {
		if _, ok := actionToRet[action]; ok {
			t.Errorf("unknown action %q should not be in actionToRet", action)
		}
	}
}

// ============================================================================
// SYSCALL MAP TESTS
// ============================================================================

// TestSyscallMap_CommonSyscalls tests that common syscalls are mapped.
func TestSyscallMap_CommonSyscalls(t *testing.T) {
	// Critical syscalls that must be present
	criticalSyscalls := []struct {
		name     string
		expected int
	}{
		{"read", 0},
		{"write", 1},
		{"open", 2},
		{"close", 3},
		{"execve", 59},
		{"exit", 60},
		{"clone", 56},
		{"fork", 57},
		{"kill", 62},
	}

	for _, sc := range criticalSyscalls {
		t.Run(sc.name, func(t *testing.T) {
			got, ok := syscallMap[sc.name]
			if !ok {
				t.Errorf("syscall %s not found in syscallMap", sc.name)
				return
			}
			if got != sc.expected {
				t.Errorf("syscallMap[%s] = %d, want %d", sc.name, got, sc.expected)
			}
		})
	}
}

// TestSyscallMap_NoNegativeNumbers tests that no syscall has a negative number.
func TestSyscallMap_NoNegativeNumbers(t *testing.T) {
	for name, nr := range syscallMap {
		if nr < 0 {
			t.Errorf("syscall %s has negative number %d", name, nr)
		}
	}
}

// ============================================================================
// BPF FILTER BUILD TESTS
// ============================================================================

// TestBuildSeccompFilter_EmptyConfig tests building filter with empty config.
func TestBuildSeccompFilter_EmptyConfig(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Should have at least arch check + default action
	if len(filter) < 3 {
		t.Errorf("filter too short: %d instructions", len(filter))
	}
}

// TestBuildSeccompFilter_SingleSyscall tests building filter with one syscall rule.
func TestBuildSeccompFilter_SingleSyscall(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Syscalls: []spec.LinuxSyscall{
			{
				Names:  []string{"write"},
				Action: spec.ActErrno,
			},
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Should have instructions for:
	// - Load arch + arch check(s) + kill
	// - Load syscall number
	// - Syscall check + return
	// - Default return
	if len(filter) < 5 {
		t.Errorf("filter too short for single syscall: %d instructions", len(filter))
	}
}

// TestBuildSeccompFilter_MultipleSyscalls tests building filter with multiple syscall rules.
func TestBuildSeccompFilter_MultipleSyscalls(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Syscalls: []spec.LinuxSyscall{
			{
				Names:  []string{"write", "read"},
				Action: spec.ActLog,
			},
			{
				Names:  []string{"execve"},
				Action: spec.ActKillProcess,
			},
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Should have instructions for all syscalls
	if len(filter) < 8 {
		t.Errorf("filter too short for multiple syscalls: %d instructions", len(filter))
	}
}

// TestBuildSeccompFilter_UnknownDefaultAction tests that unknown default action returns error.
func TestBuildSeccompFilter_UnknownDefaultAction(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: "SCMP_ACT_INVALID",
	}

	_, err := buildSeccompFilter(config)
	if err == nil {
		t.Error("expected error for unknown default action")
	}
}

// TestBuildSeccompFilter_MultipleArches tests filter with multiple architectures.
func TestBuildSeccompFilter_MultipleArches(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Architectures: []spec.Arch{
			spec.ArchX86_64,
			spec.ArchX86,
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Should have 2 arch check instructions + kill + other instructions
	if len(filter) < 4 {
		t.Errorf("filter too short for multiple arches: %d instructions", len(filter))
	}
}

// TestBuildSeccompFilter_UnknownArchFiltered tests that unknown arches are filtered.
func TestBuildSeccompFilter_UnknownArchFiltered(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Architectures: []spec.Arch{
			spec.ArchX86_64,
			"SCMP_ARCH_UNKNOWN", // Should be filtered out
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Should still produce valid filter (unknown arch just skipped)
	if len(filter) < 3 {
		t.Errorf("filter too short: %d instructions", len(filter))
	}
}

// TestBuildSeccompFilter_ErrnoWithValue tests errno action with custom value.
func TestBuildSeccompFilter_ErrnoWithValue(t *testing.T) {
	errnoVal := uint(1) // EPERM
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Syscalls: []spec.LinuxSyscall{
			{
				Names:    []string{"write"},
				Action:   spec.ActErrno,
				ErrnoRet: &errnoVal,
			},
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Verify filter was built (detailed verification would require BPF interpretation)
	if len(filter) < 5 {
		t.Errorf("filter too short: %d instructions", len(filter))
	}
}

// ============================================================================
// BPF INSTRUCTION TESTS
// ============================================================================

// TestBpfStmt_Encoding tests that BPF statements are encoded correctly.
func TestBpfStmt_Encoding(t *testing.T) {
	tests := []struct {
		name string
		code uint16
		k    uint32
	}{
		{"load arch", BPF_LD | BPF_W | BPF_ABS, offsetArch},
		{"load nr", BPF_LD | BPF_W | BPF_ABS, offsetNR},
		{"ret allow", BPF_RET | BPF_K, SECCOMP_RET_ALLOW},
		{"ret kill", BPF_RET | BPF_K, SECCOMP_RET_KILL_PROCESS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := bpfStmt(tt.code, tt.k)
			if inst.Code != tt.code {
				t.Errorf("Code = %d, want %d", inst.Code, tt.code)
			}
			if inst.K != tt.k {
				t.Errorf("K = %d, want %d", inst.K, tt.k)
			}
			if inst.Jt != 0 || inst.Jf != 0 {
				t.Error("statement should have Jt=0 and Jf=0")
			}
		})
	}
}

// TestBpfJump_Encoding tests that BPF jumps are encoded correctly.
func TestBpfJump_Encoding(t *testing.T) {
	tests := []struct {
		name string
		code uint16
		k    uint32
		jt   uint8
		jf   uint8
	}{
		{"jeq arch", BPF_JMP | BPF_JEQ | BPF_K, AUDIT_ARCH_X86_64, 1, 0},
		{"jeq syscall", BPF_JMP | BPF_JEQ | BPF_K, 1, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := bpfJump(tt.code, tt.k, tt.jt, tt.jf)
			if inst.Code != tt.code {
				t.Errorf("Code = %d, want %d", inst.Code, tt.code)
			}
			if inst.K != tt.k {
				t.Errorf("K = %d, want %d", inst.K, tt.k)
			}
			if inst.Jt != tt.jt {
				t.Errorf("Jt = %d, want %d", inst.Jt, tt.jt)
			}
			if inst.Jf != tt.jf {
				t.Errorf("Jf = %d, want %d", inst.Jf, tt.jf)
			}
		})
	}
}

// ============================================================================
// ARCH JUMP CALCULATION TESTS
// ============================================================================

// TestArchJumpCalculation_SingleArch tests jump calculation with single architecture.
func TestArchJumpCalculation_SingleArch(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Architectures: []spec.Arch{spec.ArchX86_64},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Find the arch check instruction (should be after the load arch instruction)
	// Instruction 0: load arch
	// Instruction 1: arch check (should jump to instruction 2 on match = jt=1)
	// Instruction 2: kill
	// Instruction 3: load nr
	// ...
	if len(filter) < 4 {
		t.Fatalf("filter too short: %d", len(filter))
	}

	archCheckInst := filter[1]
	// For single arch, jt should be 1 (jump over kill instruction)
	if archCheckInst.Jt != 1 {
		t.Errorf("single arch jt = %d, want 1", archCheckInst.Jt)
	}
}

// TestArchJumpCalculation_TwoArches tests jump calculation with two architectures.
func TestArchJumpCalculation_TwoArches(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Architectures: []spec.Arch{spec.ArchX86_64, spec.ArchX86},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Instruction 0: load arch
	// Instruction 1: arch check x86_64 (jt=2: jump over next arch check + kill)
	// Instruction 2: arch check x86 (jt=1: jump over kill)
	// Instruction 3: kill
	// Instruction 4: load nr
	if len(filter) < 5 {
		t.Fatalf("filter too short: %d", len(filter))
	}

	firstArchCheck := filter[1]
	secondArchCheck := filter[2]

	// First arch should jump 2 instructions (over second arch check + kill)
	if firstArchCheck.Jt != 2 {
		t.Errorf("first arch jt = %d, want 2", firstArchCheck.Jt)
	}
	// Second arch should jump 1 instruction (over kill)
	if secondArchCheck.Jt != 1 {
		t.Errorf("second arch jt = %d, want 1", secondArchCheck.Jt)
	}
}

// TestArchJumpCalculation_WithUnknownArch tests that unknown arches don't break jump calculation.
func TestArchJumpCalculation_WithUnknownArch(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Architectures: []spec.Arch{
			spec.ArchX86_64,
			"SCMP_ARCH_UNKNOWN", // Unknown - should be filtered
			spec.ArchX86,
		},
	}

	filter, err := buildSeccompFilter(config)
	if err != nil {
		t.Fatalf("buildSeccompFilter failed: %v", err)
	}

	// Unknown arch should be filtered out, so we should have 2 arch checks
	// Instruction 0: load arch
	// Instruction 1: arch check x86_64 (jt=2)
	// Instruction 2: arch check x86 (jt=1)
	// Instruction 3: kill
	if len(filter) < 5 {
		t.Fatalf("filter too short: %d", len(filter))
	}

	firstArchCheck := filter[1]
	secondArchCheck := filter[2]

	// First arch should jump 2 (over second arch check + kill)
	if firstArchCheck.Jt != 2 {
		t.Errorf("first arch jt = %d, want 2 (unknown arch should be filtered)", firstArchCheck.Jt)
	}
	// Second arch should jump 1 (over kill)
	if secondArchCheck.Jt != 1 {
		t.Errorf("second arch jt = %d, want 1", secondArchCheck.Jt)
	}
}

// ============================================================================
// SETUP SECCOMP TESTS
// ============================================================================

// TestSetupSeccomp_TooManyUnrecognized tests that high unrecognized syscall ratio fails.
func TestSetupSeccomp_TooManyUnrecognized(t *testing.T) {
	// Create a config with mostly unknown syscalls
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Syscalls: []spec.LinuxSyscall{
			{
				Names:  []string{"totally_fake_syscall_1", "totally_fake_syscall_2", "totally_fake_syscall_3"},
				Action: spec.ActLog,
			},
			{
				Names:  []string{"read"}, // Only one real syscall
				Action: spec.ActAllow,
			},
		},
	}

	// This should fail because >20% are unrecognized
	err := SetupSeccomp(config)
	if err == nil {
		t.Error("expected error when >20% syscalls are unrecognized")
	}
}

// TestSetupSeccomp_NilConfig tests that nil config returns no error.
func TestSetupSeccomp_NilConfig(t *testing.T) {
	err := SetupSeccomp(nil)
	if err != nil {
		t.Errorf("nil config should not error: %v", err)
	}
}

// TestSetupSeccomp_EmptySyscalls tests that empty syscalls config returns no error.
func TestSetupSeccomp_EmptySyscalls(t *testing.T) {
	config := &spec.LinuxSeccomp{
		DefaultAction: spec.ActAllow,
		Syscalls:      []spec.LinuxSyscall{},
	}

	err := SetupSeccomp(config)
	if err != nil {
		t.Errorf("empty syscalls should not error: %v", err)
	}
}
