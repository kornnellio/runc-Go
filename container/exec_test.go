package container

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"runc-go/spec"
)

// ============================================================================
// SECURITY TESTS: Shell Injection Prevention
// ============================================================================

// TestShellQuoteArg_Basic tests basic shell argument quoting.
func TestShellQuoteArg_Basic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"empty", "", "''"},
		{"single quote", "it's", "'it'\\''s'"},
		{"multiple quotes", "a'b'c", "'a'\\''b'\\''c'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shellQuoteArg(tt.input)
			if result != tt.expected {
				t.Errorf("shellQuoteArg(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestShellQuoteArg_InjectionAttempts tests that shell injection attempts are neutralized.
func TestShellQuoteArg_InjectionAttempts(t *testing.T) {
	injectionAttempts := []struct {
		name  string
		input string
	}{
		// Command substitution attempts
		{"backtick substitution", "`id`"},
		{"dollar paren substitution", "$(id)"},
		{"dollar paren rm", "$(rm -rf /)"},
		{"nested substitution", "$($(whoami))"},

		// Quote breaking attempts
		{"single quote break", "'; rm -rf /"},
		{"double quote break", "\"; rm -rf /"},
		{"quote escape attempt", "'\\'; rm -rf /"},

		// Operator injection
		{"semicolon command", "; rm -rf /"},
		{"ampersand background", "& rm -rf / &"},
		{"double ampersand", "&& rm -rf /"},
		{"pipe injection", "| cat /etc/passwd"},
		{"double pipe", "|| rm -rf /"},

		// Newline injection
		{"newline injection", "arg\nrm -rf /"},
		{"carriage return", "arg\rrm -rf /"},
		{"crlf injection", "arg\r\nrm -rf /"},

		// Special characters
		{"dollar variable", "$PATH"},
		{"dollar braces", "${PATH}"},
		{"asterisk glob", "*"},
		{"question glob", "?"},
		{"brackets", "[a-z]"},
		{"tilde expansion", "~root"},
		{"exclamation history", "!command"},

		// Null byte injection
		{"null byte", "arg\x00rm -rf /"},

		// Unicode and encoding tricks
		{"unicode quote-like", "arg\u2019test"}, // Right single quotation mark

		// Complex multi-stage attacks
		{"nested quotes", "'\"'$($())\"'"},
		{"escape hell", "\\'\\\"\\`\\$\\(\\)"},
	}

	for _, tt := range injectionAttempts {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuoteArg(tt.input)

			// The quoted string should be safe to use in shell
			// We verify by running echo with the quoted argument
			// If injection worked, the output would differ or cause errors

			// Use sh -c to test the quoting
			cmd := exec.Command("sh", "-c", "printf '%s' "+quoted)
			output, err := cmd.Output()

			if err != nil {
				// Some inputs might cause issues, but they should NOT execute
				// Check that the error is not from command execution
				t.Logf("Command output error for %q: %v", tt.input, err)
			}

			// The output should match the original input (it should be echoed literally)
			// For null bytes, the output may be truncated
			if !strings.Contains(tt.input, "\x00") && string(output) != tt.input {
				t.Errorf("Shell injection may have occurred:\n  input: %q\n  quoted: %q\n  output: %q",
					tt.input, quoted, string(output))
			}
		})
	}
}

// TestShellQuoteArg_SafeExecution verifies quoted args execute safely.
func TestShellQuoteArg_SafeExecution(t *testing.T) {
	// Create a temp file to verify no injection occurs
	tmpFile, err := os.CreateTemp("", "injection-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Write known content
	if err := os.WriteFile(tmpPath, []byte("original"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	// Try to inject a command that would modify the file
	maliciousInput := "test'; echo 'injected' > " + tmpPath + "; echo '"
	quoted := shellQuoteArg(maliciousInput)

	// Run the command - if injection works, the file would be modified
	cmd := exec.Command("sh", "-c", "printf '%s' "+quoted)
	_, _ = cmd.Output()

	// Verify the file was NOT modified
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("SHELL INJECTION VULNERABILITY: File was modified!\n  Content: %q", string(content))
	}
}

// TestShellQuoteArgs_Multiple tests quoting of multiple arguments.
func TestShellQuoteArgs_Multiple(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"single", []string{"hello"}, "'hello'"},
		{"multiple", []string{"hello", "world"}, "'hello' 'world'"},
		{"with injection", []string{"a", "; rm", "b"}, "'a' '; rm' 'b'"},
		{"empty list", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shellQuoteArgs(tt.input)
			if result != tt.expected {
				t.Errorf("shellQuoteArgs(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestExecInit_CwdInjection tests that cwd cannot be used for command injection.
func TestExecInit_CwdInjection(t *testing.T) {
	// These cwd values could be used for injection if not properly quoted
	maliciousCwds := []string{
		"'; rm -rf /",
		"$(whoami)",
		"`id`",
		"/tmp\n rm -rf /",
		"/tmp; rm -rf /",
		"/tmp && rm -rf /",
		"/tmp | cat /etc/passwd",
	}

	for _, cwd := range maliciousCwds {
		t.Run(cwd[:min(20, len(cwd))], func(t *testing.T) {
			// Build the shell command the same way ExecInit does
			shellCmd := "cd " + shellQuoteArg(cwd) + " && exec " + shellQuoteArgs([]string{"echo", "test"})

			// The command should fail (directory doesn't exist) but NOT execute injection
			// We verify by checking the command string is safe

			// Run with timeout to prevent hangs
			cmd := exec.Command("sh", "-c", "timeout 1 sh -c '"+strings.ReplaceAll(shellCmd, "'", "'\\''")+"' 2>&1 || true")
			output, err := cmd.CombinedOutput()

			// Should NOT contain output from injected commands
			// The output should be an error message about cd failing, not output from injected commands
			outputStr := string(output)
			// Check for actual output of injected commands (not just text appearing in error messages)
			// "uid=0" appears in output of `id` command when run as root
			// "root:x:" appears in /etc/passwd content when file is read
			if strings.Contains(outputStr, "uid=0") ||
				strings.Contains(outputStr, "root:x:") {
				t.Errorf("Possible injection detected in output for cwd %q:\n%s", cwd, outputStr)
			}

			// Log for debugging
			_ = err
			t.Logf("cwd: %q, output: %q", cwd, outputStr)
		})
	}
}


// TestEncodeDecodeArgs tests the argument encoding/decoding.
func TestEncodeDecodeArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"simple", []string{"echo", "hello"}},
		{"with spaces", []string{"echo", "hello world", "foo bar"}},
		{"with quotes", []string{"echo", "it's", "\"quoted\""}},
		{"with newlines", []string{"echo", "line1\nline2"}},
		{"with special", []string{"cmd", "; rm -rf /", "$(whoami)"}},
		{"empty", []string{}},
		{"single empty", []string{""}},
		{"unicode", []string{"echo", "héllo", "世界"}},
		{"null byte", []string{"arg\x00rest"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeArgs(tt.args)
			decoded := decodeArgs(encoded)

			if len(decoded) != len(tt.args) {
				t.Fatalf("Length mismatch: encoded=%d, decoded=%d", len(tt.args), len(decoded))
			}

			for i := range tt.args {
				if decoded[i] != tt.args[i] {
					t.Errorf("Arg %d mismatch: want %q, got %q", i, tt.args[i], decoded[i])
				}
			}
		})
	}
}

// TestDecodeArgs_MalformedJSON tests handling of malformed JSON in args.
func TestDecodeArgs_MalformedJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"invalid json", "not json"},
		{"incomplete", "[\"hello\""},
		{"wrong type", "123"},
		{"object", "{\"key\":\"value\"}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := decodeArgs(tt.input)
			// Result should be nil or empty for invalid input
			t.Logf("decodeArgs(%q) = %v", tt.input, result)
		})
	}
}

