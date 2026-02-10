// runc-go is an OCI-compliant container runtime.
//
// This is an educational implementation that follows the OCI Runtime Specification.
// It can be used as a drop-in replacement for runc with Docker or other container engines.
package main

import (
	"fmt"
	"os"

	"runc-go/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
