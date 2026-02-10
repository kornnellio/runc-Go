package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var startCmd = &cobra.Command{
	Use:   "start <container-id>",
	Short: "Start a created container",
	Long:  `Start a container that has been created with 'create'.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	c, err := container.Load(ctx, containerID, GetStateRoot())
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	return nil
}