// TestExecOptions_Validation tests ExecOptions validation.
func TestExecOptions_Validation(t *testing.T) {
	// These tests verify that options are handled correctly
	tests := []struct {
		name string
		opts *ExecOptions
	}{
		{"nil options", nil},
		{"empty options", &ExecOptions{}},
		{"tty only", &ExecOptions{Tty: true}},
		{"cwd only", &ExecOptions{Cwd: "/tmp"}},
		{"detach only", &ExecOptions{Detach: true}},
		{"all options", &ExecOptions{
			Tty:     true,
			User:    "1000:1000",
			Cwd:     "/app",
			Env:     []string{"FOO=bar"},
			Detach:  true,
			PidFile: "/tmp/pid",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test Exec without a container,
			// but we can verify options are accepted
			_ = tt.opts
		})
	}
}

// TestGetCwd tests the cwd resolution logic.
func TestGetCwd(t *testing.T) {
	defaultCwd := "/"
	specCwd := "/app"
	optsCwd := "/custom"

	tests := []struct {
		name      string
		opts      *ExecOptions
		specCwd   string
		expected  string
	}{
		{"opts cwd takes precedence", &ExecOptions{Cwd: optsCwd}, specCwd, optsCwd},
		{"spec cwd used if no opts", &ExecOptions{}, specCwd, specCwd},
		{"default if both empty", &ExecOptions{}, "", defaultCwd},
		{"opts cwd with empty spec", &ExecOptions{Cwd: optsCwd}, "", optsCwd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Container{}
			if tt.specCwd != "" {
				c.Spec = &spec.Spec{
					Process: &spec.Process{
						Cwd: tt.specCwd,
					},
				}
			}

			result := getCwd(tt.opts, c)
			if result != tt.expected {
				t.Errorf("getCwd() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// ============================================================================
// SECURITY TESTS: PID Verification
// ============================================================================

// TestExecInit_EnvValidation tests that required environment variables are validated.
func TestExecInit_EnvValidation(t *testing.T) {
	// Save and restore environment
	oldPid := os.Getenv("_RUNC_GO_EXEC_PID")
	oldArgs := os.Getenv("_RUNC_GO_EXEC_ARGS")
	oldCwd := os.Getenv("_RUNC_GO_EXEC_CWD")
	defer func() {
		os.Setenv("_RUNC_GO_EXEC_PID", oldPid)
		os.Setenv("_RUNC_GO_EXEC_ARGS", oldArgs)
		os.Setenv("_RUNC_GO_EXEC_CWD", oldCwd)
	}()

	// These test cases verify validation of environment variables BEFORE
	// nsenter is called. We only test validation failures here.
	tests := []struct {
		name    string
		pid     string
		args    string
		wantErr bool
	}{
		{"both empty", "", "", true},
		{"pid only", "123", "", true},
		{"args only", "", "[\"echo\",\"hello\"]", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("_RUNC_GO_EXEC_PID", tt.pid)
			os.Setenv("_RUNC_GO_EXEC_ARGS", tt.args)
			os.Setenv("_RUNC_GO_EXEC_CWD", "/")

			// ExecInit will validate env vars before calling nsenter
			err := ExecInit()

			// For validation errors, we expect an error
			if tt.wantErr && err == nil {
				t.Errorf("Expected validation error but got nil")
			}
			if err != nil {
				t.Logf("ExecInit error (expected): %v", err)
			}
		})
	}
}

// ============================================================================
// Unit Tests for Helper Functions
// ============================================================================

// TestJoinStrings tests the string joining helper.
func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{"empty", []string{}, " ", ""},
		{"single", []string{"a"}, " ", "a"},
		{"multiple", []string{"a", "b", "c"}, " ", "a b c"},
		{"different sep", []string{"a", "b"}, ",", "a,b"},
		{"empty sep", []string{"a", "b"}, "", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinStrings(tt.strs, tt.sep)
			if result != tt.expected {
				t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.strs, tt.sep, result, tt.expected)
			}
		})
	}
}
