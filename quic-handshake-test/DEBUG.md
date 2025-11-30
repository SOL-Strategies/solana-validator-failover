# Debugging quic-go Handshake Issues

## Observation

- ✅ Works locally on Ubuntu 24
- ❌ Fails on Ubuntu 24 servers

This suggests an **environment-specific issue**, not a general Ubuntu 24 incompatibility.

## Things to Check

### 1. Kernel Version
```bash
uname -r
```

### 2. Network Stack Configuration
```bash
# Check UDP buffer sizes
sysctl net.core.rmem_max net.core.wmem_max net.core.rmem_default net.core.wmem_default

# Check UDP receive buffer
sysctl net.ipv4.udp_mem

# Check if GRO/GSO is enabled
ethtool -k <interface> | grep -E "gro|gso|tso|ufo"
```

### 3. Firewall/iptables Rules
```bash
# Check iptables rules
sudo iptables -L -n -v
sudo ip6tables -L -n -v

# Check nftables
sudo nft list ruleset
```

### 4. Network Interface Configuration
```bash
# Check MTU
ip link show

# Check interface features
ethtool <interface>
```

### 5. Systemd-resolved / DNS
```bash
# Check if systemd-resolved is interfering
systemctl status systemd-resolved
```

### 6. Network Namespaces
```bash
# Check if running in a network namespace
ip netns list
```

### 7. Containerization
```bash
# Check if running in container
systemd-detect-virt
cat /proc/1/cgroup
```

### 8. SELinux/AppArmor
```bash
# Check security modules
getenforce 2>/dev/null || echo "SELinux not installed"
aa-status 2>/dev/null || echo "AppArmor not installed"
```

### 9. UDP Packet Capture
```bash
# On server, capture UDP packets on port 9898
sudo tcpdump -i any -n udp port 9898 -v

# On client, capture outgoing packets
sudo tcpdump -i any -n udp port 9898 -v
```

### 10. Compare Network Configurations
Compare these between local and server:
- `/etc/sysctl.conf`
- `/etc/sysctl.d/*.conf`
- Network manager configs
- Firewall rules

## Quick Comparison Script

Run `./compare-env.sh` on both local and server to compare environments:

```bash
# On local machine
cd quic-handshake-test
./compare-env.sh > local-env.txt

# On server
cd quic-handshake-test
./compare-env.sh > server-env.txt

# Compare
diff local-env.txt server-env.txt
```

This will help identify what's different between the working local environment and the failing server environment.

