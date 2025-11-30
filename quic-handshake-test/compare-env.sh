#!/bin/bash
# Script to compare network environment between local and server
# Run this on both local Ubuntu 24 and server Ubuntu 24

echo "=== System Information ==="
echo "Hostname: $(hostname)"
echo "Kernel: $(uname -r)"
echo "OS: $(lsb_release -d 2>/dev/null | cut -f2 || cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2)"
echo ""

echo "=== UDP Buffer Sizes ==="
sysctl net.core.rmem_max net.core.wmem_max net.core.rmem_default net.core.wmem_default 2>/dev/null
sysctl net.ipv4.udp_mem 2>/dev/null
echo ""

echo "=== Network Interfaces ==="
ip -4 addr show | grep -E "^[0-9]+:|inet " | head -20
echo ""

echo "=== MTU Settings ==="
ip link show | grep -E "^[0-9]+:|mtu"
echo ""

echo "=== Firewall Status ==="
if command -v ufw >/dev/null 2>&1; then
    echo "UFW: $(sudo ufw status 2>/dev/null | head -1)"
fi
if command -v iptables >/dev/null 2>&1; then
    echo "iptables rules count: $(sudo iptables -L -n 2>/dev/null | grep -c "^[A-Z]")"
fi
if command -v nft >/dev/null 2>&1; then
    echo "nftables: $(sudo nft list ruleset 2>/dev/null | wc -l) lines"
fi
echo ""

echo "=== Container/Virtualization ==="
if command -v systemd-detect-virt >/dev/null 2>&1; then
    echo "Virtualization: $(systemd-detect-virt 2>/dev/null || echo 'none')"
fi
if [ -f /proc/1/cgroup ]; then
    echo "Cgroup info: $(cat /proc/1/cgroup | head -1)"
fi
echo ""

echo "=== Network Namespaces ==="
ip netns list 2>/dev/null || echo "No network namespaces"
echo ""

echo "=== Systemd-resolved ==="
systemctl is-active systemd-resolved 2>/dev/null || echo "systemd-resolved not active"
echo ""

echo "=== Security Modules ==="
if command -v getenforce >/dev/null 2>&1; then
    echo "SELinux: $(getenforce 2>/dev/null)"
fi
if command -v aa-status >/dev/null 2>&1; then
    echo "AppArmor: $(sudo aa-status 2>/dev/null | head -1)"
fi
echo ""

echo "=== Go Version ==="
go version 2>/dev/null || echo "Go not found"
echo ""

echo "=== Network Routes ==="
ip route show | head -10
echo ""

echo "=== DNS Configuration ==="
cat /etc/resolv.conf 2>/dev/null | head -5
echo ""

echo "=== UDP Socket Statistics ==="
ss -u -a 2>/dev/null | grep -E "9898|State" | head -5 || netstat -u -a 2>/dev/null | grep -E "9898|State" | head -5
echo ""

