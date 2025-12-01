#!/bin/bash
# Test if server receives packets on tailscale0 interface

PORT=9898
INTERFACE="${1:-tailscale0}"

echo "Capturing UDP packets on $INTERFACE, port $PORT..."
echo "Run the client, then check if packets appear here"
echo "Press Ctrl+C to stop"
echo ""

sudo tcpdump -i "$INTERFACE" -n "udp port $PORT"

