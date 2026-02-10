package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print version information for runc-go.`,
	Args:  cobra.NoArgs,
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("runc-go version %s\n", Version)
	fmt.Printf("spec: %s\n", SpecVer)
	fmt.Printf("go: %s\n", runtime.Version())
	if BuildTime != "unknown" {
		fmt.Printf("build: %s\n", BuildTime)
	}
}
