package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"runc-go/spec"
)

// ============================================================================
// RUN TESTS
// ============================================================================

// TestRun_NilHooks tests that nil hooks don't cause errors.
func TestRun_NilHooks(t *testing.T) {
	err := Run(nil, Prestart, &spec.State{})
	if err != nil {
		t.Errorf("nil hooks should not error: %v", err)
	}
}

// TestRun_EmptyHooks tests that empty hooks list doesn't cause errors.
func TestRun_EmptyHooks(t *testing.T) {
	hooks := &spec.Hooks{}
	err := Run(hooks, Prestart, &spec.State{})
	if err != nil {
		t.Errorf("empty hooks should not error: %v", err)
	}
}

// TestRun_UnknownHookType tests that unknown hook type returns error.
func TestRun_UnknownHookType(t *testing.T) {
	hooks := &spec.Hooks{}
	err := Run(hooks, "unknown", &spec.State{})
	if err == nil {
		t.Error("expected error for unknown hook type")
	}
}

// TestRun_AllHookTypes tests that all hook types are handled.
func TestRun_AllHookTypes(t *testing.T) {
	hookTypes := []HookType{
		Prestart,
		CreateRuntime,
		CreateContainer,
		StartContainer,
		Poststart,
		Poststop,
	}

	hooks := &spec.Hooks{}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusRunning,
		Bundle:  "/tmp/bundle",
	}

	for _, ht := range hookTypes {
		t.Run(string(ht), func(t *testing.T) {
			err := Run(hooks, ht, state)
			if err != nil {
				t.Errorf("hook type %s should not error: %v", ht, err)
			}
		})
	}
}

// TestRun_SuccessfulHook tests running a successful hook.
func TestRun_SuccessfulHook(t *testing.T) {
	// Create a temp script that exits successfully
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: scriptPath},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Errorf("successful hook should not error: %v", err)
	}
}

// TestRun_FailingHook tests running a failing hook.
func TestRun_FailingHook(t *testing.T) {
	// Create a temp script that exits with error
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: scriptPath},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err == nil {
		t.Error("failing hook should return error")
	}
}

// TestRun_NonexistentHook tests running a nonexistent hook.
func TestRun_NonexistentHook(t *testing.T) {
	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: "/nonexistent/hook"},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err == nil {
		t.Error("nonexistent hook should return error")
	}
}

