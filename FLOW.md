# Execution Flow Guide

A detailed walkthrough of how runc-go executes, with code navigation.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture Overview](#architecture-overview)
3. [Command Dispatch (Cobra)](#command-dispatch-cobra)
4. [The `run` Command](#the-run-command)
5. [The `create` Command](#the-create-command)
6. [The `start` Command](#the-start-command)
7. [The `exec` Command](#the-exec-command)
8. [Container Init Process](#container-init-process)
9. [Namespace Setup](#namespace-setup)
10. [Rootfs Setup](#rootfs-setup)
11. [Cgroup Setup](#cgroup-setup)
12. [Security Setup](#security-setup)
13. [Context Propagation](#context-propagation)
14. [Error Handling Flow](#error-handling-flow)
15. [Logging Integration](#logging-integration)

---

## Overview

When you run a container, here's the high-level flow:

```
User runs: sudo runc-go run mycontainer --bundle /bundle

main.go                     Call cmd.Execute()
    │
    ▼
cmd/root.go                 Cobra parses flags, dispatches to runRun()
    │
    ▼
cmd/run.go                  Creates context with signal handling
    │
    ▼
container/create.go         Fork child with namespace flags
    │
    ├──► Parent process     Wait for child, manage state
    │
    └──► Child process      Setup isolation, exec user command
            │
            ▼
        linux/namespace.go  Create/join namespaces
            │
            ▼
        linux/rootfs.go     pivot_root, mount filesystems
            │
            ▼
        linux/cgroup.go     Apply resource limits
            │
            ▼
        linux/capabilities.go  Drop privileges
            │
            ▼
        linux/seccomp.go    Install syscall filter
            │
            ▼
        syscall.Exec()      Replace with user's command
```

---

## Architecture Overview

### Package Structure

```
runc-go/
├── main.go              Entry point - calls cmd.Execute()
├── cmd/                 Cobra CLI commands
│   ├── root.go          Root command, global flags, context/logging setup
│   ├── create.go        create command
│   ├── start.go         start command
│   ├── run.go           run command (create + start + wait)
│   ├── exec.go          exec command
│   ├── kill.go          kill command
│   ├── delete.go        delete command
│   ├── list.go          list command
│   ├── state.go         state command
│   ├── spec.go          spec command
│   ├── version.go       version command
│   └── init.go          Internal init/exec-init commands
├── container/           Container lifecycle management
├── linux/               Linux-specific implementations
├── spec/                OCI specification types
├── errors/              Custom error types
├── logging/             Structured logging with slog
└── utils/               Utility functions
```

### Key Design Patterns

1. **Cobra CLI**: All commands use github.com/spf13/cobra for argument parsing
2. **Context Propagation**: context.Context flows through all public APIs
3. **Signal Handling**: SIGINT/SIGTERM trigger graceful cancellation via context
4. **Custom Errors**: Typed errors with Kind, Operation, and Container ID
5. **Structured Logging**: slog-based logging with JSON/text output formats

---

## Command Dispatch (Cobra)

### Entry Point: `main.go`

```go
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
```

### Root Command: `cmd/root.go`

```go
var rootCmd = &cobra.Command{
    Use:   "runc-go",
    Short: "OCI container runtime in Go",
    Long:  `A lightweight OCI-compliant container runtime.`,
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        // Setup logging based on flags
        return setupLogging()
    },
}

// Global flags
func init() {
    rootCmd.PersistentFlags().StringVar(&globalRoot, "root", "/run/runc-go",
        "root directory for storage of container state")
    rootCmd.PersistentFlags().StringVar(&globalLog, "log", "",
        "log file path")
    rootCmd.PersistentFlags().StringVar(&globalLogFormat, "log-format", "text",
        "log format (text, json)")
    rootCmd.PersistentFlags().BoolVar(&globalDebug, "debug", false,
        "enable debug logging")
    rootCmd.PersistentFlags().BoolVar(&globalSystemdCgroup, "systemd-cgroup", false,
        "use systemd cgroup driver")
}
```

### Context Setup: `cmd/root.go`

```go
// GetContext returns a context that cancels on SIGINT/SIGTERM
func GetContext() context.Context {
    ctx, _ := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    return ctx
}

// GetStateRoot returns the state root directory
func GetStateRoot() string {
    return globalRoot
}
```

### Command Registration: Each `cmd/*.go`

```go
// cmd/run.go
var runCmd = &cobra.Command{
    Use:   "run <container-id>",
    Short: "Create and run a container",
    Args:  cobra.ExactArgs(1),
    RunE:  runRun,
}

func init() {
    rootCmd.AddCommand(runCmd)

    runCmd.Flags().StringVarP(&runBundle, "bundle", "b", ".",
        "path to the root of the bundle directory")
    runCmd.Flags().StringVar(&runConsoleSocket, "console-socket", "",
        "path to AF_UNIX socket for console")
    runCmd.Flags().StringVar(&runPidFile, "pid-file", "",
        "path to write container PID")
    runCmd.Flags().BoolVarP(&runDetach, "detach", "d", false,
        "detach from the container's process")
}
```

---

## The `run` Command

**Command:** `sudo runc-go run mycontainer --bundle /bundle`

### Step 1: Cobra Parses Arguments

**File:** `cmd/run.go`

```go
func runRun(cmd *cobra.Command, args []string) error {
    ctx := GetContext()  // Context with signal handling

    containerID := args[0]  // "mycontainer"
    bundle := runBundle     // "/bundle" from --bundle flag
```

### Step 2: Create Container Object

**File:** `cmd/run.go` → `container/container.go:107`

```go
    // Create new container
    c, err := container.New(ctx, containerID, bundle, GetStateRoot())
    if err != nil {
        return err  // Returns *errors.ContainerError
    }
```

**Container.New with Context:** `container/container.go`

```go
func New(ctx context.Context, id, bundle, stateRoot string) (*Container, error) {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Get logger from context
    logger := logging.FromContext(ctx)
    logger.Debug("creating new container", "id", id, "bundle", bundle)

    // Validate container ID (security check)
    if err := ValidateContainerID(id); err != nil {
        return nil, &errors.ContainerError{
            Op:        "New",
            Container: id,
            Kind:      errors.ErrInvalidConfig,
            Err:       err,
        }
    }

    // Load OCI spec from bundle/config.json
    specPath := filepath.Join(bundle, "config.json")
    s, err := spec.LoadSpec(specPath)
    if err != nil {
        return nil, &errors.ContainerError{
            Op:        "New",
            Container: id,
            Kind:      errors.ErrInvalidConfig,
            Err:       err,
        }
    }

    // Create state directory /run/runc-go/<id>/
    stateDir := filepath.Join(stateRoot, id)
    os.MkdirAll(stateDir, 0700)

    // Initialize container struct
    c := &Container{
        ID:       id,
        Bundle:   bundle,
        StateDir: stateDir,
        Spec:     s,
        State: &spec.ContainerState{
            Status: spec.StatusCreating,
            // ...
        },
    }
    return c, nil
}
```

### Step 3: Run (Create + Start + Wait)

**File:** `cmd/run.go` → `container/start.go`

```go
    opts := &container.CreateOptions{
        ConsoleSocket: runConsoleSocket,
        PidFile:       runPidFile,
        Detach:        runDetach,
    }

    // Run handles create + start + wait
    exitCode, err := c.Run(ctx, opts)
    if err != nil {
        return err
    }

    os.Exit(exitCode)
    return nil
}
```

**Container.Run:** `container/start.go`

```go
func (c *Container) Run(ctx context.Context, opts *CreateOptions) (int, error) {
    // Create the container (fork child, setup namespaces)
    if err := Create(ctx, c, opts); err != nil {
        return -1, err
    }

    // Start it (signal child to exec)
    if err := c.Start(ctx); err != nil {
        return -1, err
    }

    // Wait for exit (respects context cancellation)
    return c.Wait(ctx)
}
```

---

## The `create` Command

**Command:** `sudo runc-go create mycontainer --bundle /bundle`

This is the heart of container creation.

### Step 1: Cobra Parses Arguments

**File:** `cmd/create.go`

```go
var createCmd = &cobra.Command{
    Use:   "create <container-id>",
    Short: "Create a container",
    Long:  `Create a container but do not start it.`,
    Args:  cobra.ExactArgs(1),
    RunE:  runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
    ctx := GetContext()
    containerID := args[0]
```

### Step 2: Setup Synchronization

**File:** `container/create.go`

```go
func Create(ctx context.Context, c *Container, opts *CreateOptions) error {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    logger := logging.FromContext(ctx)
    logger.Info("creating container", "id", c.ID)

    s := c.Spec

    // Create FIFO for create/start synchronization
    // Child will block reading this until start is called
    if err := c.CreateExecFifo(); err != nil {
        return &errors.ContainerError{
            Op:        "Create",
            Container: c.ID,
            Kind:      errors.ErrResource,
            Err:       err,
        }
    }
    // Creates /run/runc-go/<id>/exec.fifo
```

### Step 3: Setup Cgroup

**File:** `container/create.go` → `linux/cgroup.go`

```go
    // Create cgroup for resource limits
    cgroupPath := filepath.Join("/sys/fs/cgroup", "runc-go", c.ID)
    cg, err := linux.NewCgroup(ctx, cgroupPath)
    if err != nil {
        return err
    }

    // Apply resource limits from config.json
    if s.Linux != nil && s.Linux.Resources != nil {
        if err := cg.ApplyResources(ctx, s.Linux.Resources); err != nil {
            logger.Warn("failed to apply some resources", "error", err)
        }
    }
```

### Step 4: Setup Console (PTY)

**File:** `container/create.go`

```go
    // If terminal requested, create PTY
    var console *utils.Console
    if s.Process.Terminal {
        console, err = utils.NewConsole()
        if err != nil {
            return err
        }
        // console.Master() = PTY master (parent keeps this)
        // console.SlavePath() = /dev/pts/N (child uses this)
    }
```

### Step 5: Build Process Attributes

**File:** `container/create.go` → `linux/namespace.go`

```go
    // Get self executable for re-exec
    self, _ := os.Executable()

    // Build command: runc-go init
    cmd := exec.Command(self, "init")

    // Build namespace flags from config
    // linux/namespace.go
    attr, err := linux.BuildSysProcAttr(s)
    // Returns &syscall.SysProcAttr{
    //     Cloneflags: CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWUTS | ...,
    //     Setsid:     true,
    // }

    cmd.SysProcAttr = attr
```

### Step 6: Pass Config to Child

**File:** `container/create.go`

```go
    // Pass information via environment variables
    cmd.Env = append(os.Environ(),
        "_RUNC_GO_INIT_PIPE=3",           // Sync pipe FD
        "_RUNC_GO_BUNDLE="+c.Bundle,       // Bundle path
        "_RUNC_GO_STATE_DIR="+c.StateDir,  // State directory
        "_RUNC_GO_CONSOLE="+consolePath,   // PTY slave path
    )

    // Setup pipes for parent-child communication
    cmd.ExtraFiles = []*os.File{syncPipe.Child()}
```

### Step 7: Fork Child Process

**File:** `container/create.go`

```go
    // START THE CHILD PROCESS
    // This forks with CLONE_NEW* flags
    // Child is now in NEW namespaces!
    logger.Debug("starting container init process")
    if err := cmd.Start(); err != nil {
        return &errors.ContainerError{
            Op:        "Create",
            Container: c.ID,
            Kind:      errors.ErrNamespace,
            Err:       err,
        }
    }

    // Save PID
    c.InitProcess = cmd.Process.Pid
    c.State.Pid = c.InitProcess
    logger.Info("container process started", "pid", c.InitProcess)
```

### Step 8: Parent Waits

**File:** `container/create.go`

```go
    // Send PTY master to console socket (if Docker is listening)
    if console != nil && opts.ConsoleSocket != "" {
        utils.SendConsoleToSocket(opts.ConsoleSocket, console.Master())
    }

    // Add child to cgroup
    cg.AddProcess(c.InitProcess)

    // Update state to "created"
    c.State.Status = spec.StatusCreated
    c.SaveState()

    logger.Info("container created", "id", c.ID, "status", "created")
    // Parent is done - child is blocked on exec FIFO
    return nil
```

---

## The `start` Command

**Command:** `sudo runc-go start mycontainer`

### Cobra Handler: `cmd/start.go`

```go
var startCmd = &cobra.Command{
    Use:   "start <container-id>",
    Short: "Start a created container",
    Args:  cobra.ExactArgs(1),
    RunE:  runStart,
}

func runStart(cmd *cobra.Command, args []string) error {
    ctx := GetContext()
    containerID := args[0]

    c, err := container.Load(ctx, containerID, GetStateRoot())
    if err != nil {
        return err
    }

    return c.Start(ctx)
}
```

### Unblock the Child: `container/start.go`

```go
func (c *Container) Start(ctx context.Context) error {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    logger := logging.FromContext(ctx)
    logger.Info("starting container", "id", c.ID)

    // Verify container is in created state
    if c.State.Status != spec.StatusCreated {
        return &errors.ContainerError{
            Op:        "Start",
            Container: c.ID,
            Kind:      errors.ErrInvalidState,
            Err:       errors.ErrContainerNotCreated,
        }
    }

    // Open the exec FIFO for writing
    // This unblocks the child who is reading it
    fifoPath := c.ExecFifoPath()

    f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
    if err != nil {
        return err
    }

    // Write anything - just need to unblock
    f.Write([]byte{0})
    f.Close()

    // Update state
    c.State.Status = spec.StatusRunning
    c.SaveState()

    // Remove FIFO (no longer needed)
    os.Remove(fifoPath)

    logger.Info("container started", "id", c.ID)
    return nil
}
```

---

## The `exec` Command

**Command:** `sudo runc-go exec -t mycontainer /bin/sh`

### Step 1: Cobra Handler

**File:** `cmd/exec.go`

```go
var execCmd = &cobra.Command{
    Use:   "exec <container-id> <command> [args...]",
    Short: "Execute a process in a running container",
    Args:  cobra.MinimumNArgs(2),
    RunE:  runExec,
}

var (
    execTty     bool
    execCwd     string
    execEnv     []string
    execProcess string
)

func init() {
    rootCmd.AddCommand(execCmd)

    execCmd.Flags().BoolVarP(&execTty, "tty", "t", false,
        "allocate a pseudo-TTY")
    execCmd.Flags().StringVar(&execCwd, "cwd", "",
        "current working directory")
    execCmd.Flags().StringArrayVarP(&execEnv, "env", "e", nil,
        "set environment variables")
    execCmd.Flags().StringVarP(&execProcess, "process", "p", "",
        "path to process.json")
}

func runExec(cmd *cobra.Command, args []string) error {
    ctx := GetContext()

    containerID := args[0]
    command := args[1:]
```

### Step 2: Load Container

**File:** `container/exec.go`

```go
func Exec(ctx context.Context, containerID, stateRoot string, args []string, opts *ExecOptions) error {
    // Check context
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    logger := logging.FromContext(ctx)

    // Load existing container
    c, err := Load(ctx, containerID, stateRoot)
    if err != nil {
        return err
    }

    // Verify it's running
    if c.State.Status != spec.StatusRunning {
        return &errors.ContainerError{
            Op:        "Exec",
            Container: containerID,
            Kind:      errors.ErrInvalidState,
            Err:       errors.ErrContainerNotRunning,
        }
    }

    logger.Info("executing in container", "id", containerID, "args", args)
```

### Step 3: Build exec-init Command

**File:** `container/exec.go`

```go
    // Get our executable
    self, _ := os.Executable()

    // Build command: runc-go exec-init
    cmd := exec.Command(self, "exec-init")

    // Pass info via environment
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("_RUNC_GO_EXEC_PID=%d", c.InitProcess),
        fmt.Sprintf("_RUNC_GO_EXEC_ROOTFS=%s", c.State.Rootfs),
        fmt.Sprintf("_RUNC_GO_EXEC_CWD=%s", opts.Cwd),
        fmt.Sprintf("_RUNC_GO_EXEC_ARGS=%s", encodeArgs(args)),
    )
```

### Step 4: Handle TTY

**File:** `container/exec.go`

```go
    if opts.Tty {
        // Create PTY pair
        ptmx, _ := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)

        // Get slave path
        slavePath := fmt.Sprintf("/dev/pts/%d", ptyNum)
        slave, _ := os.OpenFile(slavePath, os.O_RDWR, 0)

        // Child uses slave
        cmd.Stdin = slave
        cmd.Stdout = slave
        cmd.Stderr = slave

        // Put terminal in raw mode
        oldState, _ := term.MakeRaw(int(os.Stdin.Fd()))
        defer term.Restore(int(os.Stdin.Fd()), oldState)

        // Copy I/O
        go io.Copy(ptmx, os.Stdin)   // User input → PTY
        go io.Copy(os.Stdout, ptmx)  // PTY output → User
    }
```

### Step 5: exec-init Joins Namespaces

**File:** `container/exec.go`

```go
func ExecInit() error {
    // Get target PID
    pidStr := os.Getenv("_RUNC_GO_EXEC_PID")
    args := decodeArgs(os.Getenv("_RUNC_GO_EXEC_ARGS"))
    cwd := os.Getenv("_RUNC_GO_EXEC_CWD")

    // Use nsenter to join all namespaces
    nsenterArgs := []string{
        "-t", pidStr,  // Target PID
        "-m",          // Mount namespace
        "-u",          // UTS namespace
        "-i",          // IPC namespace
        "-n",          // Network namespace
        "-p",          // PID namespace
        "--",
    }

    // Add command with proper quoting (security: prevent injection)
    shellCmd := fmt.Sprintf("cd %s && exec %s",
        shellQuoteArg(cwd),
        shellQuoteArgs(args))
    nsenterArgs = append(nsenterArgs, "sh", "-c", shellCmd)

    // Exec nsenter (replaces this process)
    syscall.Exec("/usr/bin/nsenter", nsenterArgs, env)
}
```

---

## Container Init Process

When the child process starts, it runs `runc-go init`.

### Cobra Handler: `cmd/init.go`

```go
var initCmd = &cobra.Command{
    Use:    "init",
    Short:  "Initialize the container (internal use)",
    Long:   `Internal command called inside the container namespace to complete setup.`,
    Hidden: true,  // Not shown in --help
    Args:   cobra.NoArgs,
    RunE:   runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
    // This runs INSIDE the new namespaces
    // but BEFORE pivot_root
    return container.InitContainer()
}
```

### Init Flow: `container/create.go`

```go
func InitContainer() error {
    // Get bundle path from environment
    bundle := os.Getenv("_RUNC_GO_BUNDLE")

    // Load config.json
    specPath := filepath.Join(bundle, "config.json")
    s, _ := spec.LoadSpec(specPath)

    // STEP 1: Open exec FIFO (before pivot_root!)
    // This file is outside the container rootfs
    fifoPath := filepath.Join(os.Getenv("_RUNC_GO_STATE_DIR"), "exec.fifo")
    fifo, _ := os.OpenFile(fifoPath, os.O_RDONLY, 0)
```

### Step 2: Setup Hostname

**File:** `container/create.go` → `linux/namespace.go`

```go
    // Set hostname in UTS namespace
    if s.Hostname != "" {
        linux.SetHostname(s.Hostname)
        // → syscall.Sethostname([]byte(hostname))
    }
```

### Step 3: Setup Rootfs

**File:** `container/create.go` → `linux/rootfs.go`

```go
    // Setup root filesystem
    linux.SetupRootfs(ctx, s, bundle)
    // This does:
    // 1. Bind mount rootfs to itself
    // 2. Setup all mounts from config
    // 3. pivot_root to new root
    // 4. Mount /proc, /dev, /sys
    // 5. Mask sensitive paths
```

### Step 4: Setup Devices

**File:** `container/create.go` → `linux/devices.go`

```go
    // Create device nodes
    linux.SetupDefaultDevices()
    // Creates: /dev/null, /dev/zero, /dev/random, etc.

    // Create symlinks
    linux.SetupDevSymlinks()
    // Creates: /dev/fd → /proc/self/fd, etc.
```

### Step 5: Setup Console

**File:** `container/create.go`

```go
    // If terminal requested, setup PTY slave
    if s.Process.Terminal {
        consolePath := os.Getenv("_RUNC_GO_CONSOLE")

        // Open PTY slave as stdin/stdout/stderr
        console, _ := os.OpenFile(consolePath, os.O_RDWR, 0)

        // Become session leader
        syscall.Setsid()

        // Set as controlling terminal
        utils.SetControllingTerminal(console)

        // Dup to stdin/stdout/stderr
        syscall.Dup2(int(console.Fd()), 0)
        syscall.Dup2(int(console.Fd()), 1)
        syscall.Dup2(int(console.Fd()), 2)
    }
```

### Step 6: Block on FIFO (Wait for Start)

**File:** `container/create.go`

```go
    // READ FROM FIFO - THIS BLOCKS!
    // We wait here until "runc-go start" writes to the FIFO
    buf := make([]byte, 1)
    fifo.Read(buf)
    fifo.Close()

    // Start command received! Continue...
```

### Step 7: Apply Security

**File:** `container/create.go`

```go
    // Drop capabilities
    if s.Process.Capabilities != nil {
        if err := linux.ApplyCapabilities(s.Process.Capabilities); err != nil {
            return &errors.ContainerError{
                Op:   "InitContainer",
                Kind: errors.ErrCapability,
                Err:  err,
            }
        }
    }

    // Install seccomp filter
    if s.Linux != nil && s.Linux.Seccomp != nil {
        if err := linux.SetupSeccomp(s.Linux.Seccomp); err != nil {
            return &errors.ContainerError{
                Op:   "InitContainer",
                Kind: errors.ErrSeccomp,
                Err:  err,
            }
        }
    }

    // Set user/group
    if s.Process.User.UID != 0 || s.Process.User.GID != 0 {
        syscall.Setgid(int(s.Process.User.GID))
        syscall.Setuid(int(s.Process.User.UID))
    }
```

### Step 8: Exec User Command

**File:** `container/create.go`

```go
    // Change to working directory
    os.Chdir(s.Process.Cwd)

    // Build environment
    env := s.Process.Env

    // Find the binary
    binary, _ := exec.LookPath(s.Process.Args[0])

    // EXEC - REPLACE THIS PROCESS WITH USER'S COMMAND
    // After this, the "init" process becomes "/bin/sh" (or whatever)
    syscall.Exec(binary, s.Process.Args, env)

    // If we get here, exec failed
    return fmt.Errorf("exec failed")
}
```

---

## Namespace Setup

### Building Namespace Flags

**File:** `linux/namespace.go`

```go
func NamespaceFlags(namespaces []spec.LinuxNamespace) uintptr {
    var flags uintptr

    for _, ns := range namespaces {
        // Only add flag if path is empty (create NEW namespace)
        // If path is set, we join existing namespace later
        if ns.Path == "" {
            switch ns.Type {
            case spec.PIDNamespace:
                flags |= CLONE_NEWPID    // 0x20000000
            case spec.MountNamespace:
                flags |= CLONE_NEWNS     // 0x00020000
            case spec.UTSNamespace:
                flags |= CLONE_NEWUTS    // 0x04000000
            case spec.IPCNamespace:
                flags |= CLONE_NEWIPC    // 0x08000000
            case spec.NetworkNamespace:
                flags |= CLONE_NEWNET    // 0x40000000
            case spec.UserNamespace:
                flags |= CLONE_NEWUSER   // 0x10000000
            case spec.CgroupNamespace:
                flags |= CLONE_NEWCGROUP // 0x02000000
            }
        }
    }
    return flags
}
```

### Joining Existing Namespaces

**File:** `linux/namespace.go`

```go
func SetNamespaces(namespaces []spec.LinuxNamespace) error {
    for _, ns := range namespaces {
        if ns.Path != "" {
            // Join existing namespace via setns()
            setns(ns.Path, ns.Type)
        }
    }
    return nil
}

func setns(path string, nsType spec.LinuxNamespaceType) error {
    // Open namespace file (e.g., /proc/1234/ns/net)
    fd, _ := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
    defer syscall.Close(fd)

    // Join the namespace
    flag := namespaceTypeToFlag[nsType]
    syscall.Syscall(unix.SYS_SETNS, uintptr(fd), flag, 0)

    return nil
}
```

---

## Rootfs Setup

### Main Setup Function

**File:** `linux/rootfs.go`

```go
func SetupRootfs(ctx context.Context, s *spec.Spec, bundlePath string) error {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    logger := logging.FromContext(ctx)

    // Get absolute rootfs path
    rootfs := s.Root.Path
    if !filepath.IsAbs(rootfs) {
        rootfs = filepath.Join(bundlePath, rootfs)
    }

    logger.Debug("setting up rootfs", "path", rootfs)

    // Make mount tree private (prevent propagation to host)
    syscall.Mount("", "/", "", MS_REC|MS_PRIVATE, "")

    // Bind mount rootfs to itself (required for pivot_root)
    syscall.Mount(rootfs, rootfs, "", MS_BIND|MS_REC, "")

    // Setup all mounts from config
    setupMounts(s.Mounts, rootfs)

    // PIVOT ROOT - Change root filesystem
    pivotRoot(rootfs)

    // Mask sensitive paths
    for _, path := range s.Linux.MaskedPaths {
        maskPath(path)
    }

    return nil
}
```

### Secure Path Joining

**File:** `linux/rootfs.go`

```go
// SecureJoin safely joins paths preventing directory traversal attacks
func SecureJoin(rootfs, unsafePath string) (string, error) {
    // Clean the path
    cleanPath := filepath.Clean(unsafePath)

    // Join with rootfs
    fullPath := filepath.Join(rootfs, cleanPath)

    // Resolve symlinks
    resolved, err := filepath.EvalSymlinks(fullPath)
    if err != nil && !os.IsNotExist(err) {
        return "", err
    }

    // Verify the resolved path is still under rootfs
    if !strings.HasPrefix(resolved, rootfs) {
        return "", &errors.ContainerError{
            Op:   "SecureJoin",
            Kind: errors.ErrPathTraversal,
            Err:  fmt.Errorf("path %q escapes rootfs", unsafePath),
        }
    }

    return fullPath, nil
}
```

### Mount Setup

**File:** `linux/rootfs.go`

```go
func setupMounts(mounts []spec.Mount, rootfs string) error {
    for _, m := range mounts {
        // SECURITY: Validate path doesn't escape rootfs
        dest, err := SecureJoin(rootfs, m.Destination)
        if err != nil {
            return err
        }

        // Parse mount options
        flags, data := parseMountOptions(m.Options)

        if m.Type == "bind" {
            // Bind mount: mount source to dest
            syscall.Mount(m.Source, dest, "", flags|MS_BIND, data)
        } else {
            // Regular mount (proc, sysfs, tmpfs, etc.)
            os.MkdirAll(dest, 0755)
            syscall.Mount(m.Source, dest, m.Type, flags, data)
        }
    }
    return nil
}
```

### Pivot Root

**File:** `linux/rootfs.go`

```go
func pivotRoot(rootfs string) error {
    // Create directory for old root
    oldRoot := filepath.Join(rootfs, ".old_root")
    os.MkdirAll(oldRoot, 0700)

    // PIVOT_ROOT: Swap root filesystem
    // - rootfs becomes new /
    // - old / is moved to .old_root
    syscall.PivotRoot(rootfs, oldRoot)

    // Change to new root
    os.Chdir("/")

    // Unmount old root (it's at /.old_root now)
    syscall.Unmount("/.old_root", syscall.MNT_DETACH)

    // Remove the directory
    os.RemoveAll("/.old_root")

    return nil
}
```

**Visual:**

```
BEFORE pivot_root:
/                           ← Host root
├── bin/
├── home/
├── tmp/
│   └── bundle/
│       └── rootfs/         ← Container root
│           ├── bin/
│           └── etc/
└── ...

AFTER pivot_root:
/                           ← Container root (was rootfs/)
├── bin/
├── etc/
└── .old_root/              ← Host root (unmounted)
    └── (empty after unmount)
```

---

## Cgroup Setup

### Create Cgroup

**File:** `linux/cgroup.go`

```go
func NewCgroup(ctx context.Context, path string) (*Cgroup, error) {
    logger := logging.FromContext(ctx)
    logger.Debug("creating cgroup", "path", path)

    // Create cgroup directory
    // e.g., /sys/fs/cgroup/runc-go/mycontainer/
    if err := os.MkdirAll(path, 0755); err != nil {
        return nil, err
    }

    // Enable controllers (cpu, memory, pids)
    enableControllers(path)

    return &Cgroup{path: path}, nil
}
```

### Apply Resource Limits

**File:** `linux/cgroup.go`

```go
func (c *Cgroup) ApplyResources(ctx context.Context, resources *spec.LinuxResources) error {
    logger := logging.FromContext(ctx)

    // Memory limit
    if resources.Memory != nil && resources.Memory.Limit != nil {
        limit := *resources.Memory.Limit
        logger.Debug("setting memory limit", "bytes", limit)
        // Write to memory.max
        // e.g., echo "536870912" > /sys/fs/cgroup/.../memory.max
        os.WriteFile(
            filepath.Join(c.path, "memory.max"),
            []byte(strconv.FormatInt(limit, 10)),
            0644,
        )
    }

    // CPU limit
    if resources.CPU != nil {
        if resources.CPU.Quota != nil && resources.CPU.Period != nil {
            // Write to cpu.max
            // Format: "$quota $period"
            // e.g., "50000 100000" = 50% CPU
            content := fmt.Sprintf("%d %d", *resources.CPU.Quota, *resources.CPU.Period)
            os.WriteFile(filepath.Join(c.path, "cpu.max"), []byte(content), 0644)
        }
    }

    // PID limit
    if resources.Pids != nil && resources.Pids.Limit > 0 {
        logger.Debug("setting pids limit", "max", resources.Pids.Limit)
        // Write to pids.max
        os.WriteFile(
            filepath.Join(c.path, "pids.max"),
            []byte(strconv.FormatInt(resources.Pids.Limit, 10)),
            0644,
        )
    }

    return nil
}
```

### Add Process to Cgroup

**File:** `linux/cgroup.go`

```go
func (c *Cgroup) AddProcess(pid int) error {
    // Write PID to cgroup.procs
    // This moves the process into the cgroup
    procsPath := filepath.Join(c.path, "cgroup.procs")
    return os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644)
}
```

---

## Security Setup

### Capabilities

**File:** `linux/capabilities.go`

```go
func ApplyCapabilities(caps *spec.LinuxCapabilities) error {
    // Clear ambient capabilities
    syscall.Syscall(syscall.SYS_PRCTL, PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR, 0)

    // Drop capabilities not in bounding set
    if err := applyBounding(caps.Bounding); err != nil {
        return &errors.ContainerError{
            Op:   "ApplyCapabilities",
            Kind: errors.ErrCapability,
            Err:  err,
        }
    }

    // Set effective, permitted, inheritable
    header := capHeader{Version: LINUX_CAPABILITY_VERSION_3}
    data := [2]capData{}

    setCapBits(&data, caps.Effective, ...)
    setCapBits(&data, caps.Permitted, ...)
    setCapBits(&data, caps.Inheritable, ...)

    syscall.Syscall(syscall.SYS_CAPSET,
        uintptr(unsafe.Pointer(&header)),
        uintptr(unsafe.Pointer(&data[0])),
        0)

    return nil
}
```

### Seccomp

**File:** `linux/seccomp.go`

```go
func SetupSeccomp(config *spec.LinuxSeccomp) error {
    // Set no new privileges
    syscall.Syscall(syscall.SYS_PRCTL, PR_SET_NO_NEW_PRIVS, 1, 0)

    // Build BPF filter
    filter, err := buildSeccompFilter(config)
    if err != nil {
        return &errors.ContainerError{
            Op:   "SetupSeccomp",
            Kind: errors.ErrSeccomp,
            Err:  err,
        }
    }

    // Install filter
    prog := sockFprog{
        Len:    uint16(len(filter)),
        Filter: &filter[0],
    }

    syscall.Syscall(syscall.SYS_PRCTL,
        PR_SET_SECCOMP,
        SECCOMP_MODE_FILTER,
        uintptr(unsafe.Pointer(&prog)))

    return nil
}
```

---

## Context Propagation

Context flows through the entire system for cancellation and timeout support.

### Signal Handling Setup

**File:** `cmd/root.go`

```go
func GetContext() context.Context {
    // Create context that cancels on SIGINT/SIGTERM
    ctx, cancel := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)

    // Cancel is called automatically when signal received
    _ = cancel  // Stored for cleanup if needed

    return ctx
}
```

### Context Check Pattern

All long-running operations check for context cancellation:

```go
func SomeOperation(ctx context.Context, ...) error {
    // Check at entry
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // ... do work ...

    // Check periodically in loops
    for i := 0; i < len(items); i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        // process item
    }

    return nil
}
```

### Context Flow Diagram

```
GetContext() [cmd/root.go]
    │
    ▼
runCreate(cmd, args) [cmd/create.go]
    │
    ├─► container.New(ctx, ...) [container/container.go]
    │
    ├─► Create(ctx, c, opts) [container/create.go]
    │       │
    │       ├─► linux.NewCgroup(ctx, ...) [linux/cgroup.go]
    │       │
    │       └─► linux.SetupRootfs(ctx, ...) [linux/rootfs.go]
    │
    └─► c.Start(ctx) [container/start.go]
            │
            └─► c.Wait(ctx) [container/start.go]
```

---

## Error Handling Flow

### Custom Error Types

**File:** `errors/errors.go`

```go
type ErrorKind int

const (
    ErrNotFound ErrorKind = iota
    ErrAlreadyExists
    ErrInvalidState
    ErrInvalidConfig
    ErrPermission
    ErrResource
    ErrNamespace
    ErrCgroup
    ErrSeccomp
    ErrCapability
    ErrDevice
    ErrRootfs
    ErrPathTraversal
)

type ContainerError struct {
    Op        string     // Operation that failed
    Container string     // Container ID (if applicable)
    Kind      ErrorKind  // Error category
    Err       error      // Underlying error
}

func (e *ContainerError) Error() string {
    if e.Container != "" {
        return fmt.Sprintf("%s: container %s: %v", e.Op, e.Container, e.Err)
    }
    return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *ContainerError) Unwrap() error {
    return e.Err
}
```

### Error Checking Pattern

```go
err := container.Load(ctx, id, stateRoot)
if err != nil {
    // Check for specific error kind
    var containerErr *errors.ContainerError
    if errors.As(err, &containerErr) {
        switch containerErr.Kind {
        case errors.ErrNotFound:
            fmt.Fprintf(os.Stderr, "container %s not found\n", id)
            return 1
        case errors.ErrInvalidState:
            fmt.Fprintf(os.Stderr, "container in wrong state: %v\n", err)
            return 1
        }
    }
    // Generic error
    return err
}
```

---

## Logging Integration

### Logger Setup

**File:** `cmd/root.go`

```go
func setupLogging() error {
    var handler slog.Handler
    var output io.Writer = os.Stderr

    // Use log file if specified
    if globalLog != "" {
        f, err := os.OpenFile(globalLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
        if err != nil {
            return err
        }
        output = f
    }

    // Set log level
    level := slog.LevelInfo
    if globalDebug {
        level = slog.LevelDebug
    }

    opts := &slog.HandlerOptions{Level: level}

    // Create handler based on format
    switch globalLogFormat {
    case "json":
        handler = slog.NewJSONHandler(output, opts)
    default:
        handler = slog.NewTextHandler(output, opts)
    }

    // Set default logger
    slog.SetDefault(slog.New(handler))

    return nil
}
```

### Log Output Examples

**Text format:**
```
time=2024-01-15T10:30:00.000Z level=INFO msg="creating container" id=mycontainer bundle=/bundle
time=2024-01-15T10:30:00.050Z level=DEBUG msg="setting up rootfs" path=/bundle/rootfs
time=2024-01-15T10:30:00.100Z level=INFO msg="container process started" pid=12345
```

**JSON format:**
```json
{"time":"2024-01-15T10:30:00.000Z","level":"INFO","msg":"creating container","id":"mycontainer","bundle":"/bundle"}
{"time":"2024-01-15T10:30:00.050Z","level":"DEBUG","msg":"setting up rootfs","path":"/bundle/rootfs"}
{"time":"2024-01-15T10:30:00.100Z","level":"INFO","msg":"container process started","pid":12345}
```

---

## Summary: Complete `run` Flow

```
1. main()                           Call cmd.Execute()
   │
2. Cobra parses                     Parse flags, find "run" command
   │
3. runRun()                         Get context with signal handling
   │
4. container.New(ctx, ...)          Create Container struct, load spec
   │
5. c.Run(ctx, opts)                 Orchestrate create/start/wait
   │
6. Create(ctx, c, opts)
   ├── CreateExecFifo()             Create sync FIFO
   ├── NewCgroup(ctx, ...)          Create cgroup
   ├── NewConsole()                 Create PTY (if terminal)
   ├── BuildSysProcAttr()           Build namespace flags
   ├── cmd.Start()                  FORK! Child in new namespaces
   │   │
   │   └── [CHILD] InitContainer()
   │       ├── Open exec FIFO       (Get handle before pivot_root)
   │       ├── SetHostname()        Set container hostname
   │       ├── SetupRootfs(ctx,.)
   │       │   ├── setupMounts()    Mount proc, dev, sys, etc.
   │       │   └── pivotRoot()      Change root filesystem
   │       ├── SetupDefaultDevices() Create /dev/null, etc.
   │       ├── Open console         Setup PTY slave
   │       ├── READ FIFO            ← BLOCKS HERE (waiting for start)
   │       │
   │       │   [After start writes to FIFO]
   │       │
   │       ├── ApplyCapabilities()  Drop privileges
   │       ├── SetupSeccomp()       Install syscall filter
   │       ├── Setuid/Setgid()      Change user
   │       └── syscall.Exec()       REPLACE WITH USER COMMAND
   │
   ├── SendConsoleToSocket()        Send PTY to Docker (if needed)
   ├── AddProcess()                 Add child to cgroup
   └── SaveState()                  Write state.json
   │
7. c.Start(ctx)
   └── Write to FIFO                Unblock child
   │
8. c.Wait(ctx)
   └── cmd.Wait()                   Wait for child to exit
   │
9. Return exit code
```

---

## Quick Reference: Key Files

| Component | File | Key Functions |
|-----------|------|---------------|
| Entry point | `main.go` | `main()` |
| CLI commands | `cmd/*.go` | `runCreate()`, `runStart()`, `runRun()`, etc. |
| Container lifecycle | `container/*.go` | `New()`, `Create()`, `Start()`, `Run()`, `Wait()` |
| Namespaces | `linux/namespace.go` | `NamespaceFlags()`, `BuildSysProcAttr()` |
| Rootfs | `linux/rootfs.go` | `SetupRootfs()`, `SecureJoin()`, `pivotRoot()` |
| Cgroups | `linux/cgroup.go` | `NewCgroup()`, `ApplyResources()` |
| Capabilities | `linux/capabilities.go` | `ApplyCapabilities()` |
| Seccomp | `linux/seccomp.go` | `SetupSeccomp()`, `buildSeccompFilter()` |
| Devices | `linux/devices.go` | `SetupDefaultDevices()` |
| Errors | `errors/errors.go` | `ContainerError`, error kinds |
| Logging | `logging/logger.go` | `FromContext()`, `NewLogger()` |
