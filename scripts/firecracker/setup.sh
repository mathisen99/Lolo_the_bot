#!/bin/bash
# Firecracker Setup Script
# Downloads Firecracker binary and kernel, prepares for rootfs creation

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FC_DIR="$SCRIPT_DIR"
ARCH="$(uname -m)"

echo "=== Firecracker Setup ==="
echo "Directory: $FC_DIR"
echo "Architecture: $ARCH"

# Check KVM access
if [ ! -e /dev/kvm ]; then
    echo "ERROR: /dev/kvm not found. KVM is required for Firecracker."
    exit 1
fi

if [ ! -r /dev/kvm ] || [ ! -w /dev/kvm ]; then
    echo "ERROR: No read/write access to /dev/kvm"
    echo "Run: sudo chmod 666 /dev/kvm"
    echo "Or add user to kvm group: sudo usermod -aG kvm $USER"
    exit 1
fi

echo "✓ KVM access verified"

# Download Firecracker binary
echo ""
echo "=== Downloading Firecracker ==="
RELEASE_URL="https://github.com/firecracker-microvm/firecracker/releases"
LATEST=$(basename $(curl -fsSLI -o /dev/null -w %{url_effective} ${RELEASE_URL}/latest))
echo "Latest version: $LATEST"

if [ -f "$FC_DIR/firecracker" ]; then
    echo "Firecracker binary already exists, skipping download"
else
    curl -L ${RELEASE_URL}/download/${LATEST}/firecracker-${LATEST}-${ARCH}.tgz | tar -xz -C "$FC_DIR"
    mv "$FC_DIR/release-${LATEST}-${ARCH}/firecracker-${LATEST}-${ARCH}" "$FC_DIR/firecracker"
    mv "$FC_DIR/release-${LATEST}-${ARCH}/jailer-${LATEST}-${ARCH}" "$FC_DIR/jailer" 2>/dev/null || true
    rm -rf "$FC_DIR/release-${LATEST}-${ARCH}"
    chmod +x "$FC_DIR/firecracker"
    echo "✓ Firecracker downloaded"
fi

# Download kernel
echo ""
echo "=== Downloading Kernel ==="
CI_VERSION="${LATEST%.*}"

if ls "$FC_DIR"/vmlinux-* 1>/dev/null 2>&1; then
    echo "Kernel already exists, skipping download"
else
    KERNEL_KEY=$(curl "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/$CI_VERSION/$ARCH/vmlinux-&list-type=2" \
        | grep -oP "(?<=<Key>)(firecracker-ci/$CI_VERSION/$ARCH/vmlinux-[0-9]+\.[0-9]+\.[0-9]{1,3})(?=</Key>)" \
        | sort -V | tail -1)
    
    if [ -z "$KERNEL_KEY" ]; then
        echo "ERROR: Could not find kernel for version $CI_VERSION"
        exit 1
    fi
    
    wget -q --show-progress -O "$FC_DIR/vmlinux-latest" "https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}"
    echo "✓ Kernel downloaded"
fi

echo ""
echo "=== Setup Complete ==="
echo "Firecracker: $FC_DIR/firecracker"
echo "Kernel: $FC_DIR/vmlinux-latest"
echo ""
echo "Next: Run ./build-rootfs.sh to create the Python rootfs"
