# Execution Flow Guide

A detailed walkthrough of how runc-go executes, with code navigation.

---

## Table of Contents

1. [Overview](#overview)
2. [Command Dispatch](#command-dispatch)
3. [The `run` Command](#the-run-command)
4. [The `create` Command](#the-create-command)
5. [The `start` Command](#the-start-command)
6. [The `exec` Command](#the-exec-command)
7. [Container Init Process](#container-init-process)
8. [Namespace Setup](#namespace-setup)
9. [Rootfs Setup](#rootfs-setup)
10. [Cgroup Setup](#cgroup-setup)
11. [Security Setup](#security-setup)

---

## Overview

When you run a container, here's the high-level flow:

```
User runs: sudo runc-go run mycontainer /bundle

main.go                     Parse CLI, dispatch to cmdRun()
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

## Command Dispatch

### Entry Point: `main.go:60`

```go
func main() {
    // Parse global flags (--root, --log, etc.)
    args := os.Args[1:]
    for len(args) > 0 {
        switch {
        case args[0] == "--root" && len(args) > 1:
            globalRoot = args[1]
            // ...
        }
    }
```

### Command Router: `main.go:114`

```go
    switch cmd {
    case "create":
        err = cmdCreate()      // → main.go:168
    case "start":
        err = cmdStart()       // → main.go:227
    case "run":
        err = cmdRun()         // → main.go:253
    case "exec":
        err = cmdExec()        // → main.go:295
    case "state":
        err = cmdState()       // → main.go:370
    case "kill":
        err = cmdKill()        // → main.go:391
    case "delete":
        err = cmdDelete()      // → main.go:420
    case "list", "ps":
        err = cmdList()        // → main.go:448
    case "spec":
        err = cmdSpec()        // → main.go:480
    case "init":
        err = cmdInit()        // → main.go:500 (internal)
    case "exec-init":
        err = cmdExecInit()    // → main.go:540 (internal)
    }
```

---

## The `run` Command

**Command:** `sudo runc-go run mycontainer /bundle`

### Step 1: Parse Arguments

**File:** `main.go:253`

```go
func cmdRun() error {
    args := parseArgs(os.Args[2:])

    containerID := args.pos(0)  // "mycontainer"
    bundle := args.pos(1)       // "/bundle"

    // Get options
    consoleSocket := args.flag("console-socket")
    pidFile := args.flag("pid-file")
    detach := args.has("detach") || args.has("d")
```

### Step 2: Create Container Object

**File:** `main.go:275` → `container/container.go:107`

```go
    // main.go
    c, err := container.New(containerID, bundle, root)

    // container/container.go:107
    func New(id, bundle, stateRoot string) (*Container, error) {
        // Validate container ID (security check)
        if err := ValidateContainerID(id); err != nil {
            return nil, err
        }

        // Load OCI spec from bundle/config.json
        specPath := filepath.Join(bundle, "config.json")
        s, err := spec.LoadSpec(specPath)

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

**File:** `main.go:282` → `container/create.go:88`

```go
    // main.go
    exitCode, err := container.Run(c, opts)

    // container/create.go:88
    func Run(c *Container, opts *CreateOptions) (int, error) {
        // Create the container (fork child, setup namespaces)
        if err := Create(c, opts); err != nil {
            return -1, err
        }

        // Start it (signal child to exec)
        if err := Start(c); err != nil {
            return -1, err
        }

        // Wait for exit
        return Wait(c)
    }
```

---

## The `create` Command

**Command:** `sudo runc-go create mycontainer /bundle`

This is the heart of container creation.

### Step 1: Setup Synchronization

**File:** `container/create.go:105`

```go
func Create(c *Container, opts *CreateOptions) error {
    s := c.Spec

    // Create FIFO for create/start synchronization
    // Child will block reading this until start is called
    if err := c.CreateExecFifo(); err != nil {
        return err
    }
    // Creates /run/runc-go/<id>/exec.fifo
```

### Step 2: Setup Cgroup

**File:** `container/create.go:115` → `linux/cgroup.go`

```go
    // Create cgroup for resource limits
    cgroupPath := filepath.Join("/sys/fs/cgroup", "runc-go", c.ID)
    cg, err := linux.NewCgroup(cgroupPath)

    // Apply resource limits from config.json
    if s.Linux != nil && s.Linux.Resources != nil {
        cg.ApplyResources(s.Linux.Resources)
    }
```

### Step 3: Setup Console (PTY)

**File:** `container/create.go:130`

```go
    // If terminal requested, create PTY
    var console *utils.Console
    if s.Process.Terminal {
        console, err = utils.NewConsole()
        // console.Master() = PTY master (parent keeps this)
        // console.SlavePath() = /dev/pts/N (child uses this)
    }
```

### Step 4: Build Process Attributes

**File:** `container/create.go:145` → `linux/namespace.go:103`

```go
    // Get self executable for re-exec
    self, _ := os.Executable()

    // Build command: runc-go init
    cmd := exec.Command(self, "init")

    // Build namespace flags from config
    // linux/namespace.go:103
    func BuildSysProcAttr(s *spec.Spec) (*syscall.SysProcAttr, error) {
        flags := NamespaceFlags(s.Linux.Namespaces)
        // flags = CLONE_NEWPID | CLONE_NEWNS | CLONE_NEWUTS | ...

        return &syscall.SysProcAttr{
            Cloneflags: flags,  // Create new namespaces
            Setsid:     true,   // New session
        }, nil
    }

    cmd.SysProcAttr = attr
```

### Step 5: Pass Config to Child

**File:** `container/create.go:165`

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

### Step 6: Fork Child Process

**File:** `container/create.go:180`

```go
    // START THE CHILD PROCESS
    // This forks with CLONE_NEW* flags
    // Child is now in NEW namespaces!
    if err := cmd.Start(); err != nil {
        return err
    }

    // Save PID
    c.InitProcess = cmd.Process.Pid
    c.State.Pid = c.InitProcess
```

### Step 7: Parent Waits

**File:** `container/create.go:195`

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

    // Parent is done - child is blocked on exec FIFO
    return nil
```

---

## The `start` Command

**Command:** `sudo runc-go start mycontainer`

### Unblock the Child

**File:** `container/start.go:20`

```go
func Start(c *Container) error {
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

    return nil
}
```

---

## The `exec` Command

**Command:** `sudo runc-go exec -t mycontainer /bin/sh`

### Step 1: Load Container

**File:** `container/exec.go:74`

```go
func Exec(containerID, stateRoot string, args []string, opts *ExecOptions) error {
    // Load existing container
    c, err := Load(containerID, stateRoot)

    // Verify it's running
    if c.State.Status != spec.StatusRunning {
        return fmt.Errorf("container is not running")
    }
```

### Step 2: Build exec-init Command

**File:** `container/exec.go:106`

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

### Step 3: Handle TTY

**File:** `container/exec.go:169`

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

### Step 4: exec-init Joins Namespaces

**File:** `container/exec.go:400`

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

    // Add command with proper quoting
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

### Entry: `main.go:500`

```go
func cmdInit() error {
    // This runs INSIDE the new namespaces
    // but BEFORE pivot_root

    return container.Init()
}
```

### Init Flow: `container/create.go:220`

```go
func Init() error {
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

**File:** `container/create.go:235` → `linux/namespace.go:187`

```go
    // Set hostname in UTS namespace
    if s.Hostname != "" {
        linux.SetHostname(s.Hostname)
        // → syscall.Sethostname([]byte(hostname))
    }
```

### Step 3: Setup Rootfs

**File:** `container/create.go:240` → `linux/rootfs.go:63`

```go
    // Setup root filesystem
    linux.SetupRootfs(s, bundle)
    // This does:
    // 1. Bind mount rootfs to itself
    // 2. Setup all mounts from config
    // 3. pivot_root to new root
    // 4. Mount /proc, /dev, /sys
    // 5. Mask sensitive paths
```

### Step 4: Setup Devices

**File:** `container/create.go:250` → `linux/devices.go`

```go
    // Create device nodes
    linux.SetupDefaultDevices()
    // Creates: /dev/null, /dev/zero, /dev/random, etc.

    // Create symlinks
    linux.SetupDevSymlinks()
    // Creates: /dev/fd → /proc/self/fd, etc.
```

### Step 5: Setup Console

**File:** `container/create.go:260`

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

**File:** `container/create.go:280`

```go
    // READ FROM FIFO - THIS BLOCKS!
    // We wait here until "runc-go start" writes to the FIFO
    buf := make([]byte, 1)
    fifo.Read(buf)
    fifo.Close()

    // Start command received! Continue...
```

### Step 7: Apply Security

**File:** `container/create.go:290`

```go
    // Drop capabilities
    if s.Process.Capabilities != nil {
        linux.ApplyCapabilities(s.Process.Capabilities)
    }

    // Install seccomp filter
    if s.Linux != nil && s.Linux.Seccomp != nil {
        linux.SetupSeccomp(s.Linux.Seccomp)
    }

    // Set user/group
    if s.Process.User.UID != 0 || s.Process.User.GID != 0 {
        syscall.Setgid(int(s.Process.User.GID))
        syscall.Setuid(int(s.Process.User.UID))
    }
```

### Step 8: Exec User Command

**File:** `container/create.go:310`

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

**File:** `linux/namespace.go:36`

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

**File:** `linux/namespace.go:72`

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

**File:** `linux/rootfs.go:63`

```go
func SetupRootfs(s *spec.Spec, bundlePath string) error {
    // Get absolute rootfs path
    rootfs := s.Root.Path
    if !filepath.IsAbs(rootfs) {
        rootfs = filepath.Join(bundlePath, rootfs)
    }

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

### Mount Setup

**File:** `linux/rootfs.go:208`

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

**File:** `linux/rootfs.go:136`

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

**File:** `linux/cgroup.go:30`

```go
func NewCgroup(path string) (*Cgroup, error) {
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

**File:** `linux/cgroup.go:80`

```go
func (c *Cgroup) ApplyResources(resources *spec.LinuxResources) error {
    // Memory limit
    if resources.Memory != nil && resources.Memory.Limit != nil {
        // Write to memory.max
        // e.g., echo "536870912" > /sys/fs/cgroup/.../memory.max
        limit := *resources.Memory.Limit
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

**File:** `linux/cgroup.go:45`

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

**File:** `linux/capabilities.go:163`

```go
func ApplyCapabilities(caps *spec.LinuxCapabilities) error {
    // Clear ambient capabilities
    syscall.Syscall(syscall.SYS_PRCTL, PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR, 0)

    // Drop capabilities not in bounding set
    applyBounding(caps.Bounding)

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

**File:** `linux/seccomp.go:184`

```go
func SetupSeccomp(config *spec.LinuxSeccomp) error {
    // Set no new privileges
    syscall.Syscall(syscall.SYS_PRCTL, PR_SET_NO_NEW_PRIVS, 1, 0)

    // Build BPF filter
    filter := buildSeccompFilter(config)

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

## Summary: Complete `run` Flow

```
1. main()                           Parse args, dispatch to cmdRun()
   │
2. cmdRun()                         Parse container ID, bundle path
   │
3. container.New()                  Create Container struct, load spec
   │
4. container.Run()                  Orchestrate create/start/wait
   │
5. container.Create()
   ├── CreateExecFifo()             Create sync FIFO
   ├── NewCgroup()                  Create cgroup
   ├── NewConsole()                 Create PTY (if terminal)
   ├── BuildSysProcAttr()           Build namespace flags
   ├── cmd.Start()                  FORK! Child in new namespaces
   │   │
   │   └── [CHILD] cmdInit()
   │       ├── Open exec FIFO       (Get handle before pivot_root)
   │       ├── SetHostname()        Set container hostname
   │       ├── SetupRootfs()
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
6. container.Start()
   └── Write to FIFO                Unblock child
   │
7. container.Wait()
   └── cmd.Wait()                   Wait for child to exit
   │
8. Return exit code
```
