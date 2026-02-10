// Package logging provides structured logging for the runc-go container runtime.
//
// This package uses Go's standard library log/slog for structured, leveled logging.
// It supports both text and JSON output formats, and integrates with context.Context
// for request-scoped logging.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

// ctxKey is the context key for the logger.
type ctxKey struct{}

var (
	// defaultLogger is the global logger instance.
	defaultLogger *slog.Logger
	// loggerMu protects defaultLogger.
	loggerMu sync.RWMutex
)

func init() {
	// Initialize with a default logger (text to stderr, info level)
	defaultLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Config holds the logger configuration.
type Config struct {
	// Level is the minimum log level (debug, info, warn, error).
	Level slog.Level
	// Format is the output format ("text" or "json").
	Format string
	// Output is the log output destination.
	Output io.Writer
	// AddSource adds source file information to log entries.
	AddSource bool
}

// NewLogger creates a new structured logger with the given configuration.
func NewLogger(cfg Config) *slog.Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stderr
	}

	opts := &slog.HandlerOptions{
		Level:     cfg.Level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	} else {
		handler = slog.NewTextHandler(cfg.Output, opts)
	}

	return slog.New(handler)
}

// SetDefault sets the default global logger.
func SetDefault(logger *slog.Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	defaultLogger = logger
}

// Default returns the default global logger.
func Default() *slog.Logger {
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return defaultLogger
}

// WithContainer returns a logger with container context.
func WithContainer(logger *slog.Logger, id string) *slog.Logger {
	return logger.With(slog.String("container_id", id))
}

// WithOperation returns a logger with operation context.
func WithOperation(logger *slog.Logger, op string) *slog.Logger {
	return logger.With(slog.String("operation", op))
}

// WithPID returns a logger with process ID context.
func WithPID(logger *slog.Logger, pid int) *slog.Logger {
	return logger.With(slog.Int("pid", pid))
}

// WithPath returns a logger with file path context.
func WithPath(logger *slog.Logger, path string) *slog.Logger {
	return logger.With(slog.String("path", path))
}

// ContextWithLogger returns a new context with the logger attached.
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext retrieves the logger from context.
// If no logger is found, returns the default logger.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return logger
	}
	return Default()
}

// ParseLevel parses a log level string and returns the corresponding slog.Level.
// Valid values: "debug", "info", "warn", "error".
// Returns slog.LevelInfo for invalid values.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Helper functions for common log patterns.

// Info logs an info message using the default logger.
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error logs an error message using the default logger.
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// Debug logs a debug message using the default logger.
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// InfoContext logs an info message using the logger from context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).InfoContext(ctx, msg, args...)
}

// WarnContext logs a warning message using the logger from context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).WarnContext(ctx, msg, args...)
}

// ErrorContext logs an error message using the logger from context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).ErrorContext(ctx, msg, args...)
}

// DebugContext logs a debug message using the logger from context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).DebugContext(ctx, msg, args...)
}
