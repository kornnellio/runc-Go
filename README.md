# runc-go

A production-quality, OCI-compliant container runtime written in Go.

> **Full Documentation:** See [DOCUMENTATION.md](DOCUMENTATION.md) for comprehensive technical documentation including architecture diagrams, API reference, and implementation details.

---

## Table of Contents

1. [What is This?](#what-is-this)
2. [Features](#features)
3. [Quick Start](#quick-start)
4. [Installation](#installation)
5. [Using with Docker](#using-with-docker)
6. [Using Standalone](#using-standalone)
7. [Commands Reference](#commands-reference)
8. [Configuration](#configuration)
9. [How Containers Work](#how-containers-work)
10. [Architecture](#architecture)
11. [Error Handling](#error-handling)
12. [Logging](#logging)
13. [Security Features](#security-features)
14. [Testing](#testing)
15. [CI/CD Pipeline](#cicd-pipeline)
16. [Troubleshooting](#troubleshooting)
17. [Project Structure](#project-structure)
18. [Differences from Production runc](#differences-from-production-runc)
19. [Learning Resources](#learning-resources)
20. [FAQ](#faq)
21. [Contributing](#contributing)

---

## What is This?

`runc-go` is a container runtime that implements the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec). It's designed as both an educational resource and a production-quality implementation with modern Go practices.

### The Car Analogy

Think of the container ecosystem like buying a car:

| Component | Car Analogy | What It Does |
|-----------|-------------|--------------|
| **Docker Hub** | Car dealership inventory | Stores container images |
| **Docker CLI** | Car salesperson | Finds and downloads images |
| **containerd** | Car dealership manager | Manages images and containers |
| **runc-go** | The engine | Actually runs the container |

When you run `docker run alpine echo "hello"`:
1. Docker downloads the Alpine image from Docker Hub
2. Docker unpacks it to a directory (the "bundle")
3. Docker generates a config.json with your settings
4. Docker calls `runc-go` to actually run the container
5. `runc-go` sets up isolation and executes your command

### What Makes a Container?

A container is NOT a virtual machine. It's just a regular Linux process with extra isolation:

```
+------------------------------------------+
|              HOST SYSTEM                 |
|                                          |
|  +----------------+  +----------------+  |
|  |  Container A   |  |  Container B   |  |
|  |                |  |                |  |
|  |  PID 1: /bin/sh|  |  PID 1: nginx  |  |
|  |  hostname: app |  |  hostname: web |  |
|  |  rootfs: /tmp/a|  |  rootfs: /tmp/b|  |
|  |  memory: 512MB |  |  memory: 1GB   |  |
|  +----------------+  +----------------+  |
|                                          |
|  Host sees these as PID 5001 and 5002    |
+------------------------------------------+
```

The container thinks it's PID 1, has its own hostname, its own filesystem, and can only use limited memory. But to the host, it's just processes 5001 and 5002.

---

## Features

### Production-Quality Features

| Feature | Description |
|---------|-------------|
| **Cobra CLI** | Modern CLI framework with subcommands, flags, and auto-completion |
| **Custom Error Types** | Rich error handling with `errors.Is()` and `errors.As()` support |
| **Structured Logging** | JSON/text logging with `log/slog` for observability |
| **Context Support** | Full `context.Context` propagation for cancellation and timeouts |
| **Security Hardening** | Path traversal protection, device whitelisting, capability dropping |
| **CI/CD Pipeline** | GitHub Actions for lint, test, security scanning, and releases |

### Container Features

| Feature | Description |
|---------|-------------|
| **Namespaces** | PID, Mount, Network, UTS, IPC, User, Cgroup isolation |
| **Cgroups v2** | Memory, CPU, and PID resource limits |
| **Capabilities** | Fine-grained privilege management |
| **Seccomp** | Syscall filtering with BPF |
| **PTY Support** | Interactive terminal sessions |
| **OCI Compliant** | Full OCI Runtime Specification 1.0.2 support |

---

## Quick Start

### 30-Second Demo

```bash
# Build
git clone <repo>
cd runc-go
make build

# Install
sudo cp runc-go /usr/local/bin/

# Configure Docker to use runc-go
sudo tee /etc/docker/daemon.json << 'EOF'
{
  "runtimes": {
    "runc-go": {
      "path": "/usr/local/bin/runc-go"
    }
  }
}
EOF
sudo systemctl restart docker

# Run a container!
sudo docker run --runtime=runc-go --rm alpine echo "Hello from runc-go!"

# Interactive shell
sudo docker run --runtime=runc-go --rm -it alpine
```

---

## Installation

### Requirements

| Requirement | Minimum Version | How to Check | Notes |
|-------------|-----------------|--------------|-------|
| **Linux** | Kernel 5.0+ | `uname -r` | Containers use Linux-specific features |
| **Go** | 1.24.0+ | `go version` | Required for building |
| **Root access** | - | `sudo whoami` | Namespaces require privileges |
| **Docker** | 20.10+ | `docker --version` | Optional, for easy testing |
| **nsenter** | - | `which nsenter` | Required for `exec` command |

### Building from Source

```bash
# Clone the repository
git clone <repo-url>
cd runc-go

# Build (creates optimized binary)
make build

# Or build with debug symbols (for development)
make build-debug

# Run tests
make test

# Run linter
make lint

# Install to /usr/local/bin
sudo make install
```

### Build Options

| Command | Description |
|---------|-------------|
| `make build` | Build optimized binary (stripped symbols, ~4MB) |
| `make build-debug` | Build with debug symbols (for gdb/delve) |
| `make test` | Run all unit tests |
| `make test-coverage` | Generate HTML coverage report |
| `make test-race` | Run tests with race detection |
| `make lint` | Run golangci-lint |
| `make clean` | Remove build artifacts |
| `make help` | Show all available targets |

### Verify Installation

```bash
# Check version
runc-go version
# Output:
# runc-go version 0.1.0
# spec: 1.0.2
# go: go1.24.x

# Check CLI help
runc-go --help
```

---

## Using with Docker

Docker can use custom runtimes instead of the default `runc`. This lets you test `runc-go` with real containers.

### Step 1: Configure Docker

Create or edit `/etc/docker/daemon.json`:

```bash
sudo tee /etc/docker/daemon.json << 'EOF'
{
  "runtimes": {
    "runc-go": {
      "path": "/usr/local/bin/runc-go"
    }
  }
}
EOF
```

### Step 2: Restart Docker

```bash
sudo systemctl restart docker
```

### Step 3: Verify Configuration

```bash
sudo docker info | grep -A5 Runtimes
```

You should see `runc-go` in the list.

### Step 4: Run Containers

```bash
# Basic test
sudo docker run --runtime=runc-go --rm alpine echo "It works!"

# Interactive shell
sudo docker run --runtime=runc-go --rm -it alpine

# Run in background
sudo docker run --runtime=runc-go -d --name myapp alpine sleep 3600

# Exec into running container
sudo docker exec -it myapp sh

# Stop and remove
sudo docker stop myapp && sudo docker rm myapp
```

### Docker Usage Examples

| Use Case | Command |
|----------|---------|
| Simple command | `docker run --runtime=runc-go --rm alpine echo "hi"` |
| Interactive shell | `docker run --runtime=runc-go --rm -it alpine sh` |
| Background daemon | `docker run --runtime=runc-go -d --name web nginx` |
| With port mapping | `docker run --runtime=runc-go -d -p 8080:80 nginx` |
| With volume | `docker run --runtime=runc-go -v /host:/container alpine ls /container` |
| With memory limit | `docker run --runtime=runc-go --memory=128m alpine free -m` |
| With CPU limit | `docker run --runtime=runc-go --cpus=0.5 alpine` |
| With environment | `docker run --runtime=runc-go -e MY_VAR=hello alpine printenv MY_VAR` |

---

## Using Standalone

You can use `runc-go` without Docker by creating an OCI bundle manually.

### What is an OCI Bundle?

An OCI bundle is a directory containing:
```
mybundle/
├── config.json    # Container configuration (what to run, limits, mounts)
└── rootfs/        # Root filesystem (the container's /)
    ├── bin/
    ├── etc/
    ├── lib/
    └── ...
```

### Creating a Bundle from Docker Image

```bash
# Create bundle directory
mkdir -p /tmp/mybundle/rootfs

# Extract an image to rootfs
docker export $(docker create alpine) | tar -xf - -C /tmp/mybundle/rootfs

# Generate default config
runc-go spec > /tmp/mybundle/config.json
```

### Running the Bundle

```bash
# Run interactively (blocks until container exits)
sudo runc-go run mycontainer -b /tmp/mybundle

# Or use two-step create/start
sudo runc-go create mycontainer -b /tmp/mybundle
sudo runc-go start mycontainer

# Check status
sudo runc-go state mycontainer

# Stop it
sudo runc-go kill mycontainer

# Remove it
sudo runc-go delete mycontainer
```

---

## Commands Reference

### Global Options

These options apply to all commands:

```bash
runc-go [global-options] <command> [command-options]
```

| Option | Description | Default |
|--------|-------------|---------|
| `--root <path>` | State directory for containers | `/run/runc-go` |
| `--log <file>` | Log file path | stderr |
| `--log-format <fmt>` | Log format: `text` or `json` | `text` |
| `--debug` | Enable debug logging | false |
| `--systemd-cgroup` | Use systemd cgroup driver | (compatibility flag) |

Example: `runc-go --root /var/run/myruntime --log /tmp/runc.log create ...`

### Commands

#### `create` - Create a Container (Paused)

Creates a container but doesn't start the user process. Use `start` to begin execution.

```bash
runc-go create <container-id> [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--bundle` | `-b` | Path to the bundle directory (default: `.`) |
| `--pid-file` | | Write container PID to file |
| `--console-socket` | | Unix socket for receiving console FD |
| `--no-pivot` | | Use chroot instead of pivot_root |
| `--no-new-keyring` | | Don't create a new session keyring |

**Example:**
```bash
sudo runc-go create myapp -b /tmp/bundle
sudo runc-go start myapp   # Now start it
```

---

#### `start` - Start a Created Container

Signals a created container to begin executing its process.

```bash
runc-go start <container-id>
```

**Example:**
```bash
sudo runc-go create myapp -b /tmp/bundle
sudo runc-go state myapp   # Status: "created"
sudo runc-go start myapp   # Start it
sudo runc-go state myapp   # Status: "running"
```

---

#### `run` - Create and Start a Container

Creates, starts, and waits for a container to exit.

```bash
runc-go run <container-id> [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--bundle` | `-b` | Path to the bundle directory (default: `.`) |
| `--detach` | `-d` | Run container in background |
| `--pid-file` | | Write container PID to file |
| `--console-socket` | | Unix socket for receiving console FD |

**Examples:**
```bash
# Run and wait for exit
sudo runc-go run myapp -b /tmp/bundle

# Run in background
sudo runc-go run -d myapp -b /tmp/bundle
```

---

#### `exec` - Execute Command in Running Container

Runs a new process inside an existing container.

```bash
runc-go exec <container-id> <command> [args...] [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--tty` | `-t` | Allocate a pseudo-TTY |
| `--detach` | `-d` | Run process in background |
| `--cwd` | | Working directory inside container |
| `--env` | `-e` | Set environment variables |
| `--process` | `-p` | Path to process.json file |
| `--user` | `-u` | User to execute as (uid:gid) |
| `--pid-file` | | Write process PID to file |
| `--console-socket` | | Unix socket for receiving console FD |

**Examples:**
```bash
# Run a command
sudo runc-go exec myapp ls -la

# Interactive shell
sudo runc-go exec -t myapp /bin/sh

# With environment variable
sudo runc-go exec -e FOO=bar myapp printenv FOO
```

---

#### `state` - Query Container State

Outputs the container state as JSON (OCI-compliant format).

```bash
runc-go state <container-id>
```

**Output:**
```json
{
  "ociVersion": "1.0.2",
  "id": "myapp",
  "status": "running",
  "pid": 12345,
  "bundle": "/tmp/bundle",
  "annotations": {}
}
```

**Status values:**
| Status | Description |
|--------|-------------|
| `creating` | Being set up |
| `created` | Created but not started |
| `running` | Process is running |
| `stopped` | Process has exited |

---

#### `kill` - Send Signal to Container

Sends a signal to the container's init process.

```bash
runc-go kill <container-id> [signal] [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--all` | `-a` | Send signal to all processes in container |

**Examples:**
```bash
# Graceful shutdown (default: SIGTERM)
sudo runc-go kill myapp

# Force kill
sudo runc-go kill myapp SIGKILL

# Kill all processes
sudo runc-go kill -a myapp SIGKILL
```

---

#### `delete` - Remove a Container

Removes container state. Container must be stopped unless `--force` is used.

```bash
runc-go delete <container-id> [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Force kill running container before delete |

**Examples:**
```bash
# Delete stopped container
sudo runc-go delete myapp

# Force delete (kills if running)
sudo runc-go delete -f myapp
```

---

#### `list` - List All Containers

Lists all containers managed by this runtime.

```bash
runc-go list [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--quiet` | `-q` | Display only container IDs |
| `--format` | `-f` | Output format: `table` or `json` |

**Output (table):**
```
ID          PID     STATUS      BUNDLE                  CREATED
myapp       12345   running     /tmp/bundle             2024-01-15 10:30:00
testapp     0       stopped     /tmp/test               2024-01-15 09:00:00
```

**Output (json):**
```json
[
  {
    "id": "myapp",
    "pid": 12345,
    "status": "running",
    "bundle": "/tmp/bundle",
    "created": "2024-01-15T10:30:00Z"
  }
]
```

---

#### `spec` - Generate Default Config

Generates a default `config.json` for an OCI bundle.

```bash
runc-go spec [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--bundle` | `-b` | Bundle directory |
| `--rootless` | | Generate rootless spec |

**Example:**
```bash
mkdir -p mybundle/rootfs
cd mybundle
runc-go spec > config.json
# Edit config.json as needed
```

---

#### `version` - Print Version Information

```bash
runc-go version
```

**Output:**
```
runc-go version 0.1.0
spec: 1.0.2
go: go1.24.x
```

---

#### Shell Completion

Cobra provides built-in shell completion:

```bash
# Bash
runc-go completion bash > /etc/bash_completion.d/runc-go

# Zsh
runc-go completion zsh > "${fpath[1]}/_runc-go"

# Fish
runc-go completion fish > ~/.config/fish/completions/runc-go.fish
```

---

## Configuration

### config.json Structure

The `config.json` file follows the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec/blob/main/config.md).

### Minimal Example

```json
{
  "ociVersion": "1.0.2",
  "root": {
    "path": "rootfs",
    "readonly": false
  },
  "process": {
    "terminal": false,
    "user": { "uid": 0, "gid": 0 },
    "args": ["/bin/sh", "-c", "echo hello"],
    "env": ["PATH=/usr/bin:/bin", "TERM=xterm"],
    "cwd": "/"
  },
  "hostname": "container",
  "linux": {
    "namespaces": [
      {"type": "pid"},
      {"type": "mount"}
    ]
  }
}
```

### Full Example with All Features

```json
{
  "ociVersion": "1.0.2",
  "root": {
    "path": "rootfs",
    "readonly": false
  },
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
      "TERM=xterm-256color",
      "HOME=/root"
    ],
    "cwd": "/",
    "capabilities": {
      "bounding": ["CAP_AUDIT_WRITE", "CAP_KILL", "CAP_NET_BIND_SERVICE"],
      "effective": ["CAP_AUDIT_WRITE", "CAP_KILL", "CAP_NET_BIND_SERVICE"],
      "permitted": ["CAP_AUDIT_WRITE", "CAP_KILL", "CAP_NET_BIND_SERVICE"],
      "ambient": ["CAP_AUDIT_WRITE", "CAP_KILL", "CAP_NET_BIND_SERVICE"]
    },
    "rlimits": [
      { "type": "RLIMIT_NOFILE", "hard": 1024, "soft": 1024 }
    ],
    "noNewPrivileges": true
  },
  "hostname": "my-container",
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
      "destination": "/sys",
      "type": "sysfs",
      "source": "sysfs",
      "options": ["nosuid", "noexec", "nodev", "ro"]
    }
  ],
  "linux": {
    "namespaces": [
      {"type": "pid"},
      {"type": "network"},
      {"type": "ipc"},
      {"type": "uts"},
      {"type": "mount"},
      {"type": "cgroup"}
    ],
    "resources": {
      "memory": {
        "limit": 536870912,
        "reservation": 268435456
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
          "names": ["read", "write", "exit", "exit_group"],
          "action": "SCMP_ACT_ALLOW"
        }
      ]
    },
    "maskedPaths": [
      "/proc/acpi",
      "/proc/kcore",
      "/proc/keys"
    ],
    "readonlyPaths": [
      "/proc/bus",
      "/proc/fs",
      "/proc/sys"
    ]
  }
}
```

---

## How Containers Work

### The Isolation Stack

```
┌─────────────────────────────────────────────────────────────┐
│                     CONTAINER PROCESS                        │
│                                                              │
│  Sees itself as PID 1, has its own hostname, filesystem,    │
│  network, and limited resources                              │
└─────────────────────────────────────────────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│  NAMESPACES │      │   CGROUPS   │      │  SECURITY   │
│             │      │             │      │             │
│ • PID       │      │ • Memory    │      │ • Caps      │
│ • Mount     │      │ • CPU       │      │ • Seccomp   │
│ • Network   │      │ • PIDs      │      │ • No new    │
│ • UTS       │      │             │      │   privs     │
│ • IPC       │      │             │      │             │
└─────────────┘      └─────────────┘      └─────────────┘
         │                    │                    │
         └────────────────────┼────────────────────┘
                              │
                              ▼
                     ┌─────────────┐
                     │   KERNEL    │
                     │             │
                     │ Enforces    │
                     │ all limits  │
                     └─────────────┘
```

### Linux Namespaces

| Namespace | What It Isolates | Effect |
|-----------|------------------|--------|
| **PID** | Process IDs | Container's first process is PID 1 |
| **Mount** | Filesystem mounts | Container has its own root filesystem |
| **Network** | Network stack | Container has its own IP, ports, routing |
| **UTS** | Hostname | Container has its own hostname |
| **IPC** | Inter-process communication | Isolated shared memory, semaphores |
| **User** | UID/GID mappings | Container UID 0 maps to unprivileged host UID |
| **Cgroup** | Cgroup view | Container sees only its cgroup |

### Container Lifecycle

```
                    create                start             process exits
        (none) ──────────────► created ──────────► running ──────────────► stopped
                                  │                   │                      │
                                  │    kill SIGKILL   │                      │
                                  │◄──────────────────┘                      │
                                  │                                          │
                                  └────────────────────────────────────────►│
                                                                    delete   │
                                                                             ▼
                                                                          (none)
```

---

## Architecture

### Code Flow

```
main.go
   │
   └─► cmd.Execute()                    Cobra CLI entry point
            │
            ├─► cmd/create.go           Create container
            │       └─► container.New() + container.Create()
            │
            ├─► cmd/start.go            Start container
            │       └─► container.Load() + c.Start()
            │
            ├─► cmd/run.go              Run (create + start + wait)
            │       └─► container.New() + c.Run() + c.Wait()
            │
            ├─► cmd/exec.go             Exec into container
            │       └─► container.Exec() / ExecWithProcessFile()
            │
            ├─► cmd/kill.go             Send signal
            │       └─► container.Kill()
            │
            ├─► cmd/delete.go           Delete container
            │       └─► container.Delete()
            │
            └─► cmd/list.go             List containers
                    └─► container.List()
```

### Package Responsibilities

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `cmd` | Cobra CLI commands | root.go, create.go, run.go, exec.go, etc. |
| `container` | Container lifecycle management | container.go, create.go, start.go, exec.go |
| `linux` | Linux isolation primitives | namespace.go, rootfs.go, cgroup.go, capabilities.go, seccomp.go |
| `spec` | OCI specification types | spec.go, state.go |
| `errors` | Custom error types | errors.go, sentinel.go |
| `logging` | Structured logging | logger.go |
| `utils` | PTY handling, utilities | console.go |
| `hooks` | OCI lifecycle hooks | hooks.go |

---

## Error Handling

### Custom Error Types

runc-go uses custom error types for rich error handling:

```go
// Check error type
if errors.Is(err, cerrors.ErrNotFound) {
    fmt.Println("Container not found")
}

// Extract error details
var containerErr *cerrors.ContainerError
if errors.As(err, &containerErr) {
    fmt.Printf("Operation: %s, Container: %s\n",
        containerErr.Op, containerErr.Container)
}
```

### Error Kinds

| Error Kind | Description |
|------------|-------------|
| `ErrNotFound` | Container or resource not found |
| `ErrAlreadyExists` | Container already exists |
| `ErrInvalidState` | Container in wrong state for operation |
| `ErrInvalidConfig` | Invalid configuration |
| `ErrPermission` | Permission denied |
| `ErrResource` | Resource allocation error |
| `ErrNamespace` | Namespace error |
| `ErrCgroup` | Cgroup error |
| `ErrSeccomp` | Seccomp filter error |
| `ErrCapability` | Capability error |

### Sentinel Errors

```go
// Common sentinel errors
cerrors.ErrContainerNotFound
cerrors.ErrContainerExists
cerrors.ErrContainerNotRunning
cerrors.ErrPathTraversal
cerrors.ErrNoProcessArgs
cerrors.ErrEmptyContainerID
```

---

## Logging

### Structured Logging with slog

runc-go uses Go 1.21+ `log/slog` for structured logging:

```bash
# Text output (default)
sudo runc-go run myapp -b /bundle

# JSON output (for log aggregation)
sudo runc-go --log-format=json run myapp -b /bundle

# Log to file
sudo runc-go --log=/var/log/runc-go.log run myapp -b /bundle

# Debug logging
sudo runc-go --debug run myapp -b /bundle
```

### Log Format Examples

**Text Format:**
```
time=2024-01-15T10:30:00Z level=INFO msg="container created" container_id=myapp
time=2024-01-15T10:30:01Z level=INFO msg="container started" container_id=myapp
```

**JSON Format:**
```json
{"time":"2024-01-15T10:30:00Z","level":"INFO","msg":"container created","container_id":"myapp"}
{"time":"2024-01-15T10:30:01Z","level":"INFO","msg":"container started","container_id":"myapp"}
```

### Context-Aware Logging

Logs include context information for tracing:

```go
// In code
logging.InfoContext(ctx, "starting container", "container_id", c.ID)
logging.WarnContext(ctx, "deprecated option", "option", "no-pivot")
logging.ErrorContext(ctx, "failed to create", "error", err)
```

---

## Security Features

### Input Validation

#### Container ID Validation

Container IDs must match: `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`

```bash
# These FAIL (security violation)
sudo runc-go create "../../../etc" -b /bundle    # Path traversal
sudo runc-go create "test;rm -rf /" -b /bundle   # Shell injection
sudo runc-go create "" -b /bundle                 # Empty ID

# These PASS
sudo runc-go create "myapp" -b /bundle
sudo runc-go create "my-app_v1.0" -b /bundle
```

#### Path Traversal Protection

All paths are validated to stay within the container's rootfs using `SecureJoin()`:

1. Resolves all symlinks
2. Normalizes paths
3. Ensures result stays within base directory

#### Device Whitelist

Only safe devices can be created:

**Allowed:**
- `/dev/null`, `/dev/zero`, `/dev/full`
- `/dev/random`, `/dev/urandom`
- `/dev/tty`, `/dev/console`, `/dev/ptmx`

**Blocked:**
- `/dev/sda`, `/dev/sdb` (block devices)
- `/dev/mem`, `/dev/kmem` (memory access)

### Privilege Restriction

#### Linux Capabilities

Containers get limited capabilities instead of full root. Dangerous capabilities like `CAP_SYS_ADMIN`, `CAP_SYS_PTRACE`, and `CAP_SYS_MODULE` are not granted.

#### Seccomp Filtering

Syscall filtering using BPF restricts which system calls containers can make.

### File Permissions

| File | Mode | Purpose |
|------|------|---------|
| State files | 0600 | Prevent info disclosure |
| Log files | 0600 | Prevent info disclosure |
| Cgroup files | 0644 | Standard cgroup perms |

---

## Testing

### Run Tests

```bash
# All tests
make test

# With coverage
make test-coverage

# With race detection
make test-race

# Specific package
go test ./container/...
go test ./errors/...
go test ./linux/...
```

### Test Categories

| Package | Tests | Focus |
|---------|-------|-------|
| `errors` | Error wrapping, Is/As | Error handling correctness |
| `logging` | Logger creation, output | Logging functionality |
| `container` | Lifecycle, state | Container management |
| `linux` | SecureJoin, capabilities | Security primitives |

### Integration Tests

```bash
# With Docker
sudo docker run --runtime=runc-go --rm alpine echo "works!"

# Standalone
sudo runc-go run test -b /tmp/bundle
```

---

## CI/CD Pipeline

### GitHub Actions Workflows

#### CI Pipeline (`.github/workflows/ci.yml`)

Runs on every push and pull request:

| Job | Description |
|-----|-------------|
| **lint** | golangci-lint with comprehensive rules |
| **test** | Unit tests with race detection |
| **test-coverage** | Coverage reporting |
| **security** | gosec security scanner |
| **build** | Cross-compile for amd64/arm64 |

#### Release Pipeline (`.github/workflows/release.yml`)

Runs on version tags (`v*`):

1. Runs full test suite
2. Builds release binaries for linux/amd64 and linux/arm64
3. Creates GitHub release with binaries and checksums
4. Auto-detects prereleases (alpha, beta, rc)

### Linter Configuration (`.golangci.yml`)

Enabled linters:
- `errcheck` - Unchecked errors
- `gosimple` - Code simplification
- `govet` - Go vet checks
- `staticcheck` - Static analysis
- `gosec` - Security issues
- `gocritic` - Code quality
- `gocyclo` - Cyclomatic complexity (max 20)
- And more...

---

## Troubleshooting

### Common Issues

#### "permission denied"

**Solution:** Use `sudo` - container operations require root privileges.

```bash
sudo runc-go run myapp -b /bundle
```

#### "container not found"

**Solution:** Check container list:
```bash
sudo runc-go list
```

#### "container is not running"

**Solution:** Start the container first:
```bash
sudo runc-go state myapp
sudo runc-go start myapp
```

#### "nsenter: failed to execute"

**Solution:** Install util-linux:
```bash
# Debian/Ubuntu
sudo apt install util-linux

# RHEL/CentOS
sudo yum install util-linux
```

### Debug Logging

```bash
# Enable debug output
sudo runc-go --debug --log=/tmp/runc.log run myapp -b /bundle

# View JSON logs
cat /tmp/runc.log | jq .
```

### Check Runtime State

```bash
# List all containers
sudo runc-go list

# Check specific container
sudo runc-go state myapp

# Check cgroup
cat /sys/fs/cgroup/runc-go/myapp/cgroup.procs
```

---

## Project Structure

```
runc-go/
├── main.go                 # Entry point (calls cmd.Execute())
│
├── cmd/                    # Cobra CLI commands
│   ├── root.go             # Root command, global flags
│   ├── create.go           # create command
│   ├── start.go            # start command
│   ├── run.go              # run command
│   ├── exec.go             # exec command
│   ├── state.go            # state command
│   ├── kill.go             # kill command
│   ├── delete.go           # delete command
│   ├── list.go             # list command
│   ├── spec.go             # spec command
│   ├── version.go          # version command
│   └── init.go             # init/exec-init (internal)
│
├── container/              # Container lifecycle management
│   ├── container.go        # Container struct, state, validation
│   ├── create.go           # Create operation
│   ├── start.go            # Start, Run, Wait operations
│   ├── exec.go             # Exec into container
│   ├── kill.go             # Signal handling
│   ├── delete.go           # Cleanup
│   ├── state.go            # State query
│   ├── syscalls.go         # Low-level syscall wrappers
│   └── container_test.go   # Unit tests
│
├── linux/                  # Linux-specific isolation
│   ├── namespace.go        # Namespace creation/joining
│   ├── rootfs.go           # Root filesystem, pivot_root
│   ├── cgroup.go           # Cgroups v2 resource limits
│   ├── capabilities.go     # Linux capability management
│   ├── seccomp.go          # Seccomp BPF filtering
│   ├── devices.go          # Device node management
│   ├── rootfs_test.go      # Security tests
│   └── capabilities_test.go # Capability tests
│
├── errors/                 # Custom error types
│   ├── errors.go           # ContainerError, ErrorKind
│   ├── sentinel.go         # Predefined sentinel errors
│   └── errors_test.go      # Error handling tests
│
├── logging/                # Structured logging
│   ├── logger.go           # slog-based logging
│   └── logger_test.go      # Logger tests
│
├── spec/                   # OCI specification types
│   ├── spec.go             # config.json structures
│   ├── state.go            # Container state
│   ├── spec_test.go        # Spec tests
│   └── state_test.go       # State tests
│
├── utils/                  # Utility functions
│   └── console.go          # PTY/terminal handling
│
├── hooks/                  # OCI lifecycle hooks
│   └── hooks.go            # Hook execution
│
├── .github/                # GitHub configuration
│   └── workflows/
│       ├── ci.yml          # CI pipeline
│       └── release.yml     # Release automation
│
├── .golangci.yml           # Linter configuration
├── Makefile                # Build automation
├── go.mod                  # Go module
├── go.sum                  # Dependency checksums
├── README.md               # This file
└── FLOW.md                 # Execution flow documentation
```

### Lines of Code by Package

| Package | Lines | Description |
|---------|-------|-------------|
| cmd | ~500 | Cobra CLI commands |
| container | ~1,800 | Lifecycle management |
| linux | ~2,500 | Isolation primitives |
| spec | ~800 | OCI types |
| errors | ~200 | Error handling |
| logging | ~150 | Structured logging |
| utils | ~400 | Utilities |
| hooks | ~150 | Hook execution |
| **Total** | ~6,500 | |

---

## Differences from Production runc

| Aspect | runc-go | runc (Production) |
|--------|---------|-------------------|
| **Purpose** | Educational + Quality | Production use |
| **Code size** | ~6,500 lines | ~50,000+ lines |
| **CLI Framework** | Cobra | urfave/cli |
| **Cgroup support** | v2 only | v1 and v2 |
| **Seccomp** | Basic BPF | Full libseccomp |
| **Checkpoint/restore** | No | Yes (CRIU) |
| **systemd integration** | No | Yes |
| **AppArmor** | No | Yes |
| **SELinux** | No | Yes |
| **Rootless mode** | Partial | Full |
| **Error handling** | Custom types | Basic |
| **Logging** | slog structured | logrus |

---

## Learning Resources

### Recommended Reading Order

1. **Start here:** `cmd/root.go` - Cobra CLI structure
2. **Container creation:** `container/create.go` - The heart of container setup
3. **Namespaces:** `linux/namespace.go` - How isolation is achieved
4. **Filesystem:** `linux/rootfs.go` - pivot_root and mount handling
5. **Execution:** `container/exec.go` - How exec enters containers
6. **Security:** `linux/capabilities.go`, `linux/seccomp.go` - Privilege restriction
7. **Error handling:** `errors/errors.go` - Custom error types

### External Resources

- [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec)
- [Linux Namespaces (man7)](https://man7.org/linux/man-pages/man7/namespaces.7.html)
- [Cgroups v2 (kernel.org)](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)
- [Linux Capabilities (man7)](https://man7.org/linux/man-pages/man7/capabilities.7.html)
- [Seccomp (kernel.org)](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)
- [Cobra CLI](https://github.com/spf13/cobra)
- [Go slog](https://pkg.go.dev/log/slog)

---

## FAQ

### General Questions

**Q: Is this production-ready?**

A: This implementation follows production-quality practices (error handling, logging, testing, CI/CD) but lacks some features of the official runc (systemd cgroups, AppArmor, SELinux, CRIU). Use official runc for critical production workloads.

**Q: What's the difference between Docker and runc-go?**

A: Docker is a full container platform (images, networking, orchestration). runc-go is just the runtime that executes containers. Docker uses runc (or runc-go) internally.

**Q: Why doesn't it work on macOS/Windows?**

A: Containers use Linux-specific features (namespaces, cgroups). macOS/Windows Docker uses a Linux VM.

### Technical Questions

**Q: Why use Cobra instead of urfave/cli?**

A: Cobra is the de-facto standard for Go CLIs, used by Kubernetes, Docker CLI, Hugo, etc. It provides better subcommand handling, flag inheritance, and auto-completion.

**Q: Why custom error types?**

A: Custom error types enable:
- Semantic error checking with `errors.Is()`
- Error type extraction with `errors.As()`
- Rich error context (operation, container ID, underlying error)
- Better debugging and logging

**Q: Why slog instead of logrus?**

A: slog is the standard library structured logging solution (Go 1.21+). It's lightweight, well-designed, and doesn't require external dependencies.

---

## Contributing

Contributions are welcome! When contributing:

1. Follow existing code style and patterns
2. Add tests for new functionality
3. Update documentation as needed
4. Run `make lint` and `make test` before submitting
5. Keep commits focused and well-documented

### Development Setup

```bash
# Clone
git clone <repo>
cd runc-go

# Install dev dependencies
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build and test
make build
make test
make lint
```

---

## License

Educational use. Based on concepts from the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec).