// TestRun_MultipleHooks tests running multiple hooks in order.
func TestRun_MultipleHooks(t *testing.T) {
	tempDir := t.TempDir()

	// Create first hook that writes "1" to file
	script1Path := filepath.Join(tempDir, "hook1.sh")
	outputFile := filepath.Join(tempDir, "output")
	script1 := "#!/bin/sh\necho -n '1' >> " + outputFile + "\nexit 0\n"
	if err := os.WriteFile(script1Path, []byte(script1), 0755); err != nil {
		t.Fatalf("failed to write script1: %v", err)
	}

	// Create second hook that writes "2" to file
	script2Path := filepath.Join(tempDir, "hook2.sh")
	script2 := "#!/bin/sh\necho -n '2' >> " + outputFile + "\nexit 0\n"
	if err := os.WriteFile(script2Path, []byte(script2), 0755); err != nil {
		t.Fatalf("failed to write script2: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: script1Path},
			{Path: script2Path},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Fatalf("hooks failed: %v", err)
	}

	// Check output file
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if string(content) != "12" {
		t.Errorf("hooks ran out of order: got %q, want %q", string(content), "12")
	}
}

// TestRun_HookStopsOnError tests that hooks stop on first error.
func TestRun_HookStopsOnError(t *testing.T) {
	tempDir := t.TempDir()

	// Create first hook that fails
	script1Path := filepath.Join(tempDir, "hook1.sh")
	script1 := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(script1Path, []byte(script1), 0755); err != nil {
		t.Fatalf("failed to write script1: %v", err)
	}

	// Create second hook that writes to file (should not run)
	script2Path := filepath.Join(tempDir, "hook2.sh")
	outputFile := filepath.Join(tempDir, "output")
	script2 := "#!/bin/sh\necho 'ran' > " + outputFile + "\nexit 0\n"
	if err := os.WriteFile(script2Path, []byte(script2), 0755); err != nil {
		t.Fatalf("failed to write script2: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: script1Path},
			{Path: script2Path},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err == nil {
		t.Error("expected error from first hook")
	}

	// Second hook should not have run
	if _, err := os.Stat(outputFile); err == nil {
		t.Error("second hook should not have run after first failed")
	}
}

// ============================================================================
// HOOK WITH ARGS TESTS
// ============================================================================

// TestRun_HookWithArgs tests hook with custom arguments.
func TestRun_HookWithArgs(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	outputFile := filepath.Join(tempDir, "output")

	// Script that writes its arguments to file
	script := "#!/bin/sh\necho \"$@\" > " + outputFile + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{
				Path: scriptPath,
				Args: []string{scriptPath, "arg1", "arg2"},
			},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	expected := "arg1 arg2\n"
	if string(content) != expected {
		t.Errorf("args not passed correctly: got %q, want %q", string(content), expected)
	}
}

// ============================================================================
// HOOK WITH ENV TESTS
// ============================================================================

// TestRun_HookWithEnv tests hook with custom environment.
func TestRun_HookWithEnv(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	outputFile := filepath.Join(tempDir, "output")

	// Script that writes env var to file
	script := "#!/bin/sh\necho \"$CUSTOM_VAR\" > " + outputFile + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{
				Path: scriptPath,
				Env:  []string{"CUSTOM_VAR=test_value"},
			},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	expected := "test_value\n"
	if string(content) != expected {
		t.Errorf("env not passed correctly: got %q, want %q", string(content), expected)
	}
}

// ============================================================================
// HOOK TIMEOUT TESTS
// ============================================================================

// TestRun_HookTimeout tests that hook timeout is enforced.
func TestRun_HookTimeout(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")

	// Script that sleeps for 10 seconds
	script := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	timeout := 1 // 1 second timeout
	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{
				Path:    scriptPath,
				Timeout: &timeout,
			},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// TestRun_HookNoTimeout tests hook with no timeout completes normally.
func TestRun_HookNoTimeout(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")

	// Quick script
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{
				Path:    scriptPath,
				Timeout: nil, // No timeout
			},
		},
	}
	state := &spec.State{
		Version: spec.Version,
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Errorf("hook should not error: %v", err)
	}
}

// ============================================================================
// STATE INPUT TESTS
// ============================================================================

// TestRun_HookReceivesState tests that hook receives state on stdin.
func TestRun_HookReceivesState(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	outputFile := filepath.Join(tempDir, "output")

	// Script that copies stdin to output file
	script := "#!/bin/sh\ncat > " + outputFile + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: scriptPath},
		},
	}
	state := &spec.State{
		Version: "1.0.0",
		ID:      "test-container",
		Status:  spec.StatusCreated,
		Pid:     12345,
		Bundle:  "/tmp/bundle",
	}

	err := Run(hooks, Prestart, state)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	// Verify it's valid JSON containing expected fields
	got := string(content)
	if !contains(got, `"ociVersion":"1.0.0"`) {
		t.Errorf("state missing version: %s", got)
	}
	if !contains(got, `"id":"test-container"`) {
		t.Errorf("state missing id: %s", got)
	}
	if !contains(got, `"status":"created"`) {
		t.Errorf("state missing status: %s", got)
	}
	if !contains(got, `"pid":12345`) {
		t.Errorf("state missing pid: %s", got)
	}
	if !contains(got, `"bundle":"/tmp/bundle"`) {
		t.Errorf("state missing bundle: %s", got)
	}
}

// ============================================================================
// RUNWITHSTATE TESTS
// ============================================================================

// TestRunWithState_CreatesState tests RunWithState creates proper state.
func TestRunWithState_CreatesState(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "hook.sh")
	outputFile := filepath.Join(tempDir, "output")

	// Script that copies stdin to output file
	script := "#!/bin/sh\ncat > " + outputFile + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	hooks := &spec.Hooks{
		Prestart: []spec.Hook{
			{Path: scriptPath},
		},
	}

	err := RunWithState(hooks, Prestart, "my-container", 999, "/my/bundle", spec.StatusRunning)
	if err != nil {
		t.Fatalf("RunWithState failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	got := string(content)
	if !contains(got, `"id":"my-container"`) {
		t.Errorf("state missing id: %s", got)
	}
	if !contains(got, `"pid":999`) {
		t.Errorf("state missing pid: %s", got)
	}
	if !contains(got, `"bundle":"/my/bundle"`) {
		t.Errorf("state missing bundle: %s", got)
	}
	if !contains(got, `"status":"running"`) {
		t.Errorf("state missing status: %s", got)
	}
}

// TestRunWithState_NilHooks tests RunWithState with nil hooks.
func TestRunWithState_NilHooks(t *testing.T) {
	err := RunWithState(nil, Prestart, "container", 1, "/bundle", spec.StatusRunning)
	if err != nil {
		t.Errorf("nil hooks should not error: %v", err)
	}
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
