#!/bin/bash
# Check Kernel TLS configuration

echo "=== Kernel TLS Modules ==="
lsmod | grep -E "tls|crypto" | head -10
echo ""

echo "=== Check if kTLS is enabled on interfaces ==="
for iface in $(ip link show | grep -E "^[0-9]+:" | awk -F: '{print $2}' | tr -d ' '); do
    if [ "$iface" != "lo" ] && [ -d "/sys/class/net/$iface" ]; then
        echo "Interface: $iface"
        if command -v ethtool >/dev/null 2>&1; then
            sudo ethtool -k $iface 2>/dev/null | grep -i tls || echo "  (no TLS offload info)"
        fi
        echo ""
    fi
done

echo "=== Kernel TLS sysctl settings ==="
sysctl -a 2>/dev/null | grep -i tls | head -10 || echo "No TLS sysctl settings found"
echo ""

echo "=== Check /proc/sys/net for TLS settings ==="
find /proc/sys/net -name "*tls*" 2>/dev/null | head -10 || echo "No TLS proc settings found"
echo ""

