// Package cmd implements the CLI commands for runc-go.
package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"runc-go/logging"
)

// Version information set at build time
var (
	Version   = "0.1.0"
	SpecVer   = "1.0.2"
	BuildTime = "unknown"
)

// Global flags
var (
	globalRoot      string
	globalLog       string
	globalLogFormat string
	globalDebug     bool
)

// rootCmd is the base command for runc-go.
var rootCmd = &cobra.Command{
	Use:   "runc-go",
	Short: "OCI container runtime",
	Long: `runc-go is an OCI-compliant container runtime.

This implementation follows the OCI Runtime Specification and can be used
as a drop-in replacement for runc with Docker or other container engines.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Setup logging
		setupLogging()
		return nil
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// GetContext returns a context that cancels on SIGINT/SIGTERM.
func GetContext() context.Context {
	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	return ctx
}

// GetStateRoot returns the state root directory.
func GetStateRoot() string {
	if globalRoot != "" {
		return globalRoot
	}
	return "/run/runc-go"
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&globalRoot, "root", "", "root directory for storage of container state (default: /run/runc-go)")
	rootCmd.PersistentFlags().StringVar(&globalLog, "log", "", "set the log file path")
	rootCmd.PersistentFlags().StringVar(&globalLogFormat, "log-format", "text", "set the format for log output (text or json)")
	rootCmd.PersistentFlags().BoolVar(&globalDebug, "debug", false, "enable debug logging")

	// Compatibility flags (accepted but may be ignored)
	rootCmd.PersistentFlags().Bool("systemd-cgroup", false, "enable systemd cgroup support (compatibility flag)")
}

func setupLogging() {
	var logOutput = os.Stderr
	if globalLog != "" {
		f, err := os.OpenFile(globalLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			logOutput = f
		}
	}

	logLevel := slog.LevelInfo
	if globalDebug {
		logLevel = slog.LevelDebug
	}

	if globalLogFormat == "json" || globalLog != "" {
		logger := logging.NewLogger(logging.Config{
			Level:  logLevel,
			Format: globalLogFormat,
			Output: logOutput,
		})
		logging.SetDefault(logger)
	}
}
