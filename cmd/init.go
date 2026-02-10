package cmd

import (
	"github.com/spf13/cobra"

	"runc-go/container"
)

var initCmd = &cobra.Command{
	Use:    "init",
	Short:  "Initialize the container (internal use)",
	Long:   `Internal command called inside the container namespace to complete setup.`,
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runInit,
}

var execInitCmd = &cobra.Command{
	Use:    "exec-init",
	Short:  "Initialize exec in container (internal use)",
	Long:   `Internal command called to join container namespaces and exec.`,
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runExecInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(execInitCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	return container.InitContainer()
}

func runExecInit(cmd *cobra.Command, args []string) error {
	return container.ExecInit()
}
