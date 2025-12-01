#!/bin/bash
# Simple UDP packet receiver to test connectivity

PORT="${1:-9898}"

echo "Listening for UDP packets on port $PORT..."
echo "Press Ctrl+C to stop"
echo ""

# Method 1: Using netcat (nc)
if command -v nc >/dev/null 2>&1; then
    echo "Using netcat..."
    nc -u -l -p "$PORT"
    exit 0
fi

# Method 2: Using socat (if available)
if command -v socat >/dev/null 2>&1; then
    echo "Using socat..."
    socat UDP-RECV:$PORT -
    exit 0
fi

# Method 3: Using Python (if available)
if command -v python3 >/dev/null 2>&1; then
    echo "Using Python..."
    python3 << EOF
import socket
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.bind(("0.0.0.0", $PORT))
print(f"Listening on port $PORT...")
while True:
    data, addr = sock.recvfrom(1024)
    print(f"Received from {addr}: {data.decode()}")
EOF
    exit 0
fi

echo "ERROR: No UDP listening tool found (nc, socat, or python3)"
exit 1

