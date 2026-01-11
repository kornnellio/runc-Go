// Package hooks implements OCI lifecycle hooks.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"runc-go/spec"
)

// HookType identifies the type of hook.
type HookType string

const (
	// Prestart hooks (deprecated, use CreateRuntime)
	Prestart HookType = "prestart"

	// CreateRuntime hooks run after namespaces created, before pivot_root
	CreateRuntime HookType = "createRuntime"

	// CreateContainer hooks run after pivot_root, before user process
	CreateContainer HookType = "createContainer"

	// StartContainer hooks run after start, before user process executes
	StartContainer HookType = "startContainer"

	// Poststart hooks run after user process starts
	Poststart HookType = "poststart"

	// Poststop hooks run after container stops
	Poststop HookType = "poststop"
)

// Run executes all hooks of the given type.
func Run(hooks *spec.Hooks, hookType HookType, state *spec.State) error {
	if hooks == nil {
		return nil
	}

	var hookList []spec.Hook
	switch hookType {
	case Prestart:
		hookList = hooks.Prestart
	case CreateRuntime:
		hookList = hooks.CreateRuntime
	case CreateContainer:
		hookList = hooks.CreateContainer
	case StartContainer:
		hookList = hooks.StartContainer
	case Poststart:
		hookList = hooks.Poststart
	case Poststop:
		hookList = hooks.Poststop
	default:
		return fmt.Errorf("unknown hook type: %s", hookType)
	}

	for _, hook := range hookList {
		if err := runHook(hook, state); err != nil {
			return fmt.Errorf("%s hook %s: %w", hookType, hook.Path, err)
		}
	}

	return nil
}

// runHook executes a single hook.
func runHook(hook spec.Hook, state *spec.State) error {
	// Serialize state to JSON for stdin
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	// Create command
	args := hook.Args
	if len(args) == 0 {
		args = []string{hook.Path}
	}

	cmd := exec.Command(hook.Path, args[1:]...)
	cmd.Stdin = bytes.NewReader(stateJSON)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), hook.Env...)

	// Handle timeout
	if hook.Timeout != nil && *hook.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*hook.Timeout)*time.Second)
		defer cancel()

		cmd = exec.CommandContext(ctx, hook.Path, args[1:]...)
		cmd.Stdin = bytes.NewReader(stateJSON)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), hook.Env...)
	}

	// Run hook
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// RunWithState is a convenience function that creates state and runs hooks.
func RunWithState(hooks *spec.Hooks, hookType HookType, id string, pid int, bundle string, status spec.ContainerStatus) error {
	state := &spec.State{
		Version: spec.Version,
		ID:      id,
		Status:  status,
		Pid:     pid,
		Bundle:  bundle,
	}
	return Run(hooks, hookType, state)
}
