# GitHub Issue Template for quic-go

## Title
quic-go 0.44.0+ hangs during DialAddr initialization on Ubuntu 24 servers (kernel 6.8.0)

## Description

quic-go versions 0.44.0 and later fail to establish QUIC connections on Ubuntu 24 servers (kernel 6.8.0), while:
- ✅ v0.43.1 works perfectly on the same servers
- ✅ v0.57.1 works on local Ubuntu 24 (kernel 6.16.3)
- ❌ v0.44.0+ hangs during `DialAddr()` initialization, no packets sent

## Environment

**Servers (failing):**
- OS: Ubuntu 24.04.3 LTS
- Kernel: 6.8.0-88-generic
- Go: 1.25.4
- Network: Tailscale mesh + doublezero0 interface
- quic-go: v0.57.1 (also tested v0.44.0, v0.49.1 - all fail)

**Local (working):**
- OS: Ubuntu 24.04.3 LTS  
- Kernel: 6.16.3-76061603-generic
- Go: 1.25.4
- Network: Tailscale only
- quic-go: v0.57.1 works

## Symptoms

1. `quic.DialAddr()` is called
2. Function hangs for exactly the context timeout duration (30s)
3. Returns "context deadline exceeded"
4. **No UDP packets are sent** (verified with tcpdump)
5. Fails before UDP socket creation

## Debug Output

```
[DEBUG] Step 1: Resolving address...
[DEBUG] Resolved to: 100.71.189.42:9898
[DEBUG] Step 2: Creating context with 30s timeout...
[DEBUG] Step 3: Creating quic.Config...
[DEBUG] Step 4: Creating TLS config...
[DEBUG] Step 5: Calling quic.DialAddr (this is where it hangs/fails)...
[DEBUG] Starting dial at 23:40:41.133
[DEBUG] ERROR at 23:41:11.137: context deadline exceeded
```

## Test Programs

Minimal reproduction case available in: `quic-handshake-test/` directory
- `server.go` - QUIC server listening on port 9898
- `client-debug.go` - Client with debug logging showing where it hangs

## What We've Ruled Out

- ❌ Firewall (0.43.1 works on same servers)
- ❌ Kernel TLS module (unloading didn't help)
- ❌ UDP buffer sizes
- ❌ Connection method (both DialAddr and Dial fail)
- ❌ TLS/crypto libraries (same versions)
- ❌ Go version (both 1.25.4)

## Related Issues

- [Issue #5331](https://github.com/quic-go/quic-go/issues/5331) - Similar tunnel interface MTU problems (packets sent but not received)
- [Tailscale issue #2633](https://github.com/tailscale/tailscale/issues/2633) - QUIC/H3 fails over Tailscale due to MTU

## Testing MTU Hypothesis

We're testing if setting `InitialPacketSize: 1200` in `quic.Config` helps work around Tailscale's 1280 byte MTU (based on [issue #5331 comment](https://github.com/quic-go/quic-go/issues/5331#issuecomment-3313524914)). Test programs available in `quic-handshake-test/` directory.

## Hypothesis

**MTU Discovery on Tunnel Interfaces** (most likely based on [issue #5331](https://github.com/quic-go/quic-go/issues/5331)):

The servers use **Tailscale** (tunnel interface with 1280 byte MTU), while QUIC requires minimum 1350 bytes (IPv6) or 1370 bytes (IPv4). quic-go 0.44.0+ may perform MTU discovery during `DialAddr()` initialization, which could hang or fail on tunnel interfaces with insufficient MTU.

**Other possibilities:**
- Network interface detection/enumeration changes in 0.44.0+
- Socket option setting that behaves differently on kernel 6.8.0
- Some system call that blocks on tunnel interfaces

## Request

Please investigate why quic-go 0.44.0+ hangs during `DialAddr()` initialization on Ubuntu 24 with kernel 6.8.0, while v0.43.1 works fine on the same environment.

