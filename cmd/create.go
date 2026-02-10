package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"runc-go/container"
)

var createCmd = &cobra.Command{
	Use:   "create <container-id>",
	Short: "Create a container",
	Long: `Create a container from a bundle directory.
The container will be in the "created" state, waiting for 'start' to be called.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

var (
	createBundle        string
	createPidFile       string
	createConsoleSocket string
	createNoPivot       bool
	createNoNewKeyring  bool
)

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&createBundle, "bundle", "b", ".", "path to the root of the bundle directory")
	createCmd.Flags().StringVar(&createPidFile, "pid-file", "", "path to write the container PID to")
	createCmd.Flags().StringVar(&createConsoleSocket, "console-socket", "", "path to a socket for receiving the console file descriptor")
	createCmd.Flags().BoolVar(&createNoPivot, "no-pivot", false, "do not use pivot root to jail process inside rootfs")
	createCmd.Flags().BoolVar(&createNoNewKeyring, "no-new-keyring", false, "do not create a new session keyring")
}

func runCreate(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	containerID := args[0]

	c, err := container.New(ctx, containerID, createBundle, GetStateRoot())
	if err != nil {
		return err
	}

	opts := &container.CreateOptions{
		PidFile:       createPidFile,
		ConsoleSocket: createConsoleSocket,
		NoPivot:       createNoPivot,
		NoNewKeyring:  createNoNewKeyring,
	}

	if err := c.Create(ctx, opts); err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	return nil
}
