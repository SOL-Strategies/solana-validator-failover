# UDP Connectivity Issue

## Finding

Basic UDP (netcat) doesn't work between client and server, BUT quic-go v0.43.1 works fine.

## What This Means

This suggests:
1. **v0.43.1 does something different** that makes it work
2. **v0.44.0+ changed** to use the same approach as netcat (which doesn't work)
3. **Network/routing issue** exists, but v0.43.1 works around it

## Next Steps to Debug

### 1. Check if tcpdump sees the netcat packet

On server, run:
```bash
sudo tcpdump -i any -n -v "udp port 9898"
```

Then send from client:
```bash
echo "test" | nc -u -w 1 100.71.189.42 9898
```

**Question**: Does tcpdump see the packet?

- **YES** → Packets arrive but aren't delivered to socket (socket binding issue)
- **NO** → Packets don't arrive at all (routing/firewall issue, but v0.43.1 somehow works)

### 2. Compare how v0.43.1 vs v0.44.0+ sends packets

We need to understand what v0.43.1 does differently. Possible differences:
- Socket binding (source IP/interface)
- Socket options
- Packet structure/headers
- Connection establishment method

### 3. Test with v0.43.1 to see what it does

If possible, run the same test programs with v0.43.1 to see:
- What source IP/interface it uses
- What socket options it sets
- How it differs from v0.44.0+

## Hypothesis

v0.43.1 might:
- Use a different source interface/IP
- Set different socket options
- Use a different connection method that works with Tailscale routing

v0.44.0+ likely changed to use standard UDP socket behavior (like netcat), which doesn't work in this environment.

