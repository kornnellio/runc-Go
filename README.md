# runc-go

An OCI-compliant container runtime written in Go for educational purposes.

---

## Table of Contents

1. [What is This?](#what-is-this)
2. [Prerequisites](#prerequisites)
3. [Building](#building)
4. [Quick Start](#quick-start)
5. [Commands Reference](#commands-reference)
6. [Understanding OCI Containers](#understanding-oci-containers)
7. [Creating a Container Bundle](#creating-a-container-bundle)
8. [The config.json File](#the-configjson-file)
9. [Container Lifecycle](#container-lifecycle)
10. [Using with Docker](#using-with-docker)
11. [Troubleshooting](#troubleshooting)
12. [Project Structure](#project-structure)
13. [How It Works](#how-it-works)
14. [Differences from Production runc](#differences-from-production-runc)

---

## What is This?

`runc-go` is a **container runtime** that implements the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec).

### In Simple Terms:

- **Docker** is like a car dealership - it helps you find, buy, and manage cars
- **runc-go** is like the engine - it's what actually makes the car run

When you run `docker run alpine`, Docker:
1. Downloads the Alpine image
2. Unpacks it to a directory
3. Creates a `config.json` with settings
4. Calls a **runtime** (like `runc` or `runc-go`) to actually run the container

This project IS that runtime. It takes a directory with files and a config, and runs it as an isolated container.

---

## Prerequisites

### Required

| Requirement | Why |
|-------------|-----|
| **Linux** | Containers use Linux-specific features (namespaces, cgroups) |
| **Go 1.21+** | To compile the source code |
| **Root access (sudo)** | Container isolation requires root privileges |

### Optional

| Tool | Why |
|------|-----|
| **Docker** | Easiest way to create container filesystems |

### Check Your System

```bash
# Check Linux
uname -s
# Should output: Linux

# Check Go
go version
# Should output: go version go1.21+ ...

# Check sudo
sudo whoami
# Should output: root
```

---

## Building

### Step 1: Navigate to the Project

```bash
cd /home/me/Desktop/Go_Linux/runc-go
```

### Step 2: Build the Binary

```bash
go build -o runc-go .
```

### Step 3: Verify the Build

```bash
./runc-go version
```

**Expected output:**
```
runc-go version 0.1.0
spec: 1.0.2
```

### Optional: Install System-Wide

```bash
sudo cp runc-go /usr/local/bin/
```

---

## Quick Start

This section gets you running a container in under 2 minutes.

### Step 1: Create a Test Directory

```bash
mkdir -p /tmp/my-container/rootfs
cd /tmp/my-container
```

### Step 2: Get a Filesystem (Using Docker)

```bash
# Pull Alpine Linux (tiny Linux distro, ~5MB)
docker pull alpine:latest

# Export it to our rootfs directory
docker export $(docker create alpine:latest) | tar -xf - -C rootfs
```

**What just happened?**
- Docker downloaded Alpine Linux
- We extracted all its files to `rootfs/`
- This directory now contains a complete Linux filesystem

### Step 3: Generate a Config File

```bash
/home/me/Desktop/Go_Linux/runc-go/runc-go spec > config.json
```

### Step 4: Run the Container

```bash
sudo /home/me/Desktop/Go_Linux/runc-go/runc-go run my-first-container .
```

**You should see a shell prompt inside the container!**

```
/ #
```

### Step 5: Explore Inside the Container

```bash
# Check the hostname
hostname

# Check the process list (only shows container processes)
ps aux

# Check the filesystem
ls /

# Exit the container
exit
```

---

## Commands Reference

### Overview

| Command | Description |
|---------|-------------|
| `create` | Create a container (but don't start it) |
| `start` | Start a created container |
| `run` | Create and start in one step |
| `state` | Show container status as JSON |
| `list` | List all containers |
| `kill` | Send a signal to a container |
| `delete` | Remove a container |
| `spec` | Generate a default config.json |

### Detailed Usage

#### `runc-go create`

Creates a container but does NOT start the main process. The container waits until you call `start`.

```bash
sudo runc-go create <container-id> <bundle-path>
```

**Example:**
```bash
sudo runc-go create myapp /tmp/my-container
```

**Options:**
- `--pid-file <path>` - Write the container PID to a file
- `--root <path>` - Use a different state directory (default: `/run/runc-go`)

---

#### `runc-go start`

Starts a container that was previously created.

```bash
sudo runc-go start <container-id>
```

**Example:**
```bash
sudo runc-go start myapp
```

---

#### `runc-go run`

Creates AND starts a container in one command. This is what you'll use most often.

```bash
sudo runc-go run <container-id> <bundle-path>
```

**Example:**
```bash
sudo runc-go run myapp /tmp/my-container
```

---

#### `runc-go state`

Shows the current state of a container as JSON.

```bash
sudo runc-go state <container-id>
```

**Example output:**
```json
{
  "ociVersion": "1.0.2",
  "id": "myapp",
  "status": "running",
  "pid": 12345,
  "bundle": "/tmp/my-container"
}
```

**Possible statuses:**
- `creating` - Container is being set up
- `created` - Container exists but process hasn't started
- `running` - Container process is running
- `stopped` - Container process has exited

---

#### `runc-go list`

Lists all containers.

```bash
sudo runc-go list
```

**Example output:**
```
ID        PID    STATUS   BUNDLE              CREATED
myapp     12345  running  /tmp/my-container   2024-01-15 10:30:00
webapp    12400  stopped  /tmp/webapp         2024-01-15 09:15:00
```

---

#### `runc-go kill`

Sends a signal to the container's main process.

```bash
sudo runc-go kill <container-id> [signal]
```

**Examples:**
```bash
# Graceful shutdown (SIGTERM)
sudo runc-go kill myapp SIGTERM

# Force kill (SIGKILL)
sudo runc-go kill myapp SIGKILL

# Using signal numbers
sudo runc-go kill myapp 9
```

**Common signals:**
| Signal | Number | Effect |
|--------|--------|--------|
| SIGTERM | 15 | Graceful shutdown (process can cleanup) |
| SIGKILL | 9 | Immediate termination (cannot be caught) |
| SIGHUP | 1 | Hangup (often used to reload config) |

---

#### `runc-go delete`

Removes a container. The container must be stopped first (unless you use `--force`).

```bash
sudo runc-go delete <container-id>
```

**Options:**
- `--force` or `-f` - Kill the container if it's still running

**Examples:**
```bash
# Delete a stopped container
sudo runc-go delete myapp

# Force delete a running container
sudo runc-go delete myapp --force
```

---

#### `runc-go spec`

Generates a default `config.json` file.

```bash
runc-go spec > config.json
```

**Options:**
- `--rootless` - Generate config for rootless (unprivileged) containers

---

## Understanding OCI Containers

### What is OCI?

OCI stands for **Open Container Initiative**. It's a standard that defines:

1. **How container images are formatted** (layers, manifests)
2. **How containers are run** (this is what runc-go implements)

### What is a Container?

A container is just a **regular Linux process** with extra isolation:

| Isolation | What It Does |
|-----------|--------------|
| **PID Namespace** | Container sees itself as PID 1, can't see host processes |
| **Mount Namespace** | Container has its own filesystem view |
| **Network Namespace** | Container has its own network stack |
| **UTS Namespace** | Container has its own hostname |
| **IPC Namespace** | Container has its own shared memory |
| **User Namespace** | Container can have its own user IDs (rootless) |
| **Cgroups** | Limits CPU, memory, and other resources |

### Visual Representation

```
┌─────────────────────────────────────────────────────────────┐
│                         HOST SYSTEM                          │
│                                                              │
│  ┌─────────────────────┐    ┌─────────────────────┐         │
│  │     Container A     │    │     Container B     │         │
│  │  ┌───────────────┐  │    │  ┌───────────────┐  │         │
│  │  │ PID 1 (bash)  │  │    │  │ PID 1 (nginx) │  │         │
│  │  │ PID 2 (app)   │  │    │  │ PID 2 (worker)│  │         │
│  │  └───────────────┘  │    │  └───────────────┘  │         │
│  │  Hostname: app-srv  │    │  Hostname: web-srv  │         │
│  │  IP: 10.0.0.2       │    │  IP: 10.0.0.3       │         │
│  │  Rootfs: /tmp/appA  │    │  Rootfs: /tmp/appB  │         │
│  └─────────────────────┘    └─────────────────────┘         │
│                                                              │
│  Host sees: PID 5001 (container A), PID 5050 (container B)  │
└─────────────────────────────────────────────────────────────┘
```

---

## Creating a Container Bundle

A **bundle** is a directory containing everything needed to run a container:

```
my-container/           <-- Bundle directory
├── config.json         <-- OCI configuration file
└── rootfs/             <-- Root filesystem
    ├── bin/
    ├── etc/
    ├── lib/
    ├── usr/
    └── ...
```

### Method 1: From Docker Image (Recommended)

```bash
# Create bundle directory
mkdir -p my-container/rootfs
cd my-container

# Export any Docker image
docker export $(docker create nginx:alpine) | tar -xf - -C rootfs

# Generate config
runc-go spec > config.json
```

### Method 2: From Scratch (Advanced)

```bash
mkdir -p my-container/rootfs
cd my-container

# Create minimal filesystem
mkdir -p rootfs/{bin,lib,lib64,etc,proc,sys,dev}

# Copy a static binary (busybox has everything)
cp /path/to/busybox-static rootfs/bin/sh

# Generate config
runc-go spec > config.json
```

### Method 3: From Alpine Mini Root Filesystem

```bash
mkdir -p my-container/rootfs
cd my-container

# Download Alpine mini root filesystem
wget https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.0-x86_64.tar.gz

# Extract it
tar -xzf alpine-minirootfs-3.19.0-x86_64.tar.gz -C rootfs

# Generate config
runc-go spec > config.json
```

---

## The config.json File

The `config.json` file tells the runtime how to run your container. Here's a complete example with explanations:

```json
{
  "ociVersion": "1.0.2",

  "process": {
    "terminal": true,
    "user": {
      "uid": 0,
      "gid": 0
    },
    "args": [
      "/bin/sh"
    ],
    "env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      "TERM=xterm",
      "HOME=/root"
    ],
    "cwd": "/",
    "noNewPrivileges": true
  },

  "root": {
    "path": "rootfs",
    "readonly": false
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
      "options": ["nosuid", "mode=755"]
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
      { "type": "mount" },
      { "type": "uts" },
      { "type": "ipc" },
      { "type": "network" }
    ],
    "resources": {
      "memory": {
        "limit": 536870912
      },
      "cpu": {
        "quota": 50000,
        "period": 100000
      },
      "pids": {
        "limit": 100
      }
    }
  }
}
```

### Key Sections Explained

#### `process` - What to Run

| Field | Description | Example |
|-------|-------------|---------|
| `terminal` | Attach a terminal (for interactive use) | `true` |
| `user.uid` | User ID inside container | `0` (root) |
| `user.gid` | Group ID inside container | `0` (root) |
| `args` | Command to run | `["/bin/sh", "-c", "echo hello"]` |
| `env` | Environment variables | `["PATH=/bin", "HOME=/root"]` |
| `cwd` | Working directory | `"/"` |

#### `root` - Filesystem

| Field | Description | Example |
|-------|-------------|---------|
| `path` | Path to rootfs (relative to bundle) | `"rootfs"` |
| `readonly` | Mount rootfs as read-only | `false` |

#### `linux.namespaces` - Isolation

| Namespace | What It Isolates |
|-----------|------------------|
| `pid` | Process IDs |
| `mount` | Filesystem mounts |
| `uts` | Hostname |
| `ipc` | Inter-process communication |
| `network` | Network interfaces |
| `user` | User/group IDs (for rootless) |

#### `linux.resources` - Limits

| Resource | What It Limits | Example |
|----------|----------------|---------|
| `memory.limit` | RAM in bytes | `536870912` (512MB) |
| `cpu.quota/period` | CPU time (quota/period = fraction) | `50000/100000` = 50% |
| `pids.limit` | Maximum processes | `100` |

---

## Container Lifecycle

### State Diagram

```
                    ┌──────────────┐
                    │   (empty)    │
                    └──────┬───────┘
                           │ create
                           ▼
                    ┌──────────────┐
                    │   created    │ ◄─── Container exists, process waiting
                    └──────┬───────┘
                           │ start
                           ▼
                    ┌──────────────┐
                    │   running    │ ◄─── Process is executing
                    └──────┬───────┘
                           │ (process exits or kill)
                           ▼
                    ┌──────────────┐
                    │   stopped    │ ◄─── Process has exited
                    └──────┬───────┘
                           │ delete
                           ▼
                    ┌──────────────┐
                    │   (empty)    │
                    └──────────────┘
```

### Example Workflow

```bash
# 1. Create container (stays in "created" state)
sudo runc-go create myapp /tmp/my-container
sudo runc-go state myapp  # status: "created"

# 2. Start container (moves to "running" state)
sudo runc-go start myapp
sudo runc-go state myapp  # status: "running"

# 3. Container runs... then exits (moves to "stopped" state)
sudo runc-go state myapp  # status: "stopped"

# 4. Delete container (removes all state)
sudo runc-go delete myapp
sudo runc-go state myapp  # error: container not found
```

### Why Separate Create and Start?

The two-step process allows:

1. **Hooks** - Run scripts between create and start
2. **Network setup** - Configure networking before process runs
3. **Inspection** - Check container setup before committing
4. **Orchestration** - Kubernetes/Docker control when containers start

For simple use cases, just use `run` which does both steps.

---

## Using with Docker

You can configure Docker to use `runc-go` as an alternative runtime. **This has been tested and works!**

### Step 1: Configure Docker

Edit or create `/etc/docker/daemon.json`:

```json
{
  "runtimes": {
    "runc-go": {
      "path": "/full/path/to/runc-go"
    }
  }
}
```

> **Note:** Use the full absolute path to the `runc-go` binary.

### Step 2: Restart Docker

```bash
sudo systemctl daemon-reload
sudo systemctl restart docker
```

### Step 3: Verify Registration

```bash
docker info | grep -A5 Runtimes
# Should show: Runtimes: io.containerd.runc.v2 runc runc-go
```

### Step 4: Run Containers

```bash
# Basic test
docker run --rm --runtime=runc-go alpine echo "Hello from runc-go!"

# With environment variables
docker run --rm --runtime=runc-go -e MY_VAR="test" alpine sh -c 'echo $MY_VAR'

# With volume mounts
docker run --rm --runtime=runc-go -v /tmp:/mnt alpine ls /mnt

# Multi-command test
docker run --rm --runtime=runc-go alpine sh -c "hostname && whoami && uname -a"
```

### Expected Output

You'll see some warnings followed by your command output:

```
[rootfs] warning: mount .../sys/fs/cgroup (cgroup): operation not permitted
[rootfs] warning: mask /proc/kcore: no such file or directory
Hello from runc-go!
```

**These warnings are non-fatal** - they occur because:
- `cgroup` mount requires special privileges that containerd already handles
- Some `/proc` paths don't exist in all kernel configurations

The container runs correctly despite these warnings.

### What Works with Docker

| Feature | Status |
|---------|--------|
| Basic containers | ✅ Works |
| Environment variables | ✅ Works |
| Volume mounts | ✅ Works |
| Custom hostname | ✅ Works |
| User specification | ✅ Works |
| Working directory | ✅ Works |
| Resource limits | ✅ Works |
| Network (bridge) | ✅ Works (Docker handles networking) |

### Step 5: Make it Default (Optional)

```json
{
  "default-runtime": "runc-go",
  "runtimes": {
    "runc-go": {
      "path": "/full/path/to/runc-go"
    }
  }
}
```

> **Warning:** Only set as default after thorough testing!

---

## Troubleshooting

### Common Errors and Solutions

#### "permission denied"

**Problem:** You're not running as root.

**Solution:**
```bash
sudo runc-go run myapp /tmp/my-container
```

#### "container already exists"

**Problem:** A container with this ID already exists.

**Solution:**
```bash
# Delete the old container
sudo runc-go delete myapp --force

# Then create the new one
sudo runc-go run myapp /tmp/my-container
```

#### "no such file or directory: config.json"

**Problem:** The bundle directory doesn't have a config.json.

**Solution:**
```bash
cd /path/to/bundle
runc-go spec > config.json
```

#### "no such file or directory: rootfs"

**Problem:** The rootfs directory doesn't exist or is empty.

**Solution:**
```bash
mkdir -p /path/to/bundle/rootfs
docker export $(docker create alpine) | tar -xf - -C /path/to/bundle/rootfs
```

#### "executable file not found"

**Problem:** The command in config.json doesn't exist in the rootfs.

**Solution:**
Check that the binary exists:
```bash
ls /path/to/bundle/rootfs/bin/sh
```

#### "operation not permitted" during pivot_root

**Problem:** Usually a namespace or mount issue.

**Solution:**
Make sure you're running as root and the system supports namespaces:
```bash
# Check namespace support
ls /proc/self/ns/

# Should show: cgroup, ipc, mnt, net, pid, user, uts
```

### Debugging Tips

#### Check Container Logs

The container's stdout/stderr go to your terminal. For background containers:
```bash
sudo runc-go run myapp /tmp/bundle > /tmp/container.log 2>&1 &
cat /tmp/container.log
```

#### Check Container State

```bash
sudo runc-go state myapp
```

#### Check if Process is Running

```bash
# Get PID from state
sudo runc-go state myapp | grep pid

# Check process
ps aux | grep <pid>
```

#### Check Cgroup

```bash
ls /sys/fs/cgroup/runc-go/myapp/
cat /sys/fs/cgroup/runc-go/myapp/cgroup.procs
```

---

## Project Structure

```
runc-go/
├── main.go                    # CLI entry point and command routing
├── go.mod                     # Go module definition
├── README.md                  # This file
│
├── spec/                      # OCI Specification types
│   ├── spec.go                # config.json schema (all struct definitions)
│   └── state.go               # Container state types
│
├── container/                 # Container lifecycle management
│   ├── container.go           # Container struct and state management
│   ├── create.go              # Create operation (fork, setup, wait)
│   ├── start.go               # Start operation (signal init to exec)
│   ├── state.go               # State query operation
│   ├── kill.go                # Signal handling
│   ├── delete.go              # Cleanup and removal
│   └── syscalls.go            # Low-level syscall wrappers
│
├── linux/                     # Linux-specific isolation primitives
│   ├── namespace.go           # Namespace creation and joining
│   ├── cgroup.go              # Cgroup v2 resource limits
│   ├── rootfs.go              # Filesystem setup (pivot_root, mounts)
│   ├── capabilities.go        # Linux capabilities management
│   ├── seccomp.go             # Seccomp BPF syscall filtering
│   └── devices.go             # Device node creation
│
├── hooks/                     # OCI lifecycle hooks
│   └── hooks.go               # Hook execution (prestart, poststart, etc.)
│
└── utils/                     # Utility functions
    ├── sync.go                # Parent-child synchronization (FIFO, pipes)
    └── console.go             # PTY/terminal handling
```

### File Descriptions

| File | Lines | Purpose |
|------|-------|---------|
| `main.go` | ~300 | CLI parsing, command dispatch |
| `spec/spec.go` | ~600 | Complete OCI config.json schema |
| `spec/state.go` | ~80 | Container state structures |
| `container/container.go` | ~200 | Container management, state persistence |
| `container/create.go` | ~300 | The create operation - forks, sets up namespaces |
| `container/start.go` | ~80 | Signals init process to exec user command |
| `linux/namespace.go` | ~150 | Namespace flags, setns, ID mappings |
| `linux/cgroup.go` | ~200 | Memory, CPU, PID limits via cgroups v2 |
| `linux/rootfs.go` | ~350 | pivot_root, mount setup, path masking |
| `linux/capabilities.go` | ~250 | Drop/keep Linux capabilities |
| `linux/seccomp.go` | ~400 | BPF filter generation and installation |

---

## How It Works

### The Create/Start Dance

When you run `runc-go run myapp /bundle`:

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. PARENT PROCESS (runc-go run)                                 │
│    - Reads config.json                                          │
│    - Creates FIFO for synchronization                           │
│    - Creates cgroup                                             │
│    - Forks child with CLONE_NEW* flags                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ fork() with namespace flags
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. CHILD PROCESS (runc-go init) - Now in NEW namespaces        │
│    - Opens FIFO (before pivot_root!)                            │
│    - Sets hostname                                              │
│    - Calls pivot_root (changes root filesystem)                 │
│    - Mounts /proc, /dev, /sys                                   │
│    - Sets up devices                                            │
│    - BLOCKS reading from FIFO... waiting for start              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ (parent writes to FIFO)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ 3. CHILD CONTINUES                                              │
│    - Drops capabilities                                         │
│    - Installs seccomp filter                                    │
│    - Sets user/group IDs                                        │
│    - exec() the user's command (e.g., /bin/sh)                  │
│    - Child process is REPLACED by user's command                │
└─────────────────────────────────────────────────────────────────┘
```

### Namespace Magic

When we call `fork()` with `CLONE_NEWPID | CLONE_NEWNS | ...`:

```
BEFORE FORK:
┌────────────────────────────────────────┐
│ Host Namespace                         │
│   PID 1: systemd                       │
│   PID 1000: runc-go                    │
└────────────────────────────────────────┘

AFTER FORK:
┌────────────────────────────────────────┐
│ Host Namespace                         │
│   PID 1: systemd                       │
│   PID 1000: runc-go (parent)           │
│   PID 1001: runc-go init ─────────────────┐
└────────────────────────────────────────┘  │
                                            │
┌────────────────────────────────────────┐  │
│ Container Namespace                    │◄─┘
│   PID 1: runc-go init                  │
│   (sees itself as PID 1!)              │
└────────────────────────────────────────┘
```

---

## Differences from Production runc

This is an **educational implementation**. Key differences from the real `runc`:

| Feature | runc-go | Production runc |
|---------|---------|-----------------|
| **Purpose** | Learning | Production use |
| **Code size** | ~4,500 lines | ~50,000+ lines |
| **Rootless containers** | Basic | Full support |
| **Seccomp** | Simple filter | Full libseccomp integration |
| **Checkpoint/Restore** | No | Yes (via CRIU) |
| **systemd integration** | No | Yes |
| **Console handling** | Basic | Full PTY support |
| **Error handling** | Minimal | Comprehensive |
| **Testing** | Manual | Extensive test suite |

### What's Missing?

- Full console socket protocol
- Checkpoint/restore (CRIU)
- systemd cgroup driver
- AppArmor/SELinux integration
- Rootless networking
- Windows support
- Comprehensive error messages

### What's Included?

- All core OCI lifecycle operations
- Linux namespace isolation
- Cgroups v2 resource limits
- Capability dropping
- Basic seccomp filtering
- Proper create/start separation

---

## Related Projects

| Project | Description |
|---------|-------------|
| [gocr](../gocr) | The simpler educational runtime this builds upon |
| [runc](https://github.com/opencontainers/runc) | The reference OCI runtime |
| [crun](https://github.com/containers/crun) | Fast OCI runtime in C |
| [youki](https://github.com/containers/youki) | OCI runtime in Rust |

---

## License

Educational use. Based on concepts from the OCI Runtime Specification and various open-source container runtimes.

---

## Quick Reference Card

```
┌─────────────────────────────────────────────────────────────────┐
│                     runc-go Quick Reference                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  SETUP BUNDLE:                                                   │
│    mkdir -p bundle/rootfs                                        │
│    docker export $(docker create alpine) | tar -xf - -C rootfs   │
│    runc-go spec > config.json                                    │
│                                                                  │
│  RUN CONTAINER:                                                  │
│    sudo runc-go run <name> <bundle>                              │
│                                                                  │
│  LIFECYCLE:                                                      │
│    sudo runc-go create <name> <bundle>  # Create (paused)        │
│    sudo runc-go start <name>            # Start                  │
│    sudo runc-go state <name>            # Check status           │
│    sudo runc-go kill <name> SIGTERM     # Stop gracefully        │
│    sudo runc-go delete <name>           # Remove                 │
│                                                                  │
│  LIST:                                                           │
│    sudo runc-go list                    # Show all containers    │
│                                                                  │
│  STATES: creating → created → running → stopped                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```
