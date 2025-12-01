#!/bin/bash
# Check if server is receiving packets on tailscale0 interface

PORT=9898
INTERFACE="${1:-tailscale0}"

echo "Checking if packets arrive on $INTERFACE interface..."
echo "Run the client, then check this output"
echo ""

sudo tcpdump -i "$INTERFACE" -n -c 10 "udp port $PORT" 2>&1

