// Package container implements the state operation.
package container

import (
	"encoding/json"
	"fmt"
	"os"
)

// State returns the OCI-compliant state and prints it to stdout.
func State(id, stateRoot string) error {
	c, err := Load(id, stateRoot)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	// Refresh status based on actual process state
	c.RefreshStatus()

	// Get OCI state
	state := c.GetState()

	// Encode as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(state)
}

// StateJSON returns the container state as a JSON string.
func StateJSON(id, stateRoot string) (string, error) {
	c, err := Load(id, stateRoot)
	if err != nil {
		return "", fmt.Errorf("load container: %w", err)
	}

	c.RefreshStatus()
	data, err := c.StateJSON()
	if err != nil {
		return "", err
	}

	return string(data), nil
}
