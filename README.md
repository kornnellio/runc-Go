# runc-go

An educational, OCI-compliant container runtime written in Go.

---

## Table of Contents

1. [What is This?](#what-is-this)
2. [Quick Start](#quick-start)
3. [Installation](#installation)
4. [Using with Docker](#using-with-docker)
5. [Using Standalone](#using-standalone)
6. [Commands Reference](#commands-reference)
7. [Configuration](#configuration)
8. [How Containers Work](#how-containers-work)
9. [Architecture](#architecture)
10. [Security Features](#security-features)
11. [Testing](#testing)
12. [Troubleshooting](#troubleshooting)
13. [Project Structure](#project-structure)
14. [Differences from Production runc](#differences-from-production-runc)
15. [Learning Resources](#learning-resources)
16. [FAQ](#faq)

---

## What is This?

`runc-go` is a container runtime that implements the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec). It's designed to teach how containers actually work under the hood by providing a clean, readable implementation.

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

# Install to /usr/local/bin
sudo make install
```

### Build Options

| Command | Description |
|---------|-------------|
| `make build` | Build optimized binary (stripped symbols, ~3MB) |
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
runc-go --version
# Output: runc-go version 0.1.0, spec: 1.0.2

# Check it's in PATH
which runc-go
# Output: /usr/local/bin/runc-go
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
sudo runc-go run mycontainer /tmp/mybundle

# Or use two-step create/start
sudo runc-go create mycontainer /tmp/mybundle
sudo runc-go start mycontainer

# Check status
sudo runc-go state mycontainer

# Stop it
sudo runc-go kill mycontainer

# Remove it
sudo runc-go delete mycontainer
```

### Creating a Bundle from Scratch

For a minimal container that just prints "Hello":

```bash
# Create directories
mkdir -p /tmp/minimal/rootfs/bin

# Copy a statically-linked binary
cp /bin/busybox /tmp/minimal/rootfs/bin/

# Create config
cat > /tmp/minimal/config.json << 'EOF'
{
  "ociVersion": "1.0.2",
  "root": {
    "path": "rootfs",
    "readonly": false
  },
  "process": {
    "terminal": false,
    "user": { "uid": 0, "gid": 0 },
    "args": ["/bin/busybox", "echo", "Hello from minimal container!"],
    "env": ["PATH=/bin"],
    "cwd": "/"
  },
  "hostname": "minimal",
  "mounts": [
    {
      "destination": "/proc",
      "type": "proc",
      "source": "proc"
    }
  ],
  "linux": {
    "namespaces": [
      {"type": "pid"},
      {"type": "mount"}
    ]
  }
}
EOF

# Run it
sudo runc-go run minimal /tmp/minimal
```

---

## Commands Reference

### Global Options

These options come BEFORE the command:

| Option | Description | Default |
|--------|-------------|---------|
| `--root <path>` | State directory for containers | `/run/runc-go` |
| `--log <file>` | Log file path | stderr |
| `--log-format <fmt>` | Log format: `text` or `json` | text |
| `--systemd-cgroup` | Use systemd cgroup driver | (ignored) |
| `--debug` | Enable debug output | (ignored) |

Example: `runc-go --root /var/run/myruntime --log /tmp/runc.log create ...`

### Commands

#### `run` - Create and Start a Container

Creates, starts, and waits for a container to exit.

```bash
runc-go run [options] <container-id> <bundle-path>
```

| Option | Description |
|--------|-------------|
| `-t, --tty` | Allocate a pseudo-TTY |
| `-d, --detach` | Run in background |
| `--pid-file <path>` | Write container PID to file |
| `--console-socket <path>` | Unix socket for console (Docker uses this) |

**Examples:**
```bash
# Run and wait for exit
sudo runc-go run myapp /tmp/bundle

# Run interactive shell
sudo runc-go run -t myapp /tmp/bundle

# Run in background
sudo runc-go run -d myapp /tmp/bundle
```

---

#### `create` - Create a Container (Paused)

Creates a container but doesn't start the user process. Use `start` to begin execution.

```bash
runc-go create [options] <container-id> <bundle-path>
```

| Option | Description |
|--------|-------------|
| `--pid-file <path>` | Write init PID to file |
| `--console-socket <path>` | Unix socket for console |

**Example:**
```bash
sudo runc-go create myapp /tmp/bundle
sudo runc-go start myapp   # Now start it
```

**What happens:**
1. Forks a child process with namespace flags
2. Child sets up namespaces, mounts, cgroups
3. Child blocks waiting for `start` signal
4. Container is in "created" state

---

#### `start` - Start a Created Container

Signals a created container to begin executing its process.

```bash
runc-go start <container-id>
```

**Example:**
```bash
sudo runc-go create myapp /tmp/bundle
sudo runc-go state myapp   # Status: "created"
sudo runc-go start myapp   # Start it
sudo runc-go state myapp   # Status: "running"
```

---

#### `exec` - Execute Command in Running Container

Runs a new process inside an existing container.

```bash
runc-go exec [options] <container-id> <command> [args...]
```

| Option | Description |
|--------|-------------|
| `-t, --tty` | Allocate a pseudo-TTY |
| `--console-socket <path>` | Unix socket for console |

**Examples:**
```bash
# Run a command
sudo runc-go exec myapp ls -la

# Interactive shell
sudo runc-go exec -t myapp /bin/sh

# Run as different user (if supported by config)
sudo runc-go exec myapp whoami
```

**Note:** The container must be running. Uses `nsenter` internally to join the container's namespaces.

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
runc-go kill [options] <container-id> [signal]
```

| Option | Description |
|--------|-------------|
| `--all` | Send signal to all processes in container |

**Examples:**
```bash
# Graceful shutdown (default: SIGTERM)
sudo runc-go kill myapp

# Force kill
sudo runc-go kill myapp SIGKILL

# Using signal number
sudo runc-go kill myapp 9

# Kill all processes
sudo runc-go kill --all myapp SIGKILL
```

**Common signals:**
| Signal | Number | Effect |
|--------|--------|--------|
| `SIGTERM` | 15 | Graceful termination |
| `SIGKILL` | 9 | Immediate termination |
| `SIGINT` | 2 | Interrupt (like Ctrl+C) |
| `SIGHUP` | 1 | Hangup |
| `SIGUSR1` | 10 | User-defined |
| `SIGUSR2` | 12 | User-defined |

---

#### `delete` - Remove a Container

Removes container state. Container must be stopped unless `--force` is used.

```bash
runc-go delete [options] <container-id>
```

| Option | Description |
|--------|-------------|
| `-f, --force` | Force kill running container before delete |

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
runc-go list
```

**Output:**
```
ID          PID     STATUS      BUNDLE                  CREATED
myapp       12345   running     /tmp/bundle             2024-01-15T10:30:00Z
testapp     0       stopped     /tmp/test               2024-01-15T09:00:00Z
```

---

#### `spec` - Generate Default Config

Generates a default `config.json` for an OCI bundle.

```bash
runc-go spec
```

**Example:**
```bash
mkdir -p mybundle/rootfs
cd mybundle
runc-go spec > config.json
# Edit config.json as needed
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
      "destination": "/dev/pts",
      "type": "devpts",
      "source": "devpts",
      "options": ["nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"]
    },
    {
      "destination": "/dev/shm",
      "type": "tmpfs",
      "source": "shm",
      "options": ["nosuid", "noexec", "nodev", "mode=1777", "size=65536k"]
    },
    {
      "destination": "/sys",
      "type": "sysfs",
      "source": "sysfs",
      "options": ["nosuid", "noexec", "nodev", "ro"]
    },
    {
      "destination": "/data",
      "type": "bind",
      "source": "/host/data",
      "options": ["rbind", "rw"]
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
        "reservation": 268435456,
        "swap": 536870912
      },
      "cpu": {
        "shares": 1024,
        "quota": 100000,
        "period": 100000,
        "cpus": "0-3",
        "mems": "0"
      },
      "pids": {
        "limit": 100
      }
    },
    "devices": [
      {"path": "/dev/null", "type": "c", "major": 1, "minor": 3, "fileMode": 438, "uid": 0, "gid": 0}
    ],
    "seccomp": {
      "defaultAction": "SCMP_ACT_ERRNO",
      "architectures": ["SCMP_ARCH_X86_64", "SCMP_ARCH_X86", "SCMP_ARCH_AARCH64"],
      "syscalls": [
        {
          "names": ["read", "write", "exit", "exit_group", "mmap"],
          "action": "SCMP_ACT_ALLOW"
        }
      ]
    },
    "maskedPaths": [
      "/proc/acpi",
      "/proc/kcore",
      "/proc/keys",
      "/proc/latency_stats",
      "/proc/timer_list",
      "/proc/timer_stats",
      "/proc/sched_debug",
      "/sys/firmware"
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

### Configuration Sections Explained

#### `root` - Filesystem Root

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Path to root filesystem (relative to bundle) |
| `readonly` | bool | Make rootfs read-only |

#### `process` - Container Process

| Field | Type | Description |
|-------|------|-------------|
| `terminal` | bool | Attach a pseudo-TTY |
| `user.uid` | int | User ID to run as |
| `user.gid` | int | Group ID to run as |
| `args` | []string | Command and arguments |
| `env` | []string | Environment variables |
| `cwd` | string | Working directory |
| `capabilities` | object | Linux capabilities (see below) |
| `noNewPrivileges` | bool | Prevent privilege escalation |

#### `mounts` - Filesystem Mounts

| Field | Type | Description |
|-------|------|-------------|
| `destination` | string | Mount point inside container |
| `type` | string | Filesystem type: bind, proc, sysfs, tmpfs, devpts |
| `source` | string | Source path or device |
| `options` | []string | Mount options |

**Common mount options:**
- `ro` / `rw` - Read-only / read-write
- `nosuid` - Ignore setuid/setgid bits
- `nodev` - Ignore device files
- `noexec` - Disallow execution
- `bind` / `rbind` - Bind mount / recursive bind mount

#### `linux.namespaces` - Isolation

| Type | Description |
|------|-------------|
| `pid` | Separate process ID space |
| `network` | Separate network stack |
| `mount` | Separate mount table |
| `ipc` | Separate IPC (shared memory, semaphores) |
| `uts` | Separate hostname/domainname |
| `user` | Separate UID/GID mapping |
| `cgroup` | Separate cgroup view |

#### `linux.resources` - Resource Limits

**Memory:**
| Field | Description |
|-------|-------------|
| `limit` | Hard memory limit in bytes |
| `reservation` | Soft limit (memory.low) |
| `swap` | Swap limit in bytes |

**CPU:**
| Field | Description |
|-------|-------------|
| `shares` | CPU weight (1-10000, default 1024) |
| `quota` | CPU time quota per period (microseconds) |
| `period` | CPU quota period (default 100000 = 100ms) |
| `cpus` | CPU affinity (e.g., "0-3" or "0,2,4") |
| `mems` | Memory node affinity |

**PIDs:**
| Field | Description |
|-------|-------------|
| `limit` | Maximum number of processes |

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

Namespaces provide isolation by giving containers their own view of system resources:

| Namespace | What It Isolates | Effect |
|-----------|------------------|--------|
| **PID** | Process IDs | Container's first process is PID 1 |
| **Mount** | Filesystem mounts | Container has its own root filesystem |
| **Network** | Network stack | Container has its own IP, ports, routing |
| **UTS** | Hostname | Container has its own hostname |
| **IPC** | Inter-process communication | Isolated shared memory, semaphores |
| **User** | UID/GID mappings | Container UID 0 maps to unprivileged host UID |
| **Cgroup** | Cgroup view | Container sees only its cgroup |

### Cgroups v2

Control Groups limit resource usage:

```
/sys/fs/cgroup/runc-go/mycontainer/
├── cgroup.procs        # PIDs in this cgroup
├── memory.max          # Memory limit (bytes)
├── memory.current      # Current memory usage
├── cpu.max             # CPU quota (quota period)
├── cpu.weight          # CPU shares
├── pids.max            # Max processes
└── pids.current        # Current process count
```

### Container Lifecycle

```
                    create                start             process exits
        (none) ──────────────► created ──────────► running ──────────────► stopped
                                  │                   │                      │
                                  │                   │                      │
                                  │    kill SIGKILL   │                      │
                                  │◄──────────────────┘                      │
                                  │                                          │
                                  └────────────────────────────────────────►│
                                                                    delete   │
                                                                             ▼
                                                                          (none)
```

**State transitions:**
1. **create**: Sets up namespaces, mounts, cgroups. Process blocks waiting for start.
2. **start**: Unblocks init process to execute user command.
3. **running**: User process is executing.
4. **stopped**: User process has exited (exit code available).
5. **delete**: Cleans up cgroup, removes state directory.

### The Create/Start Split

Why separate `create` and `start`?

1. **Orchestration**: Kubernetes/Docker can set up networking between create and start
2. **Validation**: Check configuration before committing resources
3. **Hooks**: Run pre-start hooks between create and start

```
Parent Process                    Child Process (init)
══════════════                    ════════════════════
fork() ────────────────────────► Born in new namespaces
   │                                    │
   ├─ Create PTY                        │
   ├─ Setup cgroup                      │
   │                                    ▼
   │                              Setup hostname
   │                              Setup rootfs (pivot_root)
   │                              Setup /dev, /proc, /sys
   │                              Apply capabilities
   │                              Apply seccomp
   │                                    │
   │                                    ▼
   │                              BLOCK on FIFO read
   │                              (waiting for start)
   │                                    │
start() ─────────────────────────► Write to FIFO
   │                                    │
   │                                    ▼
   │                              UNBLOCK!
   │                              exec() user process
   │                                    │
   │                                    ▼
   │                              User process runs...
   │                                    │
wait() ◄───────────────────────── Process exits
```

---

## Architecture

### Code Flow

```
main.go
   │
   ├─► cmdRun()  ────────────────► container.Run()
   │                                    │
   │                                    ├─► Create() ─► InitContainer()
   │                                    ├─► Start()
   │                                    └─► Wait()
   │
   ├─► cmdCreate() ──────────────► container.Create()
   │                                    │
   │                                    └─► Forks child with clone flags
   │                                              │
   │                                              ▼
   │                                        InitContainer()
   │                                              │
   │                                              ├─► linux.SetupRootfs()
   │                                              ├─► linux.SetupDevices()
   │                                              ├─► linux.ApplyCgroups()
   │                                              ├─► linux.DropCapabilities()
   │                                              └─► linux.ApplySeccomp()
   │
   ├─► cmdStart() ───────────────► container.Start()
   │                                    │
   │                                    └─► Opens FIFO (unblocks init)
   │
   ├─► cmdExec() ────────────────► container.Exec()
   │                                    │
   │                                    └─► nsenter ─► joins namespaces
   │                                              │
   │                                              └─► exec user command
   │
   ├─► cmdKill() ────────────────► container.Kill()
   │                                    │
   │                                    └─► syscall.Kill(pid, signal)
   │
   └─► cmdDelete() ──────────────► container.Delete()
                                        │
                                        ├─► Remove cgroup
                                        └─► Remove state directory
```

### Package Responsibilities

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `main` | CLI parsing, command dispatch | main.go |
| `container` | Container lifecycle management | create.go, start.go, exec.go, kill.go, delete.go |
| `linux` | Linux isolation primitives | namespace.go, rootfs.go, cgroup.go, capabilities.go, seccomp.go |
| `spec` | OCI specification types | spec.go, state.go |
| `utils` | PTY handling, synchronization | console.go, sync.go |
| `hooks` | OCI lifecycle hooks | hooks.go |

---

## Security Features

### Input Validation

#### Container ID Validation

Container IDs must match: `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`

```bash
# These FAIL (security violation)
sudo runc-go create "../../../etc" /bundle    # Path traversal
sudo runc-go create "test;rm -rf /" /bundle   # Shell injection
sudo runc-go create "" /bundle                 # Empty ID

# These PASS
sudo runc-go create "myapp" /bundle
sudo runc-go create "my-app_v1.0" /bundle
sudo runc-go create "container123" /bundle
```

#### Path Traversal Protection

All paths are validated to stay within the container's rootfs:

```bash
# This mount will FAIL
{
  "mounts": [{
    "destination": "/../../../etc/passwd",
    "source": "/malicious",
    "type": "bind"
  }]
}
# Error: "path escapes base"
```

The `SecureJoin()` function:
1. Resolves all symlinks
2. Checks the result stays within base directory
3. Rejects any escapes

#### Device Whitelist

Only safe devices can be created:

**Allowed:**
- `/dev/null`, `/dev/zero`, `/dev/full`
- `/dev/random`, `/dev/urandom`
- `/dev/tty`, `/dev/console`, `/dev/ptmx`
- `/dev/pts/*` (pseudo-terminals)

**Blocked:**
- `/dev/sda`, `/dev/sdb` (block devices)
- `/dev/mem`, `/dev/kmem` (memory access)
- Anything not in whitelist

### Privilege Restriction

#### Linux Capabilities

Instead of full root, containers get limited capabilities:

| Capability | Allows |
|------------|--------|
| `CAP_AUDIT_WRITE` | Write audit logs |
| `CAP_CHOWN` | Change file ownership |
| `CAP_DAC_OVERRIDE` | Bypass file permissions |
| `CAP_FOWNER` | Bypass ownership checks |
| `CAP_FSETID` | Set setuid/setgid bits |
| `CAP_KILL` | Send signals |
| `CAP_MKNOD` | Create device nodes |
| `CAP_NET_BIND_SERVICE` | Bind ports < 1024 |
| `CAP_NET_RAW` | Raw sockets |
| `CAP_SETFCAP` | Set file capabilities |
| `CAP_SETGID` | Change GID |
| `CAP_SETPCAP` | Transfer capabilities |
| `CAP_SETUID` | Change UID |
| `CAP_SYS_CHROOT` | Use chroot() |

Capabilities NOT granted (container cannot):
- `CAP_SYS_ADMIN` - Mount filesystems, many admin operations
- `CAP_SYS_PTRACE` - Trace other processes
- `CAP_SYS_MODULE` - Load kernel modules
- `CAP_SYS_RAWIO` - Raw I/O port access

#### Seccomp Filtering

Syscall filtering using BPF:

```json
{
  "seccomp": {
    "defaultAction": "SCMP_ACT_ERRNO",
    "syscalls": [
      {
        "names": ["read", "write", "open", "close"],
        "action": "SCMP_ACT_ALLOW"
      }
    ]
  }
}
```

**Actions:**
| Action | Effect |
|--------|--------|
| `SCMP_ACT_ALLOW` | Allow syscall |
| `SCMP_ACT_ERRNO` | Return error (EPERM) |
| `SCMP_ACT_KILL` | Kill thread |
| `SCMP_ACT_KILL_PROCESS` | Kill process |
| `SCMP_ACT_TRAP` | Send SIGSYS |
| `SCMP_ACT_LOG` | Log and allow |

### File Permissions

| File | Mode | Purpose |
|------|------|---------|
| State files | 0600 | Prevent info disclosure |
| Log files | 0600 | Prevent info disclosure |
| Cgroup files | 0644 | Standard cgroup perms |

### Masked and Readonly Paths

Sensitive paths are protected:

**Masked (bind-mounted to /dev/null):**
- `/proc/acpi` - ACPI hardware info
- `/proc/kcore` - Physical memory
- `/proc/keys` - Kernel keys
- `/proc/timer_list` - Timer info
- `/sys/firmware` - Firmware data

**Readonly:**
- `/proc/bus` - Bus info
- `/proc/fs` - Filesystem info
- `/proc/irq` - IRQ info
- `/proc/sys` - Kernel params
- `/proc/sysrq-trigger` - Magic SysRq

---

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run with coverage report
make test-coverage
# Open coverage/coverage.html in browser

# Run with race detection
make test-race

# Run only unit tests (no root required)
make test-unit
```

### Integration Tests

These require root and a working Docker installation.

```bash
# Basic functionality
sudo docker run --runtime=runc-go --rm alpine echo "works!"

# Interactive shell
sudo docker run --runtime=runc-go --rm -it alpine sh

# Background container + exec
sudo docker run --runtime=runc-go -d --name test alpine sleep 300
sudo docker exec -it test sh
sudo docker rm -f test
```

### Security Tests

```bash
# Test path traversal protection
mkdir -p /tmp/testbundle/rootfs
cat > /tmp/testbundle/config.json << 'EOF'
{
  "ociVersion": "1.0.2",
  "root": {"path": "rootfs"},
  "mounts": [{"destination": "/../../../tmp/pwned", "source": "/etc", "type": "bind"}],
  "process": {"args": ["/bin/sh"], "cwd": "/"}
}
EOF
sudo runc-go create pathtest /tmp/testbundle
# Should fail with "path escapes base"

# Test container ID validation
sudo runc-go create "../../../etc" /tmp/testbundle
# Should fail with "invalid container ID"

# Test device whitelist
# Add to config.json:
# "linux": {"devices": [{"path": "/dev/sda", "type": "b", "major": 8, "minor": 0}]}
sudo runc-go create devtest /tmp/testbundle
# Should fail with "not in allowed list"
```

### Resource Limit Tests

```bash
# Memory limit (should show limited memory)
sudo docker run --runtime=runc-go --rm --memory=64m alpine free -m

# CPU limit (use stress tool)
sudo docker run --runtime=runc-go --rm --cpus=0.5 alpine sh -c 'cat /sys/fs/cgroup/cpu.max'

# PID limit
sudo docker run --runtime=runc-go --rm --pids-limit=10 alpine sh -c 'cat /sys/fs/cgroup/pids.max'
```

### Network Tests

```bash
# Ping test
sudo docker run --runtime=runc-go --rm alpine ping -c 3 8.8.8.8

# DNS resolution
sudo docker run --runtime=runc-go --rm alpine nslookup google.com

# Port binding
sudo docker run --runtime=runc-go --rm -p 8080:80 nginx &
curl http://localhost:8080
```

---

## Troubleshooting

### Common Issues

#### "permission denied"

**Problem:** Running runc-go without root privileges.

**Solution:** Use `sudo` or configure rootless containers (not yet supported).

```bash
sudo runc-go run myapp /bundle
```

#### "container not found"

**Problem:** Container doesn't exist or already deleted.

**Solution:** Check container list:
```bash
sudo runc-go list
```

#### "container is not running"

**Problem:** Trying to exec into a stopped container.

**Solution:** Start the container first or check its state:
```bash
sudo runc-go state myapp
sudo runc-go start myapp
```

#### "nsenter: failed to execute"

**Problem:** `nsenter` not installed (required for exec).

**Solution:** Install util-linux:
```bash
# Debian/Ubuntu
sudo apt install util-linux

# RHEL/CentOS
sudo yum install util-linux
```

#### "cgroup: permission denied"

**Problem:** Cgroups v2 not properly configured or not running as root.

**Solution:**
```bash
# Check cgroups version
cat /sys/fs/cgroup/cgroup.controllers

# If empty, enable controllers
echo "+cpu +memory +pids" | sudo tee /sys/fs/cgroup/cgroup.subtree_control
```

#### "Docker: Unknown runtime"

**Problem:** Docker not configured to use runc-go.

**Solution:**
```bash
# Check Docker config
cat /etc/docker/daemon.json

# Should contain:
# "runtimes": {"runc-go": {"path": "/usr/local/bin/runc-go"}}

# Restart Docker after editing
sudo systemctl restart docker
```

#### Container hangs on start

**Problem:** Init process blocked, never received start signal.

**Solution:**
```bash
# Check if FIFO exists
ls -la /run/runc-go/<container-id>/

# Try force delete and recreate
sudo runc-go delete -f <container-id>
```

### Debug Logging

```bash
# Enable logging
sudo runc-go --log=/tmp/runc.log --log-format=json run myapp /bundle

# View logs
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

# Check namespace (get PID first)
sudo ls -la /proc/<PID>/ns/
```

---

## Project Structure

```
runc-go/
│
├── main.go                 # CLI entry point
│                           # - Parses global flags (--root, --log)
│                           # - Routes commands to handlers
│                           # - 571 lines
│
├── container/              # Container lifecycle management
│   ├── container.go        # Container struct, state, validation
│   ├── create.go           # Create operation, fork child, setup
│   ├── start.go            # Start operation, unblock init
│   ├── exec.go             # Exec into running container
│   ├── kill.go             # Send signals to container
│   ├── delete.go           # Remove container and cleanup
│   ├── state.go            # Query container state
│   ├── syscalls.go         # Low-level syscall wrappers
│   └── container_test.go   # Unit tests
│
├── linux/                  # Linux-specific isolation
│   ├── namespace.go        # Namespace creation/joining
│   │                       # - Clone flags (CLONE_NEWPID, etc)
│   │                       # - UID/GID mappings
│   │
│   ├── rootfs.go           # Root filesystem setup
│   │                       # - pivot_root
│   │                       # - Mount handling
│   │                       # - SecureJoin (path validation)
│   │
│   ├── cgroup.go           # Cgroups v2 resource limits
│   │                       # - Memory, CPU, PIDs
│   │                       # - Controller enablement
│   │
│   ├── capabilities.go     # Linux capability management
│   │                       # - Drop/keep capabilities
│   │                       # - Bounding set
│   │
│   ├── seccomp.go          # Seccomp BPF filtering
│   │                       # - Syscall filtering
│   │                       # - Architecture validation
│   │
│   ├── devices.go          # Device node management
│   │                       # - Device whitelist
│   │                       # - Mknod operations
│   │
│   ├── namespace_test.go   # Namespace tests
│   └── cgroup_test.go      # Cgroup tests
│
├── spec/                   # OCI specification types
│   ├── spec.go             # config.json structures
│   │                       # - Spec, Process, Mount, Linux
│   │                       # - DefaultSpec() generation
│   │
│   ├── state.go            # Container state types
│   ├── spec_test.go        # Spec parsing tests
│   └── state_test.go       # State tests
│
├── utils/                  # Utility functions
│   ├── console.go          # PTY/terminal handling
│   │                       # - CreatePTY, SendToSocket
│   │                       # - Raw mode, signal handling
│   │
│   └── sync.go             # Process synchronization
│                           # - Pipes, FIFOs
│
├── hooks/                  # OCI lifecycle hooks
│   └── hooks.go            # Hook execution framework
│
├── DOCS/                   # Documentation
│   ├── testing.md          # Testing guide
│   ├── MissingFeatures.md  # Comparison with runc
│   └── cheatsheet.txt      # Quick reference
│
├── FLOW.md                 # Detailed execution flow
├── Makefile                # Build automation
├── go.mod                  # Go module definition
└── go.sum                  # Dependency checksums
```

### Lines of Code by Package

| Package | Lines | Description |
|---------|-------|-------------|
| main | ~570 | CLI and command routing |
| container | ~1,800 | Lifecycle management |
| linux | ~2,500 | Isolation primitives |
| spec | ~800 | OCI types |
| utils | ~400 | PTY and sync |
| hooks | ~150 | Hook execution |
| **Total** | ~6,200 | |

---

## Differences from Production runc

| Aspect | runc-go (Educational) | runc (Production) |
|--------|----------------------|-------------------|
| **Purpose** | Learning | Production use |
| **Code size** | ~6,200 lines | ~50,000+ lines |
| **Cgroup support** | v2 only | v1 and v2 |
| **Seccomp** | Basic BPF | Full libseccomp |
| **Checkpoint/restore** | No | Yes (CRIU) |
| **systemd integration** | No | Yes |
| **AppArmor** | No | Yes |
| **SELinux** | No | Yes |
| **Rootless mode** | Partial | Full |
| **User namespaces** | Basic | Full (newuidmap) |
| **Error handling** | Basic | Comprehensive |
| **Performance** | Good | Highly optimized |
| **Stability** | Educational | Battle-tested |

### Missing Features

See [DOCS/MissingFeatures.md](DOCS/MissingFeatures.md) for a complete list.

**Critical for Production:**
- Checkpoint/Restore (CRIU)
- systemd cgroup driver
- AppArmor/SELinux support
- Full seccomp with argument filtering
- Rootless networking (slirp4netns)

---

## Learning Resources

### Recommended Reading Order

1. **Start here:** `main.go` - See how commands are parsed and dispatched
2. **Container creation:** `container/create.go` - The heart of container setup
3. **Namespaces:** `linux/namespace.go` - How isolation is achieved
4. **Filesystem:** `linux/rootfs.go` - pivot_root and mount handling
5. **Execution:** `container/exec.go` - How exec enters containers
6. **Security:** `linux/capabilities.go`, `linux/seccomp.go` - Privilege restriction

### Key Concepts to Understand

1. **fork/exec model**: How Linux creates processes
2. **Clone flags**: CLONE_NEWPID, CLONE_NEWNS, etc.
3. **pivot_root**: Changing the root filesystem
4. **Cgroups v2**: Resource limiting via filesystem interface
5. **Linux capabilities**: Fine-grained root privileges
6. **Seccomp-BPF**: Syscall filtering with Berkeley Packet Filter

### External Resources

- [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec)
- [Linux Namespaces (man7)](https://man7.org/linux/man-pages/man7/namespaces.7.html)
- [Cgroups v2 (kernel.org)](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)
- [Linux Capabilities (man7)](https://man7.org/linux/man-pages/man7/capabilities.7.html)
- [Seccomp (kernel.org)](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)

### Related Projects

- [runc](https://github.com/opencontainers/runc) - Reference OCI runtime
- [containerd](https://github.com/containerd/containerd) - Industry-standard container runtime
- [crun](https://github.com/containers/crun) - Fast OCI runtime in C
- [youki](https://github.com/containers/youki) - OCI runtime in Rust

---

## FAQ

### General Questions

**Q: Is this production-ready?**

A: No. This is an educational implementation. Use the official `runc` for production.

**Q: What's the difference between Docker and runc-go?**

A: Docker is a full container platform (images, networking, orchestration). runc-go is just the runtime that executes containers. Docker uses runc (or runc-go) internally.

**Q: Why doesn't it work on macOS/Windows?**

A: Containers use Linux-specific features (namespaces, cgroups). macOS/Windows Docker uses a Linux VM to run containers.

**Q: Can I run without root?**

A: Partially. Some operations require root. Full rootless support requires user namespace setup with newuidmap/newgidmap, which isn't fully implemented.

### Technical Questions

**Q: Why separate create and start?**

A: The split allows orchestrators (Docker, Kubernetes) to set up networking and other resources between creation and execution.

**Q: Why does exec use nsenter?**

A: `nsenter` is a standard tool to enter existing namespaces. It's simpler and more reliable than implementing namespace joining from scratch.

**Q: Why cgroups v2 only?**

A: Cgroups v2 is the modern unified hierarchy. Supporting both v1 and v2 significantly increases complexity. Most modern systems use v2.

**Q: Why are some seccomp syscalls skipped?**

A: The implementation uses a built-in syscall table. If a config references syscalls not in the table, they're logged and skipped. Production runc uses libseccomp which has the full syscall table.

### Troubleshooting Questions

**Q: My container immediately exits**

A: Check the command in config.json. The container exits when its main process exits. For a shell, use `terminal: true` and run interactively.

**Q: Ctrl+C doesn't work**

A: Make sure you're using `-t` or `terminal: true`. Signal forwarding requires a PTY.

**Q: Network doesn't work**

A: Container networking is set up by Docker/containerd, not the runtime. In standalone mode, you need to configure networking separately (veth pairs, bridges, etc.).

---

## License

Educational use. Based on concepts from the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec).

---

## Contributing

This is an educational project. Contributions that improve clarity, fix bugs, or add educational value are welcome.

When contributing:
1. Keep the code readable and well-commented
2. Maintain the educational focus
3. Add tests for new functionality
4. Update documentation as needed
