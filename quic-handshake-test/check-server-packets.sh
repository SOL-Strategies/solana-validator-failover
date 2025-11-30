#!/bin/bash
# Check if server is receiving packets

PORT=9898
INTERFACE="${1:-tailscale0}"

echo "Capturing packets on $INTERFACE for port $PORT..."
echo "Run the client in another terminal, then check this output"
echo "Press Ctrl+C to stop"
echo ""

sudo tcpdump -i "$INTERFACE" -n -v -X "udp port $PORT" 2>&1 | grep -E "(listening|UDP|length|^[0-9])"

