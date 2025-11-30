# quic-go 0.44.0+ Handshake Failure - Investigation Summary

## Problem

- ✅ quic-go v0.43.1 works on servers
- ❌ quic-go v0.44.0+ fails on servers (handshake timeout)
- ✅ quic-go v0.57.1 works locally (Ubuntu 24)
- ❌ quic-go v0.57.1 fails on servers (Ubuntu 24)

## What We've Tested

### ❌ Ruled Out:
1. **Firewall** - 0.43.1 works on same servers
2. **Kernel TLS module** - Unloading `tls` module didn't help
3. **UDP buffer sizes** - Servers have larger buffers (should be better)
4. **Connection method** - Both `DialAddr()` and `Dial()` fail
5. **TLS/crypto libraries** - Same OpenSSL versions
6. **Go version** - Both using 1.25.4

### ⚠️ Still Different:
1. **Kernel version**: 6.16.3 (local) vs 6.8.0 (servers)
2. **Network setup**: Servers have `doublezero0` interface + Tailscale
3. **iptables rules**: More rules on servers (84-89 vs 29)

## Next Steps

1. **Packet capture** - Use `capture-packets.sh` to see what's actually happening
2. **Check quic-go 0.44.0 changelog** - Find what actually changed
3. **Test kernel 6.16.x on servers** - Match local kernel version
4. **File issue with quic-go** - With all findings

## Current Status

**Staying on quic-go v0.43.1** until we find the root cause or quic-go fixes it.

