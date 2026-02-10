# runc-go: OCI Container Runtime Documentation

A production-quality, OCI Runtime Specification-compliant container runtime written in Go.

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Installation](#installation)
4. [Building from Source](#building-from-source)
5. [Usage Guide](#usage-guide)
6. [Container Lifecycle](#container-lifecycle)
7. [Configuration](#configuration)
8. [OCI Runtime Specification](#oci-runtime-specification)
9. [Testing](#testing)
10. [Security](#security)
11. [Troubleshooting](#troubleshooting)
12. [API Reference](#api-reference)
13. [Contributing](#contributing)

---

## Overview

**runc-go** is a lightweight, OCI-compliant container runtime that implements the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec). It provides the low-level container execution capabilities used by container platforms like Docker and containerd.

### Key Features

- **Full OCI Compliance**: Implements OCI Runtime Spec v1.0.2
- **Linux Namespaces**: PID, mount, network, UTS, IPC, user, and cgroup namespaces
- **Cgroups v2**: Resource limits for memory, CPU, and PIDs
- **Seccomp**: System call filtering via BPF
- **Capabilities**: Linux capability management
- **Lifecycle Hooks**: All OCI-defined hook points
- **PTY Support**: Interactive terminal sessions
- **Thread-Safe**: Concurrent access protection

### Project Statistics

| Metric | Value |
|--------|-------|
| Lines of Code | ~12,800 |
| Go Files | 49 |
| Packages | 9 |
| Test Coverage | Comprehensive |
| Go Version | 1.24+ |

---

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     User / containerd                    │
└───────────────────────────┬─────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────┐
│                    CLI Layer (cmd)                       │
│     create | start | run | exec | kill | delete | ...   │
└───────────────────────────┬─────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────┐
│               Container Lifecycle (container)            │
│         New() → Create() → Start() → Wait() → Delete()  │
└───────────────────────────┬─────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│  spec (OCI)   │   │ linux (Isol.) │   │   Utilities   │
│  - Spec types │   │ - Namespace   │   │ - Console/PTY │
│  - State mgmt │   │ - Cgroup      │   │ - Sync pipes  │
│  - Hooks def  │   │ - Seccomp     │   │ - Logging     │
└───────────────┘   │ - Capability  │   │ - Errors      │
                    │ - Rootfs      │   └───────────────┘
                    └───────────────┘
```

### Package Structure

```
runc-go/
├── main.go                 # Application entry point
├── cmd/                    # CLI commands (Cobra framework)
│   ├── root.go            # Root command, global flags
│   ├── create.go          # Create container
│   ├── start.go           # Start created container
│   ├── run.go             # Create + start combined
│   ├── exec.go            # Execute in container
│   ├── kill.go            # Send signals
│   ├── delete.go          # Delete container
│   ├── state.go           # Query state
│   ├── list.go            # List containers
│   ├── spec.go            # Generate spec template
│   ├── version.go         # Version info
│   └── init.go            # Internal init commands
├── container/              # Container lifecycle management
│   ├── container.go       # Container struct, Load/New
│   ├── create.go          # Create operation
│   ├── start.go           # Start/Wait operations
│   ├── exec.go            # Exec operation
│   ├── delete.go          # Delete operation
│   ├── kill.go            # Signal operations
│   └── *_test.go          # Tests
├── spec/                   # OCI specification types
│   ├── spec.go            # OCI config types
│   ├── state.go           # Container state types
│   └── *_test.go          # Tests
├── linux/                  # Linux isolation primitives
│   ├── namespace.go       # Namespace management
│   ├── cgroup.go          # Cgroup v2 support
│   ├── seccomp.go         # Seccomp BPF filters
│   ├── capabilities.go    # Linux capabilities
│   ├── rootfs.go          # Root filesystem setup
│   ├── devices.go         # Device management
│   └── *_test.go          # Tests
├── hooks/                  # OCI lifecycle hooks
│   ├── hooks.go           # Hook execution
│   └── hooks_test.go      # Tests
├── logging/                # Structured logging
│   ├── logger.go          # slog-based logging
│   └── logger_test.go     # Tests
├── errors/                 # Error handling
│   ├── errors.go          # Error types
│   ├── sentinel.go        # Sentinel errors
│   └── errors_test.go     # Tests
└── utils/                  # Utilities
    ├── console.go         # PTY/console handling
    └── sync.go            # Synchronization primitives
```

### Component Interactions

#### Container Lifecycle Flow

```
┌──────────────┐
│   CREATING   │  ← container.New() called
└──────┬───────┘
       │ container.Create()
       │   1. Create cgroup
       │   2. Apply resource limits
       │   3. Create exec.fifo
       │   4. Fork init process
       │   5. Save state
       ▼
┌──────────────┐
│   CREATED    │  ← Init process blocked on FIFO
└──────┬───────┘
       │ container.Start()
       │   1. Write to FIFO
       │   2. Init continues
       │   3. Update state
       ▼
┌──────────────┐
│   RUNNING    │  ← User process executing
└──────┬───────┘
       │ Process exits or container.Kill()
       ▼
┌──────────────┐
│   STOPPED    │  ← Ready for cleanup
└──────┬───────┘
       │ container.Delete()
       ▼
    (destroyed)
```

#### Init Process Flow

```
Parent Process                    Child Process (init)
      │                                  │
      │ fork()                           │
      ├─────────────────────────────────►│
      │                                  │ Setup namespaces
      │                                  │ Setup rootfs (pivot_root)
      │                                  │ Setup devices
      │                                  │ Open exec.fifo (blocks)
      │                                  │       │
      │ Start() - write to FIFO          │       │
      ├─────────────────────────────────►│◄──────┘
      │                                  │ Read from FIFO (unblocks)
      │                                  │ Apply capabilities
      │                                  │ Apply seccomp
      │                                  │ Set user/group
      │                                  │ exec() user command
      │                                  ▼
      │                            [User Process]
```

---

## Installation

### Prerequisites

- Linux kernel 4.18+ (for cgroup v2)
- Go 1.24 or later
- Root privileges (for container operations)

### From Binary

```bash
# Download latest release
curl -LO https://github.com/your-org/runc-go/releases/latest/download/runc-go

# Make executable
chmod +x runc-go

# Move to PATH
sudo mv runc-go /usr/local/bin/
```

### From Source

See [Building from Source](#building-from-source) below.

---

## Building from Source

### Clone Repository

```bash
git clone https://github.com/your-org/runc-go.git
cd runc-go
```

### Build

```bash
# Simple build
go build -o runc-go .

# Build with version info
go build -ldflags "-X runc-go/cmd.Version=1.0.0 -X runc-go/cmd.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o runc-go .

# Static build (for containers)
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o runc-go .
```

### Install

```bash
sudo install -m 755 runc-go /usr/local/bin/
```

### Dependencies

The project uses Go modules. Dependencies are automatically downloaded:

| Dependency | Purpose |
|------------|---------|
| golang.org/x/sys | Low-level syscall access |
| golang.org/x/term | Terminal handling |
| github.com/spf13/cobra | CLI framework |

---

## Usage Guide

### Quick Start

```bash
# Create a bundle directory
mkdir -p mycontainer/rootfs

# Create a minimal root filesystem
docker export $(docker create alpine) | tar -C mycontainer/rootfs -xf -

# Generate OCI spec
cd mycontainer
runc-go spec

# Create and start container
sudo runc-go run mycontainer
```

### Command Reference

#### create - Create a Container

```bash
# Basic create
sudo runc-go create <container-id> -b <bundle-path>

# With options
sudo runc-go create mycontainer \
    --bundle /path/to/bundle \
    --pid-file /run/mycontainer.pid \
    --console-socket /run/console.sock

Options:
  -b, --bundle string          Path to OCI bundle (default: current dir)
  --pid-file string            Write container PID to file
  --console-socket string      Unix socket for console PTY
  --no-pivot                   Use chroot instead of pivot_root
  --no-new-keyring            Don't create new session keyring
```

#### start - Start a Created Container

```bash
sudo runc-go start <container-id>
```

#### run - Create and Start in One Step

```bash
# Interactive shell
sudo runc-go run -t mycontainer

# Detached
sudo runc-go run -d mycontainer

Options:
  -b, --bundle string          Path to OCI bundle
  -d, --detach                 Run in background
  -t, --tty                    Allocate pseudo-TTY
  --pid-file string            Write PID to file
  --console-socket string      Unix socket for console
```

#### exec - Execute Command in Container

```bash
# Run command
sudo runc-go exec mycontainer /bin/ls -la

# Interactive shell
sudo runc-go exec -t mycontainer /bin/sh

# With process spec file
sudo runc-go exec -p process.json mycontainer

Options:
  -t, --tty                    Allocate pseudo-TTY
  -d, --detach                 Run in background
  -e, --env strings            Set environment variables
  -u, --user string            User (uid:gid)
  --cwd string                 Working directory
  -p, --process string         Process spec file
  --pid-file string            Write PID to file
  --console-socket string      Unix socket for console
```

#### kill - Send Signal to Container

```bash
# Send SIGTERM (default)
sudo runc-go kill mycontainer

# Send specific signal
sudo runc-go kill mycontainer SIGKILL
sudo runc-go kill mycontainer 9

# Kill all processes in container
sudo runc-go kill --all mycontainer

Options:
  -a, --all                    Signal all processes
```

#### delete - Delete Container

```bash
# Delete stopped container
sudo runc-go delete mycontainer

# Force delete running container
sudo runc-go delete --force mycontainer

Options:
  -f, --force                  Force delete even if running
```

#### state - Query Container State

```bash
sudo runc-go state mycontainer

# Output:
{
  "ociVersion": "1.0.2",
  "id": "mycontainer",
  "status": "running",
  "pid": 12345,
  "bundle": "/path/to/bundle"
}
```

#### list - List All Containers

```bash
sudo runc-go list

# Output:
ID              PID     STATUS      BUNDLE                  CREATED
mycontainer     12345   running     /path/to/bundle         2024-01-15T10:30:00Z
test-container  0       stopped     /path/to/test           2024-01-15T09:00:00Z
```

#### spec - Generate OCI Spec Template

```bash
# Generate default spec
runc-go spec

# Generate rootless spec
runc-go spec --rootless

# Custom output
runc-go spec -o custom-config.json
```

#### version - Show Version Information

```bash
runc-go version

# Output:
runc-go version 0.1.0
spec: 1.0.2
go: go1.24.0
```

### Global Flags

```bash
--root string        State root directory (default: /run/runc-go)
--log string         Log file path
--log-format string  Log format: text or json (default: text)
--debug              Enable debug logging
--systemd-cgroup     Use systemd cgroup driver (compatibility)
```

---

## Container Lifecycle

### States

| State | Description |
|-------|-------------|
| `creating` | Container is being created |
| `created` | Container created, init blocked on FIFO |
| `running` | Container process is running |
| `stopped` | Container process has exited |

### State Transitions

```
         create()
    ─────────────────►
NEW                    CREATED
                           │
                           │ start()
                           ▼
                       RUNNING
                           │
                           │ exit/kill
                           ▼
                       STOPPED
                           │
                           │ delete()
                           ▼
                      (destroyed)
```

### State Directory Structure

```
/run/runc-go/
└── <container-id>/
    ├── state.json       # Container state
    └── exec.fifo        # Create/start synchronization (temporary)
```

### state.json Format

```json
{
  "ociVersion": "1.0.2",
  "id": "mycontainer",
  "status": "running",
  "pid": 12345,
  "bundle": "/path/to/bundle",
  "created": "2024-01-15T10:30:00.000000000Z",
  "rootfs": "/path/to/bundle/rootfs",
  "annotations": {}
}
```

---

## Configuration

### config.json (OCI Runtime Spec)

The container is configured via `config.json` in the bundle directory.

#### Minimal Example

```json
{
    "ociVersion": "1.0.2",
    "process": {
        "terminal": true,
        "user": { "uid": 0, "gid": 0 },
        "args": ["/bin/sh"],
        "env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "TERM=xterm"
        ],
        "cwd": "/"
    },
    "root": {
        "path": "rootfs",
        "readonly": false
    },
    "hostname": "container"
}
```

#### Full Example

```json
{
    "ociVersion": "1.0.2",
    "process": {
        "terminal": true,
        "user": {
            "uid": 0,
            "gid": 0,
            "additionalGids": [1, 2, 3]
        },
        "args": ["/bin/sh"],
        "env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "TERM=xterm",
            "HOME=/root"
        ],
        "cwd": "/",
        "capabilities": {
            "bounding": ["CAP_NET_BIND_SERVICE", "CAP_CHOWN"],
            "effective": ["CAP_NET_BIND_SERVICE"],
            "inheritable": [],
            "permitted": ["CAP_NET_BIND_SERVICE"],
            "ambient": []
        },
        "rlimits": [
            { "type": "RLIMIT_NOFILE", "hard": 1024, "soft": 1024 }
        ],
        "noNewPrivileges": true
    },
    "root": {
        "path": "rootfs",
        "readonly": false
    },
    "hostname": "mycontainer",
    "mounts": [
        {
            "destination": "/proc",
            "type": "proc",
            "source": "proc"
        },
        {
            "destination": "/dev",
            "type": "tmpfs",
            "source": "tmpfs",
            "options": ["nosuid", "strictatime", "mode=755", "size=65536k"]
        },
        {
            "destination": "/dev/pts",
            "type": "devpts",
            "source": "devpts",
            "options": ["nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"]
        },
        {
            "destination": "/sys",
            "type": "sysfs",
            "source": "sysfs",
            "options": ["nosuid", "noexec", "nodev", "ro"]
        }
    ],
    "linux": {
        "namespaces": [
            { "type": "pid" },
            { "type": "network" },
            { "type": "ipc" },
            { "type": "uts" },
            { "type": "mount" },
            { "type": "cgroup" }
        ],
        "resources": {
            "memory": {
                "limit": 536870912
            },
            "cpu": {
                "shares": 1024,
                "quota": 100000,
                "period": 100000
            },
            "pids": {
                "limit": 100
            }
        },
        "seccomp": {
            "defaultAction": "SCMP_ACT_ERRNO",
            "architectures": ["SCMP_ARCH_X86_64"],
            "syscalls": [
                {
                    "names": ["read", "write", "exit", "exit_group", "mmap"],
                    "action": "SCMP_ACT_ALLOW"
                }
            ]
        },
        "maskedPaths": [
            "/proc/kcore",
            "/proc/keys",
            "/proc/timer_list"
        ],
        "readonlyPaths": [
            "/proc/asound",
            "/proc/bus",
            "/proc/fs"
        ]
    },
    "hooks": {
        "prestart": [
            {
                "path": "/usr/bin/fix-mounts.sh",
                "args": ["fix-mounts.sh"],
                "env": ["PATH=/usr/bin"]
            }
        ],
        "poststart": [
            {
                "path": "/usr/bin/notify-started.sh",
                "timeout": 5
            }
        ],
        "poststop": [
            {
                "path": "/usr/bin/cleanup.sh"
            }
        ]
    }
}
```

### Resource Limits

#### Memory

```json
"resources": {
    "memory": {
        "limit": 536870912,        // 512MB hard limit
        "reservation": 268435456,  // 256MB soft limit
        "swap": 536870912          // Swap limit
    }
}
```

#### CPU

```json
"resources": {
    "cpu": {
        "shares": 1024,     // Relative weight
        "quota": 50000,     // Microseconds per period
        "period": 100000    // Period in microseconds
    }
}
```

#### PIDs

```json
"resources": {
    "pids": {
        "limit": 100        // Maximum number of processes
    }
}
```

### Namespaces

```json
"namespaces": [
    { "type": "pid" },
    { "type": "network" },
    { "type": "ipc" },
    { "type": "uts" },
    { "type": "mount" },
    { "type": "user" },
    { "type": "cgroup" }
]
```

Join existing namespace:

```json
"namespaces": [
    { "type": "network", "path": "/proc/1234/ns/net" }
]
```

### Hooks

| Hook Type | When Executed |
|-----------|---------------|
| `prestart` | After namespaces created, before pivot_root (deprecated) |
| `createRuntime` | After namespaces created, before pivot_root |
| `createContainer` | After pivot_root, before user process |
| `startContainer` | After start(), before user process executes |
| `poststart` | After user process starts |
| `poststop` | After container stops |

Hook specification:

```json
{
    "path": "/path/to/hook",
    "args": ["hook", "arg1", "arg2"],
    "env": ["KEY=value"],
    "timeout": 10
}
```

---

## OCI Runtime Specification

runc-go implements the [OCI Runtime Specification v1.0.2](https://github.com/opencontainers/runtime-spec/blob/v1.0.2/spec.md).

### Compliance

| Feature | Status |
|---------|--------|
| Container lifecycle | Fully supported |
| Linux namespaces | All 7 types |
| Cgroups v2 | Memory, CPU, PIDs |
| Seccomp | BPF filter compilation |
| Capabilities | Full support |
| Hooks | All hook types |
| User namespace | Supported |

### Bundle Structure

```
bundle/
├── config.json      # OCI runtime configuration
└── rootfs/          # Container root filesystem
    ├── bin/
    ├── etc/
    ├── lib/
    ├── usr/
    └── ...
```

---

## Testing

### Run All Tests

```bash
# Standard test run
go test ./...

# With verbose output
go test -v ./...

# With race detector
go test -race ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Run Specific Package Tests

```bash
# Container tests
go test -v ./container/...

# Linux primitives tests
go test -v ./linux/...

# Spec tests
go test -v ./spec/...

# Hooks tests
go test -v ./hooks/...
```

### Run Specific Test

```bash
# Run tests matching pattern
go test -v ./... -run TestStart
go test -v ./... -run "TestSeccomp.*"

# Run single test
go test -v ./container -run TestStart_FIFOWrite
```

### Test Categories

#### Unit Tests

Located in `*_test.go` files alongside source:

```bash
go test -v ./container/... -run "TestValidate|TestNew|TestLoad"
go test -v ./linux/... -run "TestBpf|TestArch|TestAction"
go test -v ./spec/... -run "TestSpec|TestState"
```

#### Security Tests

```bash
# Shell injection tests
go test -v ./container/... -run "Injection"

# Path traversal tests
go test -v ./... -run "Traversal|Symlink"

# Seccomp tests
go test -v ./linux/... -run "Seccomp"
```

#### Concurrency Tests

```bash
# Race detector tests
go test -race ./container/... -run "Concurrent|Race"
```

### Integration Testing

```bash
# Create test bundle
mkdir -p /tmp/test-bundle/rootfs
docker export $(docker create alpine) | tar -C /tmp/test-bundle/rootfs -xf -
cd /tmp/test-bundle
runc-go spec

# Run integration test
sudo runc-go run test-container

# Cleanup
sudo runc-go delete test-container
```

### Test Coverage

Current test coverage includes:

| Package | Coverage |
|---------|----------|
| container | High (lifecycle, exec, state) |
| linux | High (seccomp, cgroups, namespace) |
| spec | High (types, serialization) |
| hooks | High (all hook types) |
| errors | Complete |
| logging | Complete |

---

## Security

### Security Features

#### Container ID Validation

```go
// Container IDs must match: ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$
// Maximum length: 1024 characters
// Path traversal explicitly blocked
```

#### Path Traversal Protection

The `SecureJoin` function prevents symlink-based escapes:

```go
// Resolves symlinks at each path component
// Ensures final path stays within base directory
path, err := linux.SecureJoin(rootfs, userPath)
```

#### Cgroup Key Validation

```go
// Cgroup keys validated against: ^[a-zA-Z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9]*)*$
// Prevents cgroup escape via malicious keys
```

#### Capability Management

- Default capabilities dropped
- Only specified capabilities retained
- Verification after capset()

#### Seccomp

- BPF filter compilation
- Architecture validation
- Syscall whitelisting/blacklisting
- Returns error if filter incomplete (>20% unknown syscalls)

### Security Best Practices

1. **Run as root only when necessary**: Container operations require root, but the container process can run as non-root

2. **Use read-only rootfs**: Set `"readonly": true` in config.json

3. **Drop all capabilities**: Only grant what's needed

4. **Enable seccomp**: Restrict system calls

5. **Use user namespaces**: Map container root to unprivileged host user

6. **Mask sensitive paths**: Use `maskedPaths` and `readonlyPaths`

### Example Secure Configuration

```json
{
    "process": {
        "noNewPrivileges": true,
        "capabilities": {
            "bounding": [],
            "effective": [],
            "inheritable": [],
            "permitted": [],
            "ambient": []
        }
    },
    "root": {
        "readonly": true
    },
    "linux": {
        "namespaces": [
            { "type": "user" },
            { "type": "pid" },
            { "type": "mount" },
            { "type": "network" },
            { "type": "ipc" },
            { "type": "uts" }
        ],
        "seccomp": {
            "defaultAction": "SCMP_ACT_ERRNO",
            "syscalls": [
                {
                    "names": ["read", "write", "exit_group", "..."],
                    "action": "SCMP_ACT_ALLOW"
                }
            ]
        },
        "maskedPaths": [
            "/proc/kcore",
            "/proc/keys",
            "/proc/timer_list",
            "/proc/sched_debug"
        ],
        "readonlyPaths": [
            "/proc/asound",
            "/proc/bus",
            "/proc/fs",
            "/proc/irq",
            "/proc/sys",
            "/proc/sysrq-trigger"
        ]
    }
}
```

---

## Troubleshooting

### Common Issues

#### Permission Denied

```
Error: permission denied
```

**Solution**: Run with sudo or as root:
```bash
sudo runc-go run mycontainer
```

#### Container Already Exists

```
Error: container "mycontainer" already exists
```

**Solution**: Delete existing container:
```bash
sudo runc-go delete mycontainer
# or force delete
sudo runc-go delete -f mycontainer
```

#### Invalid Bundle

```
Error: load spec: open /path/config.json: no such file or directory
```

**Solution**: Ensure bundle directory contains config.json:
```bash
ls -la /path/to/bundle/
# Should contain: config.json, rootfs/
```

#### Cgroup Not Mounted

```
Error: create cgroup: mkdir /sys/fs/cgroup/...: no such file or directory
```

**Solution**: Mount cgroup v2:
```bash
sudo mount -t cgroup2 none /sys/fs/cgroup
```

#### Namespace Operation Failed

```
Error: unshare: operation not permitted
```

**Solution**: Check kernel configuration and run as root. User namespaces may need:
```bash
echo 1 > /proc/sys/kernel/unprivileged_userns_clone
```

### Debug Mode

Enable debug logging:

```bash
sudo runc-go --debug run mycontainer
sudo runc-go --log /tmp/runc.log --log-format json run mycontainer
```

### Checking Container State

```bash
# View current state
sudo runc-go state mycontainer

# List all containers
sudo runc-go list

# Check state directory
ls -la /run/runc-go/mycontainer/
cat /run/runc-go/mycontainer/state.json
```

### Checking Cgroups

```bash
# Find container cgroup
cat /run/runc-go/mycontainer/state.json | jq .cgroupPath

# Check cgroup limits
cat /sys/fs/cgroup/runc-go/mycontainer/memory.max
cat /sys/fs/cgroup/runc-go/mycontainer/cpu.max
cat /sys/fs/cgroup/runc-go/mycontainer/pids.max
```

### Process Inspection

```bash
# Get container PID
sudo runc-go state mycontainer | jq .pid

# Check process namespaces
ls -la /proc/<pid>/ns/

# Check process cgroup
cat /proc/<pid>/cgroup
```

---

## API Reference

### Package: container

#### Container Struct

```go
type Container struct {
    ID          string
    Bundle      string
    StateDir    string
    Spec        *spec.Spec
    State       *spec.ContainerState
    InitProcess int
    CgroupPath  string
}
```

#### Key Functions

```go
// Create new container instance
func New(ctx context.Context, id, bundle, stateRoot string) (*Container, error)

// Load existing container
func Load(ctx context.Context, id, stateRoot string) (*Container, error)

// List all containers
func List(ctx context.Context, stateRoot string) ([]*Container, error)

// Container operations
func (c *Container) Create(ctx context.Context, opts *CreateOptions) error
func (c *Container) Start(ctx context.Context) error
func (c *Container) Run(ctx context.Context, opts *CreateOptions) error
func (c *Container) Exec(ctx context.Context, args []string, opts *ExecOptions) error
func (c *Container) Signal(sig syscall.Signal) error
func (c *Container) Wait(ctx context.Context) (int, error)
func (c *Container) Delete() error

// State queries
func (c *Container) GetState() *spec.State
func (c *Container) IsRunning() bool
func (c *Container) RefreshStatus()
```

### Package: spec

#### Key Types

```go
type Spec struct {
    Version     string
    Process     *Process
    Root        *Root
    Hostname    string
    Mounts      []Mount
    Hooks       *Hooks
    Linux       *Linux
    Annotations map[string]string
}

type Process struct {
    Terminal     bool
    User         User
    Args         []string
    Env          []string
    Cwd          string
    Capabilities *LinuxCapabilities
    Rlimits      []POSIXRlimit
}

type State struct {
    Version     string
    ID          string
    Status      ContainerStatus
    Pid         int
    Bundle      string
    Annotations map[string]string
}
```

### Package: linux

#### Key Functions

```go
// Namespace operations
func BuildSysProcAttr(spec *spec.Spec) (*syscall.SysProcAttr, error)
func SetNamespaces(namespaces []spec.LinuxNamespace) error

// Cgroup operations
func NewCgroup(path string) (*Cgroup, error)
func (c *Cgroup) ApplyResources(resources *spec.LinuxResources) error
func (c *Cgroup) AddProcess(pid int) error

// Security
func SetupSeccomp(config *spec.LinuxSeccomp) error
func ApplyCapabilities(caps *spec.LinuxCapabilities) error

// Filesystem
func SetupRootfs(spec *spec.Spec, bundle string) error
func SecureJoin(root, path string) (string, error)
```

### Package: hooks

```go
// Hook types
const (
    Prestart        HookType = "prestart"
    CreateRuntime   HookType = "createRuntime"
    CreateContainer HookType = "createContainer"
    StartContainer  HookType = "startContainer"
    Poststart       HookType = "poststart"
    Poststop        HookType = "poststop"
)

// Execute hooks
func Run(hooks *spec.Hooks, hookType HookType, state *spec.State) error
func RunWithState(hooks *spec.Hooks, hookType HookType, id string, pid int, bundle string, status spec.ContainerStatus) error
```

### Package: errors

```go
// Error kinds
const (
    ErrNotFound      ErrorKind = "not_found"
    ErrAlreadyExists ErrorKind = "already_exists"
    ErrInvalidState  ErrorKind = "invalid_state"
    ErrInvalidConfig ErrorKind = "invalid_config"
    ErrPermission    ErrorKind = "permission"
    ErrResource      ErrorKind = "resource"
    ErrInternal      ErrorKind = "internal"
    // ...
)

// Error creation
func New(kind ErrorKind, operation, message string) error
func Wrap(err error, kind ErrorKind, operation string) error
func WrapWithContainer(err error, kind ErrorKind, operation, containerID string) error
```

---

## Contributing

### Development Setup

```bash
# Clone
git clone https://github.com/your-org/runc-go.git
cd runc-go

# Install dependencies
go mod download

# Build
go build -o runc-go .

# Run tests
go test -race ./...
```

### Code Style

- Follow Go conventions (gofmt, golint)
- Use meaningful variable names
- Add comments for exported functions
- Include tests for new features

### Pull Request Process

1. Fork the repository
2. Create feature branch: `git checkout -b feature/my-feature`
3. Make changes with tests
4. Run tests: `go test -race ./...`
5. Commit: `git commit -m "Add my feature"`
6. Push: `git push origin feature/my-feature`
7. Create Pull Request

### Testing Requirements

- All tests must pass
- Race detector must pass: `go test -race ./...`
- New features require tests
- Security-sensitive changes require security tests

---

## License

This project is licensed under the Apache License 2.0.

---

## Acknowledgments

- [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec)
- [runc](https://github.com/opencontainers/runc) - Reference implementation
- [containerd](https://github.com/containerd/containerd) - Container runtime

---

*Documentation generated for runc-go v0.1.0*
