package cmd

import (
	"fmt"
	"syscall"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var killCmd = &cobra.Command{
	Use:   "kill <container-id> [signal]",
	Short: "Send a signal to a container",
	Long:  `Send the specified signal to the container's init process. Default signal is SIGTERM.`,
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runKill,
}

var killAll bool

func init() {
	rootCmd.AddCommand(killCmd)

	killCmd.Flags().BoolVarP(&killAll, "all", "a", false, "send signal to all processes in the container")
}

func runKill(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	sigStr := "SIGTERM"
	if len(args) > 1 {
		sigStr = args[1]
	}

	sig, err := container.ParseSignal(sigStr)
	if err != nil {
		return fmt.Errorf("parse signal: %w", err)
	}

	return container.Kill(ctx, containerID, GetStateRoot(), syscall.Signal(sig), killAll)
}
