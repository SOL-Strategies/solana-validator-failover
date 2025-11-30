#!/bin/bash
# Check MTU settings on all interfaces, especially tunnel interfaces

echo "=== Interface MTU Settings ==="
ip link show | grep -E "^[0-9]+:|mtu" | grep -B1 mtu

echo ""
echo "=== Tailscale Interface Details ==="
if ip link show tailscale0 >/dev/null 2>&1; then
    echo "tailscale0:"
    ip link show tailscale0 | grep -E "mtu|state"
    ip addr show tailscale0 | grep -E "inet|inet6" | head -3
else
    echo "tailscale0 interface not found"
fi

echo ""
echo "=== All Tunnel Interfaces ==="
ip link show | grep -E "tun|tap|wg|tailscale|vpn" -i || echo "No tunnel interfaces found"

echo ""
echo "=== Route to Test Server ==="
# Replace with your test server IP
TEST_SERVER="100.71.189.42"
if command -v ip >/dev/null 2>&1; then
    ip route get "$TEST_SERVER" 2>/dev/null | head -1
    echo "Interface used: $(ip route get "$TEST_SERVER" 2>/dev/null | grep -oP 'dev \K\S+')"
fi

echo ""
echo "=== MTU Discovery Test ==="
echo "Testing path MTU to server..."
# Test with ping to discover path MTU
if ping -c 1 -M do -s 1472 "$TEST_SERVER" >/dev/null 2>&1; then
    echo "Path MTU appears to be >= 1500 bytes"
else
    echo "Path MTU may be < 1500 bytes"
fi

