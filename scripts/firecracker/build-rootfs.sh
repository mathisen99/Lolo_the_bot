#!/bin/bash
# Build Python rootfs for Firecracker
# Creates an ext4 filesystem with Python and required libraries

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOTFS_SIZE="2G"  # Size of the rootfs image (2GB needed for Python + libs)
ROOTFS_NAME="python-rootfs.ext4"

echo "=== Building Python Rootfs ==="

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "ERROR: Docker is required to build the rootfs"
    exit 1
fi

# Create a temporary directory for building
BUILD_DIR=$(mktemp -d)
trap "rm -rf $BUILD_DIR" EXIT

echo "Build directory: $BUILD_DIR"

# Create Dockerfile for the rootfs
cat > "$BUILD_DIR/Dockerfile" << 'EOF'
FROM ubuntu:24.04

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install Python and system dependencies
RUN apt-get update && apt-get install -y \
    python3.12 \
    python3.12-venv \
    python3-pip \
    graphviz \
    fonts-liberation \
    fontconfig \
    openssh-server \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Create symlinks
RUN ln -sf /usr/bin/python3.12 /usr/bin/python3 && \
    ln -sf /usr/bin/python3 /usr/bin/python

# Install Python libraries
RUN pip3 install --break-system-packages \
    matplotlib \
    numpy \
    pandas \
    Pillow \
    graphviz \
    diagrams \
    mermaid-py \
    seaborn \
    scipy \
    sympy \
    networkx \
    plotly \
    openpyxl \
    xlsxwriter \
    requests-mock

# Create output directory
RUN mkdir -p /tmp/output && chmod 777 /tmp/output

# Create vsock listener Python script
RUN cat > /usr/local/bin/vsock-python-server.py << 'PYSERVER'
#!/usr/bin/env python3
"""
Vsock server that receives Python code, executes it, and returns results.

Protocol:
- Receive: JSON with "code" field
- Send: JSON with "stdout", "stderr", "exitcode", "files" fields
"""

import socket
import json
import subprocess
import os
import base64
import tempfile
import sys

VSOCK_PORT = 5000
TIMEOUT = 180  # 3 minutes

def execute_code(code):
    """Execute Python code and return results."""
    # Create temp directory for output files
    work_dir = tempfile.mkdtemp(prefix="pyexec_")
    
    # Write code to file
    code_file = os.path.join(work_dir, "code.py")
    with open(code_file, "w") as f:
        f.write(code)
    
    # Execute with timeout
    try:
        result = subprocess.run(
            ["python3", code_file],
            capture_output=True,
            text=True,
            timeout=TIMEOUT,
            cwd=work_dir
        )
        stdout = result.stdout
        stderr = result.stderr
        exitcode = result.returncode
    except subprocess.TimeoutExpired:
        stdout = ""
        stderr = f"Execution timed out after {TIMEOUT} seconds"
        exitcode = 124
    except Exception as e:
        stdout = ""
        stderr = str(e)
        exitcode = 1
    
    # Collect output files (base64 encoded)
    files = {}
    for filename in os.listdir(work_dir):
        if filename == "code.py":
            continue
        filepath = os.path.join(work_dir, filename)
        if os.path.isfile(filepath):
            with open(filepath, "rb") as f:
                files[filename] = base64.b64encode(f.read()).decode("ascii")
    
    # Cleanup
    import shutil
    shutil.rmtree(work_dir, ignore_errors=True)
    
    return {
        "stdout": stdout,
        "stderr": stderr,
        "exitcode": exitcode,
        "files": files
    }

