package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

func TestNewLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Output: &buf,
	})

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, `"msg":"test message"`) {
		t.Errorf("Expected JSON output to contain msg field, got: %s", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("Expected JSON output to contain key field, got: %s", output)
	}
}

func TestNewLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelWarn,
		Format: "text",
		Output: &buf,
	})

	// Info should be filtered out
	logger.Info("info message")
	if strings.Contains(buf.String(), "info message") {
		t.Error("Info message should be filtered at Warn level")
	}

	// Warn should be logged
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Error("Warn message should be logged at Warn level")
	}
}

func TestWithContainer(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	containerLogger := WithContainer(logger, "test-container")
	containerLogger.Info("container message")

	output := buf.String()
	if !strings.Contains(output, "container_id=test-container") {
		t.Errorf("Expected container_id in output, got: %s", output)
	}
}

func TestWithOperation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	opLogger := WithOperation(logger, "create")
	opLogger.Info("operation message")

	output := buf.String()
	if !strings.Contains(output, "operation=create") {
		t.Errorf("Expected operation in output, got: %s", output)
	}
}

func TestWithPID(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	pidLogger := WithPID(logger, 12345)
	pidLogger.Info("pid message")

	output := buf.String()
	if !strings.Contains(output, "pid=12345") {
		t.Errorf("Expected pid in output, got: %s", output)
	}
}

func TestWithPath(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	pathLogger := WithPath(logger, "/some/path")
	pathLogger.Info("path message")

	output := buf.String()
	if !strings.Contains(output, "path=/some/path") {
		t.Errorf("Expected path in output, got: %s", output)
	}
}

func TestContextWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	ctx := ContextWithLogger(context.Background(), logger)
	retrieved := FromContext(ctx)

	if retrieved != logger {
		t.Error("Expected to retrieve the same logger from context")
	}

	// Test logging via context
	retrieved.Info("context message")
	if !strings.Contains(buf.String(), "context message") {
		t.Error("Expected message to be logged via context logger")
	}
}

func TestFromContext_Default(t *testing.T) {
	// FromContext should return default logger when no logger in context
	ctx := context.Background()
	logger := FromContext(ctx)

	if logger == nil {
		t.Error("Expected non-nil default logger")
	}
	if logger != Default() {
		t.Error("Expected default logger when no logger in context")
	}
}

func TestSetDefault(t *testing.T) {
	var buf bytes.Buffer
	newLogger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "text",
		Output: &buf,
	})

	oldDefault := Default()
	SetDefault(newLogger)
	defer SetDefault(oldDefault) // Restore

	if Default() != newLogger {
		t.Error("SetDefault did not change the default logger")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo}, // Default to info
		{"", slog.LevelInfo},        // Default to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelDebug,
		Format: "text",
		Output: &buf,
	})

	oldDefault := Default()
	SetDefault(logger)
	defer SetDefault(oldDefault)

	// Test each helper function
	Info("info message")
	if !strings.Contains(buf.String(), "INFO") || !strings.Contains(buf.String(), "info message") {
		t.Errorf("Info() failed, output: %s", buf.String())
	}
	buf.Reset()

	Warn("warn message")
	if !strings.Contains(buf.String(), "WARN") || !strings.Contains(buf.String(), "warn message") {
		t.Errorf("Warn() failed, output: %s", buf.String())
	}
	buf.Reset()

	Error("error message")
	if !strings.Contains(buf.String(), "ERROR") || !strings.Contains(buf.String(), "error message") {
		t.Errorf("Error() failed, output: %s", buf.String())
	}
	buf.Reset()

	Debug("debug message")
	if !strings.Contains(buf.String(), "DEBUG") || !strings.Contains(buf.String(), "debug message") {
		t.Errorf("Debug() failed, output: %s", buf.String())
	}
}

func TestContextHelperFunctions(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelDebug,
		Format: "text",
		Output: &buf,
	})

	ctx := ContextWithLogger(context.Background(), logger)

	InfoContext(ctx, "info context message")
	if !strings.Contains(buf.String(), "info context message") {
		t.Errorf("InfoContext() failed, output: %s", buf.String())
	}
	buf.Reset()

	WarnContext(ctx, "warn context message")
	if !strings.Contains(buf.String(), "warn context message") {
		t.Errorf("WarnContext() failed, output: %s", buf.String())
	}
	buf.Reset()

	ErrorContext(ctx, "error context message")
	if !strings.Contains(buf.String(), "error context message") {
		t.Errorf("ErrorContext() failed, output: %s", buf.String())
	}
	buf.Reset()

	DebugContext(ctx, "debug context message")
	if !strings.Contains(buf.String(), "debug context message") {
		t.Errorf("DebugContext() failed, output: %s", buf.String())
	}
}

func TestChainedWith(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Config{
		Level:  slog.LevelInfo,
		Format: "json",
		Output: &buf,
	})

	// Chain multiple With calls
	chainedLogger := WithContainer(WithOperation(WithPID(logger, 1234), "exec"), "my-container")
	chainedLogger.Info("chained message")

	output := buf.String()
	if !strings.Contains(output, `"container_id":"my-container"`) {
		t.Errorf("Missing container_id in output: %s", output)
	}
	if !strings.Contains(output, `"operation":"exec"`) {
		t.Errorf("Missing operation in output: %s", output)
	}
	if !strings.Contains(output, `"pid":1234`) {
		t.Errorf("Missing pid in output: %s", output)
	}
}
