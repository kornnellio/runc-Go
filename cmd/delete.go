package cmd

import (
	"github.com/spf13/cobra"

	"runc-go/container"
)

var deleteCmd = &cobra.Command{
	Use:     "delete <container-id>",
	Aliases: []string{"rm"},
	Short:   "Delete a container",
	Long:    `Delete any resources held by the container.`,
	Args:    cobra.ExactArgs(1),
	RunE:    runDelete,
}

var deleteForce bool

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "force delete the container if it is still running")
}

func runDelete(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	opts := &container.DeleteOptions{
		Force: deleteForce,
	}

	return container.Delete(ctx, containerID, GetStateRoot(), opts)
}
