# runc-go

An educational OCI-compliant container runtime written in Go.

## What is This?

`runc-go` is a container runtime that implements the [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec). It's designed to teach how containers actually work under the hood.

**Think of it this way:**
- Docker is the car dealership (finds and manages containers)
- `runc-go` is the engine (actually runs them)

When you run `docker run alpine`, Docker downloads the image, unpacks it, creates a config, then calls a runtime like `runc-go` to actually run the container.

## Features

- Full OCI runtime specification compliance
- Works as a Docker runtime
- Linux namespace isolation (PID, mount, UTS, IPC, network, cgroup)
- Cgroups v2 resource limits (memory, CPU, PIDs)
- Interactive shell support with PTY
- Exec into running containers
- Signal forwarding (Ctrl+C works!)
- Security hardening (path validation, input sanitization)

## Quick Start

### Build & Install

```bash
make build
sudo cp runc-go /usr/local/bin/
```

### Use with Docker

```bash
# Configure Docker (one-time)
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

# Run containers
sudo docker run --runtime=runc-go --rm alpine echo "Hello!"
sudo docker run --runtime=runc-go --rm -it alpine
```

### Use Standalone

```bash
# Create a bundle
mkdir -p /tmp/bundle/rootfs
docker export $(docker create alpine) | tar -xf - -C /tmp/bundle/rootfs
runc-go spec > /tmp/bundle/config.json

# Run it
sudo runc-go run mycontainer /tmp/bundle
```

## Commands

| Command | Description |
|---------|-------------|
| `run` | Create and start a container |
| `create` | Create a container (paused) |
| `start` | Start a created container |
| `exec` | Run a command in a running container |
| `state` | Show container state as JSON |
| `list` | List all containers |
| `kill` | Send signal to container |
| `delete` | Remove a container |
| `spec` | Generate default config.json |

### Examples

```bash
# Run interactive shell
sudo runc-go run -t myapp /tmp/bundle

# Two-step create/start
sudo runc-go create myapp /tmp/bundle
sudo runc-go start myapp

# Exec into running container
sudo runc-go exec -t myapp /bin/sh

# Stop and remove
sudo runc-go kill myapp SIGTERM
sudo runc-go delete myapp
```

## How Containers Work

A container is just a Linux process with extra isolation:

```
+------------------------------------------+
|              HOST SYSTEM                 |
|                                          |
|  +----------------+  +----------------+  |
|  |  Container A   |  |  Container B   |  |
|  |  PID 1: /bin/sh|  |  PID 1: nginx  |  |
|  |  hostname: app |  |  hostname: web |  |
|  |  rootfs: /tmp/a|  |  rootfs: /tmp/b|  |
|  +----------------+  +----------------+  |
|                                          |
|  Host sees these as PID 5001 and 5050    |
+------------------------------------------+
```

**Isolation mechanisms:**

| Feature | What It Does |
|---------|--------------|
| PID Namespace | Container sees itself as PID 1 |
| Mount Namespace | Container has its own filesystem |
| Network Namespace | Container has its own network stack |
| UTS Namespace | Container has its own hostname |
| Cgroups | Limits CPU, memory, PIDs |
| Capabilities | Restricts root powers |
| Seccomp | Filters syscalls |

## Project Structure

```
runc-go/
├── main.go              # CLI entry point
├── container/           # Container lifecycle
│   ├── container.go     # State management
│   ├── create.go        # Create operation
│   ├── start.go         # Start operation
│   ├── exec.go          # Exec into containers
│   ├── kill.go          # Signal handling
│   └── delete.go        # Cleanup
├── linux/               # Linux primitives
│   ├── namespace.go     # Namespace setup
│   ├── cgroup.go        # Resource limits
│   ├── rootfs.go        # Filesystem isolation
│   ├── capabilities.go  # Capability management
│   ├── seccomp.go       # Syscall filtering
│   └── devices.go       # Device nodes
├── spec/                # OCI spec types
│   ├── spec.go          # config.json schema
│   └── state.go         # Container state
├── utils/               # Utilities
│   ├── sync.go          # Process synchronization
│   └── console.go       # PTY handling
└── hooks/               # Lifecycle hooks
    └── hooks.go
```

## Container Lifecycle

```
        create              start           exit/kill
(empty) -----> [created] -------> [running] --------> [stopped]
                                                          |
                                                     delete
                                                          |
                                                          v
                                                      (empty)
```

## Security

This educational runtime includes security measures:

- **Path traversal protection** - Prevents `../` escapes in mounts and devices
- **Container ID validation** - Rejects malicious container names
- **Shell injection prevention** - Properly quotes shell commands
- **Device whitelist** - Only allows safe device nodes
- **Architecture-portable syscalls** - Works on x86_64 and ARM64
- **Restrictive file permissions** - State files use 0600

## Testing

See [testing.md](testing.md) for complete testing instructions.

```bash
# Run unit tests
make test

# Quick Docker test
sudo docker run --runtime=runc-go --rm -it alpine
```

## Differences from Production runc

| Aspect | runc-go | runc |
|--------|---------|------|
| Purpose | Learning | Production |
| Code size | ~6,000 lines | ~50,000+ lines |
| Checkpoint/restore | No | Yes |
| systemd integration | No | Yes |
| AppArmor/SELinux | No | Yes |
| Error handling | Basic | Comprehensive |

## Requirements

- Linux (containers use Linux-specific features)
- Go 1.21+
- Root access (for namespace operations)
- Docker (optional, for easy testing)

## Building

```bash
# Build
make build

# Test
make test

# Install
sudo make install

# All targets
make help
```

## Learning Resources

To understand how this works, read the source in this order:

1. `main.go` - See how commands are dispatched
2. `container/create.go` - See how containers are created
3. `linux/namespace.go` - See how isolation works
4. `linux/rootfs.go` - See how filesystems are set up
5. `container/exec.go` - See how exec works

## License

Educational use. Based on concepts from the OCI Runtime Specification.
