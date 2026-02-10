package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var runCmd = &cobra.Command{
	Use:   "run <container-id>",
	Short: "Create and run a container",
	Long:  `Create and run a container in a single operation.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runRun,
}

var (
	runBundle        string
	runPidFile       string
	runConsoleSocket string
	runDetach        bool
)

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&runBundle, "bundle", "b", ".", "path to the root of the bundle directory")
	runCmd.Flags().StringVar(&runPidFile, "pid-file", "", "path to write the container PID to")
	runCmd.Flags().StringVar(&runConsoleSocket, "console-socket", "", "path to a socket for receiving the console file descriptor")
	runCmd.Flags().BoolVarP(&runDetach, "detach", "d", false, "detach from the container's process")
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	c, err := container.New(ctx, containerID, runBundle, GetStateRoot())
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	opts := &container.CreateOptions{
		PidFile:       runPidFile,
		ConsoleSocket: runConsoleSocket,
	}

	if err := c.Run(ctx, opts); err != nil {
		return fmt.Errorf("run container: %w", err)
	}

	if runDetach {
		return nil
	}

	// Wait for container to exit
	code, err := c.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait for container: %w", err)
	}

	os.Exit(code)
	return nil
}
