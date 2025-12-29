#!/bin/bash
# Stop the Firecracker VM

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="${SCRIPT_DIR}/firecracker.pid"

if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Stopping Firecracker (PID: $PID)..."
        kill "$PID"
        rm -f "$PID_FILE"
        rm -f "${SCRIPT_DIR}/firecracker.sock"
        echo "✓ VM Stopped"
    else
        echo "Process $PID not running"
        rm -f "$PID_FILE"
    fi
else
    echo "No PID file found"
    # Try to find and kill any running firecracker
    pkill -f "firecracker.*${SCRIPT_DIR}" 2>/dev/null && echo "Killed orphan process" || true
fi
