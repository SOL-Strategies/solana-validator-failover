#!/bin/bash
# Debug network connectivity between client and server

SERVER_IP="${1:-100.71.189.42}"
PORT=9898

echo "=== Network Debugging ==="
echo "Server IP: $SERVER_IP"
echo "Port: $PORT"
echo ""

echo "=== 1. Check if server IP is reachable ==="
ping -c 2 "$SERVER_IP" 2>&1 | head -5

echo ""
echo "=== 2. Check UDP connectivity ==="
echo "Testing UDP port $PORT..."
timeout 2 bash -c "echo 'test' > /dev/udp/$SERVER_IP/$PORT" 2>&1 || echo "UDP test completed (may timeout, that's OK)"

echo ""
echo "=== 3. Check routing to server ==="
ip route get "$SERVER_IP" 2>/dev/null || echo "Could not determine route"

echo ""
echo "=== 4. Check firewall rules ==="
if command -v iptables >/dev/null 2>&1; then
    echo "INPUT chain rules:"
    sudo iptables -L INPUT -n -v | grep -E "9898|UDP" | head -5 || echo "No specific rules for port 9898"
fi

echo ""
echo "=== 5. Check if port is listening ==="
echo "On server, run: sudo netstat -ulnp | grep 9898"
echo "Or: sudo ss -ulnp | grep 9898"

echo ""
echo "=== 6. Test with nc (netcat) ==="
echo "On server, run: nc -u -l 9898"
echo "Then on client, run: echo 'test' | nc -u $SERVER_IP $PORT"

