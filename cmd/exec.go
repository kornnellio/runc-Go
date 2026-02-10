package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var execCmd = &cobra.Command{
	Use:   "exec <container-id> <command> [args...]",
	Short: "Execute a command in a running container",
	Long:  `Execute a new process inside a running container.`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExec,
}

var (
	execTty           bool
	execCwd           string
	execDetach        bool
	execPidFile       string
	execConsoleSocket string
	execEnv           []string
	execProcess       string
	execUser          string
)

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.Flags().BoolVarP(&execTty, "tty", "t", false, "allocate a pseudo-TTY")
	execCmd.Flags().StringVar(&execCwd, "cwd", "", "working directory inside the container")
	execCmd.Flags().BoolVarP(&execDetach, "detach", "d", false, "detach from the container's process")
	execCmd.Flags().StringVar(&execPidFile, "pid-file", "", "path to write the process PID to")
	execCmd.Flags().StringVar(&execConsoleSocket, "console-socket", "", "path to a socket for receiving the console file descriptor")
	execCmd.Flags().StringArrayVarP(&execEnv, "env", "e", nil, "set environment variables")
	execCmd.Flags().StringVarP(&execProcess, "process", "p", "", "path to process.json file")
	execCmd.Flags().StringVarP(&execUser, "user", "u", "", "user to execute as (uid:gid)")
}

func runExec(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	opts := &container.ExecOptions{
		Tty:           execTty,
		Cwd:           execCwd,
		Detach:        execDetach,
		PidFile:       execPidFile,
		ConsoleSocket: execConsoleSocket,
		Env:           execEnv,
		User:          execUser,
	}

	// Check if --process flag is used (Docker/containerd style)
	if execProcess != "" {
		return container.ExecWithProcessFile(ctx, containerID, GetStateRoot(), execProcess, opts)
	}

	// Get command to execute
	if len(args) < 2 {
		return fmt.Errorf("command required")
	}
	execArgs := args[1:]

	return container.Exec(ctx, containerID, GetStateRoot(), execArgs, opts)
}