def main():
    # Create vsock socket
    # AF_VSOCK = 40, SOCK_STREAM = 1
    sock = socket.socket(40, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    
    # VMADDR_CID_ANY = -1 (0xFFFFFFFF)
    # Bind to any CID on our port
    sock.bind((socket.VMADDR_CID_ANY, VSOCK_PORT))
    sock.listen(5)
    
    print(f"Vsock Python server listening on port {VSOCK_PORT}", flush=True)
    
    while True:
        try:
            conn, addr = sock.accept()
            print(f"Connection from CID {addr}", flush=True)
            
            # Receive data (up to 10MB)
            data = b""
            while True:
                chunk = conn.recv(65536)
                if not chunk:
                    break
                data += chunk
                # Check for end marker
                if b"\n\n" in data:
                    break
            
            # Parse JSON
            try:
                request = json.loads(data.decode("utf-8").strip())
                code = request.get("code", "")
                
                # Execute
                result = execute_code(code)
                
                # Send response
                response = json.dumps(result) + "\n\n"
                conn.sendall(response.encode("utf-8"))
                
            except json.JSONDecodeError as e:
                error_response = json.dumps({
                    "stdout": "",
                    "stderr": f"Invalid JSON: {e}",
                    "exitcode": 1,
                    "files": {}
                }) + "\n\n"
                conn.sendall(error_response.encode("utf-8"))
            
            conn.close()
            
        except Exception as e:
            print(f"Error: {e}", flush=True)

if __name__ == "__main__":
    main()
PYSERVER

RUN chmod +x /usr/local/bin/vsock-python-server.py

# Create systemd service for vsock server
RUN cat > /etc/systemd/system/vsock-python.service << 'SERVICE'
[Unit]
Description=Vsock Python Execution Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/python3 /usr/local/bin/vsock-python-server.py
Restart=always
RestartSec=1

[Install]
WantedBy=multi-user.target
SERVICE

RUN systemctl enable vsock-python.service

# Clean up
RUN apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
EOF

echo "Building Docker image..."
docker build -t firecracker-python-rootfs "$BUILD_DIR"

echo "Exporting filesystem..."
CONTAINER_ID=$(docker create firecracker-python-rootfs)
docker export "$CONTAINER_ID" > "$BUILD_DIR/rootfs.tar"
docker rm "$CONTAINER_ID"

echo "Creating ext4 image..."
# Create empty image
truncate -s "$ROOTFS_SIZE" "$BUILD_DIR/$ROOTFS_NAME"

# Format as ext4
mkfs.ext4 -F "$BUILD_DIR/$ROOTFS_NAME"

# Mount and extract
MOUNT_DIR=$(mktemp -d)
sudo mount -o loop "$BUILD_DIR/$ROOTFS_NAME" "$MOUNT_DIR"
sudo tar -xf "$BUILD_DIR/rootfs.tar" -C "$MOUNT_DIR"

# Generate SSH key for the VM
if [ ! -f "$SCRIPT_DIR/vm_key" ]; then
    ssh-keygen -t ed25519 -f "$SCRIPT_DIR/vm_key" -N "" -C "firecracker-vm"
    echo "Generated SSH key: $SCRIPT_DIR/vm_key"
fi

# Copy SSH public key to rootfs
sudo mkdir -p "$MOUNT_DIR/root/.ssh"
sudo cp "$SCRIPT_DIR/vm_key.pub" "$MOUNT_DIR/root/.ssh/authorized_keys"
sudo chmod 600 "$MOUNT_DIR/root/.ssh/authorized_keys"

# Unmount
sudo umount "$MOUNT_DIR"
rmdir "$MOUNT_DIR"

# Move to final location
mv "$BUILD_DIR/$ROOTFS_NAME" "$SCRIPT_DIR/$ROOTFS_NAME"

echo ""
echo "=== Rootfs Build Complete ==="
echo "Rootfs: $SCRIPT_DIR/$ROOTFS_NAME"
echo "SSH Key: $SCRIPT_DIR/vm_key"
echo ""
echo "Installed Python packages:"
echo "  - matplotlib, numpy, pandas, Pillow"
echo "  - graphviz, diagrams, mermaid-py"
echo "  - seaborn, scipy, sympy, networkx, plotly"
