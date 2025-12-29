#!/bin/bash
# Start Firecracker microVM for Python execution
# NO NETWORK - completely isolated for security

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
API_SOCKET="${SCRIPT_DIR}/firecracker.sock"
LOGFILE="${SCRIPT_DIR}/firecracker.log"

# VM Configuration
VM_MEMORY_MB=2048
VM_VCPUS=8

# Find kernel and rootfs
KERNEL="${SCRIPT_DIR}/vmlinux-latest"
ROOTFS="${SCRIPT_DIR}/python-rootfs.ext4"

# Verify files exist
if [ ! -f "$KERNEL" ]; then
    echo "ERROR: Kernel not found at $KERNEL"
    echo "Run ./setup.sh first"
    exit 1
fi

if [ ! -f "$ROOTFS" ]; then
    echo "ERROR: Rootfs not found at $ROOTFS"
    echo "Run ./build-rootfs.sh first"
    exit 1
fi

if [ ! -f "${SCRIPT_DIR}/firecracker" ]; then
    echo "ERROR: Firecracker binary not found"
    echo "Run ./setup.sh first"
    exit 1
fi

# Clean up old socket
rm -f "$API_SOCKET"

echo "=== Starting Firecracker microVM ==="
echo "Memory: ${VM_MEMORY_MB}MB"
echo "vCPUs: ${VM_VCPUS}"
echo "Network: DISABLED (isolated)"
echo "API Socket: $API_SOCKET"

# Start Firecracker in background
"${SCRIPT_DIR}/firecracker" --api-sock "$API_SOCKET" &
FC_PID=$!
echo "Firecracker PID: $FC_PID"

# Wait for socket to be available
sleep 0.5

# Configure logging
curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"log_path\": \"${LOGFILE}\",
        \"level\": \"Warning\",
        \"show_level\": true,
        \"show_log_origin\": true
    }" \
    "http://localhost/logger"

# Set boot source (console on serial for debugging)
KERNEL_BOOT_ARGS="console=ttyS0 reboot=k panic=1 pci=off"

curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"kernel_image_path\": \"${KERNEL}\",
        \"boot_args\": \"${KERNEL_BOOT_ARGS}\"
    }" \
    "http://localhost/boot-source"

# Set rootfs
curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"drive_id\": \"rootfs\",
        \"path_on_host\": \"${ROOTFS}\",
        \"is_root_device\": true,
        \"is_read_only\": false
    }" \
    "http://localhost/drives/rootfs"

# Set shared drive for code/output exchange
SHARED_DRIVE="${SCRIPT_DIR}/shared.ext4"
if [ -f "$SHARED_DRIVE" ]; then
    curl -s -X PUT --unix-socket "$API_SOCKET" \
        --data "{
            \"drive_id\": \"shared\",
            \"path_on_host\": \"${SHARED_DRIVE}\",
            \"is_root_device\": false,
            \"is_read_only\": false
        }" \
        "http://localhost/drives/shared"
    echo "Shared drive: $SHARED_DRIVE"
else
    echo "WARNING: Shared drive not found. Run ./create-shared-drive.sh first"
fi

# Set machine config
curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"vcpu_count\": ${VM_VCPUS},
        \"mem_size_mib\": ${VM_MEMORY_MB}
    }" \
    "http://localhost/machine-config"

# NO NETWORK INTERFACE - This is intentional for security
# The VM cannot access the internet or host services

# Configure vsock for host-guest communication
VSOCK_PATH="${SCRIPT_DIR}/vsock.sock"
rm -f "$VSOCK_PATH"
curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"guest_cid\": 3,
        \"uds_path\": \"${VSOCK_PATH}\"
    }" \
    "http://localhost/vsock"

# Make vsock socket accessible (needed for non-root bot process)
sleep 0.5
chmod 666 "$VSOCK_PATH" 2>/dev/null || true

echo "Vsock: $VSOCK_PATH (CID 3)"

# Start the VM
curl -s -X PUT --unix-socket "$API_SOCKET" \
    --data "{
        \"action_type\": \"InstanceStart\"
    }" \
    "http://localhost/actions"

echo "✓ VM Started"
echo ""
echo "PID file: ${SCRIPT_DIR}/firecracker.pid"
echo $FC_PID > "${SCRIPT_DIR}/firecracker.pid"

# Keep running
wait $FC_PID
