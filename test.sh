#!/bin/bash
# Test script for runc-go
# Run with: sudo ./test.sh

set -e

RUNC_GO="$(dirname "$0")/runc-go"
BUNDLE="/tmp/test-bundle"

echo "=== Testing runc-go ==="
echo

# Show version
echo "1. Version:"
$RUNC_GO version
echo

# Generate spec
echo "2. Generate spec:"
$RUNC_GO spec | head -20
echo "..."
echo

# Ensure bundle exists
if [ ! -d "$BUNDLE/rootfs/bin" ]; then
    echo "Creating test bundle..."
    mkdir -p "$BUNDLE/rootfs"
    docker export $(docker create alpine:latest) | tar -xf - -C "$BUNDLE/rootfs"
fi

# Create config.json for testing
cat > "$BUNDLE/config.json" << 'EOF'
{
  "ociVersion": "1.0.2",
  "process": {
    "terminal": false,
    "user": { "uid": 0, "gid": 0 },
    "args": ["/bin/sh", "-c", "echo 'Hello from runc-go!' && hostname && uname -a"],
    "env": ["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"],
    "cwd": "/",
    "noNewPrivileges": true
  },
  "root": { "path": "rootfs" },
  "hostname": "runc-go-test",
  "mounts": [
    { "destination": "/proc", "type": "proc", "source": "proc" },
    { "destination": "/dev", "type": "tmpfs", "source": "tmpfs", "options": ["nosuid", "mode=755"] },
    { "destination": "/sys", "type": "sysfs", "source": "sysfs", "options": ["nosuid", "noexec", "nodev", "ro"] }
  ],
  "linux": {
    "namespaces": [
      { "type": "pid" },
      { "type": "mount" },
      { "type": "uts" },
      { "type": "ipc" }
    ]
  }
}
EOF

echo "3. Test create + start + delete:"
echo "   Creating container..."
$RUNC_GO create test-container "$BUNDLE"
echo "   Created."

echo "   Container state:"
$RUNC_GO state test-container
echo

echo "   Starting container..."
$RUNC_GO start test-container
echo "   Started."

sleep 1

echo "   Container state after start:"
$RUNC_GO state test-container 2>/dev/null || echo "   (container exited)"
echo

echo "   Deleting container..."
$RUNC_GO delete test-container --force 2>/dev/null || true
echo "   Deleted."
echo

echo "4. Test run (create+start combined):"
$RUNC_GO run test-run "$BUNDLE" || true
$RUNC_GO delete test-run --force 2>/dev/null || true
echo

echo "5. List containers:"
$RUNC_GO list
echo

echo "=== All tests completed ==="
