package cmd

import (
	"github.com/spf13/cobra"

	"runc-go/container"
)

var stateCmd = &cobra.Command{
	Use:   "state <container-id>",
	Short: "Output the state of a container",
	Long:  `Output the OCI-compliant state of a container as JSON.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runState,
}

func init() {
	rootCmd.AddCommand(stateCmd)
}

func runState(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	return container.State(ctx, containerID, GetStateRoot())
}
