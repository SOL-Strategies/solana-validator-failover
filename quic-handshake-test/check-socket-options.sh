#!/bin/bash
# Check UDP socket options and network features that might differ

echo "=== Kernel Version ==="
uname -r
echo ""

echo "=== UDP Socket Options (checking what quic-go might use) ==="
# Check if we can see socket options
echo "Checking UDP socket features..."
echo ""

echo "=== Network Interface Features ==="
for iface in $(ip link show | grep -E "^[0-9]+:" | awk -F: '{print $2}' | tr -d ' '); do
    if [ "$iface" != "lo" ]; then
        echo "Interface: $iface"
        if command -v ethtool >/dev/null 2>&1; then
            sudo ethtool -k $iface 2>/dev/null | grep -E "udp|gro|gso|tso" || echo "  (ethtool not available or no relevant features)"
        fi
        echo ""
    fi
done

echo "=== IP Forwarding ==="
sysctl net.ipv4.ip_forward net.ipv6.conf.all.forwarding 2>/dev/null
echo ""

echo "=== ECN (Explicit Congestion Notification) ==="
sysctl net.ipv4.tcp_ecn net.ipv4.ip_no_pmtu_disc 2>/dev/null
echo ""

echo "=== UDP-specific settings ==="
sysctl net.ipv4.udp_early_demux net.ipv4.udp_l3mdev_accept 2>/dev/null
echo ""

echo "=== Check if running through Tailscale ==="
if ip link show tailscale0 >/dev/null 2>&1; then
    echo "Tailscale interface found"
    echo "Tailscale MTU: $(ip link show tailscale0 | grep mtu | awk '{print $5}')"
    echo "Check Tailscale ACLs: tailscale status"
else
    echo "No Tailscale interface"
fi

