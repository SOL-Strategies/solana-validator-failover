#!/bin/bash
# Simple UDP server test to verify UDP connectivity

PORT=9898

echo "Starting simple UDP server on port $PORT..."
echo "This will listen for UDP packets and print them"
echo "Press Ctrl+C to stop"
echo ""

# Use netcat or socat to listen for UDP packets
if command -v nc >/dev/null 2>&1; then
    echo "Using netcat..."
    nc -u -l -p "$PORT"
elif command -v socat >/dev/null 2>&1; then
    echo "Using socat..."
    socat UDP-RECV:$PORT -
else
    echo "ERROR: Neither netcat nor socat found"
    echo "Install one of them:"
    echo "  sudo apt-get install netcat-openbsd"
    echo "  or"
    echo "  sudo apt-get install socat"
    exit 1
fi

