#!/bin/bash
# Capture UDP packets to see what's happening during handshake
 
if [ -z "$1" ]; then
    echo "Usage: $0 <interface>"
    echo "Example: $0 tailscale0"
    echo ""
    echo "Available interfaces:"
    ip link show | grep -E "^[0-9]+:" | awk -F: '{print $2}' | tr -d ' '
    exit 1
fi

INTERFACE=$1
PORT=9898

echo "Capturing UDP packets on interface $INTERFACE, port $PORT"
echo "Run the QUIC client in another terminal, then press Ctrl+C to stop"
echo ""

sudo tcpdump -i $INTERFACE -n -v -X udp port $PORT

