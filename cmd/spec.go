package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"runc-go/spec"
)

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Create a new specification file",
	Long:  `Generate a default OCI runtime specification (config.json) to stdout.`,
	Args:  cobra.NoArgs,
	RunE:  runSpec,
}

var (
	specBundle   string
	specRootless bool
)

func init() {
	rootCmd.AddCommand(specCmd)

	specCmd.Flags().StringVarP(&specBundle, "bundle", "b", ".", "bundle directory")
	specCmd.Flags().BoolVar(&specRootless, "rootless", false, "generate a rootless spec")
}

func runSpec(cmd *cobra.Command, args []string) error {
	s := spec.DefaultSpec()

	if specRootless {
		// Add user namespace for rootless
		s.Linux.Namespaces = append(s.Linux.Namespaces, spec.LinuxNamespace{
			Type: spec.UserNamespace,
		})

		// Add UID/GID mappings
		uid := uint32(os.Getuid())
		gid := uint32(os.Getgid())
		s.Linux.UIDMappings = []spec.LinuxIDMapping{
			{ContainerID: 0, HostID: uid, Size: 1},
		}
		s.Linux.GIDMappings = []spec.LinuxIDMapping{
			{ContainerID: 0, HostID: gid, Size: 1},
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s)
}
