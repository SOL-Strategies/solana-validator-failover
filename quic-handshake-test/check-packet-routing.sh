#!/bin/bash
# Check packet routing and interface selection

SERVER_IP="${1:-100.71.189.42}"
PORT=9898

echo "=== Checking Packet Routing ==="
echo "Server: $SERVER_IP:$PORT"
echo ""

echo "=== 1. Route to server ==="
ip route get "$SERVER_IP" 2>/dev/null

echo ""
echo "=== 2. Source IP that would be used ==="
SOURCE_IP=$(ip route get "$SERVER_IP" 2>/dev/null | grep -oP 'src \K\S+' || echo "unknown")
echo "Source IP: $SOURCE_IP"

echo ""
echo "=== 3. Interface that would be used ==="
INTERFACE=$(ip route get "$SERVER_IP" 2>/dev/null | grep -oP 'dev \K\S+' || echo "unknown")
echo "Interface: $INTERFACE"

echo ""
echo "=== 4. Tailscale interface info ==="
if ip link show tailscale0 >/dev/null 2>&1; then
    echo "tailscale0 exists:"
    ip addr show tailscale0 | grep -E "inet|inet6" | head -2
    echo ""
    echo "Is source IP on tailscale0?"
    if ip addr show tailscale0 | grep -q "$SOURCE_IP"; then
        echo "YES - Source IP is on tailscale0"
    else
        echo "NO - Source IP is NOT on tailscale0"
        echo "This might be the problem!"
    fi
else
    echo "tailscale0 interface not found"
fi

echo ""
echo "=== 5. Test UDP send (check which interface is used) ==="
echo "Sending test UDP packet..."
timeout 1 bash -c "echo 'test' > /dev/udp/$SERVER_IP/$PORT" 2>&1 || echo "UDP test sent (may timeout)"

echo ""
echo "=== 6. Check if packets are being sent from correct interface ==="
echo "Run this on the server to see which interface receives packets:"
echo "  sudo tcpdump -i any -n 'udp port $PORT'"

