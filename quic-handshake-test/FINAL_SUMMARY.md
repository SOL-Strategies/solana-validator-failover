# Final Summary: quic-go v0.57.1 Testing

## Current Status

**Test programs are using quic-go v0.57.1** ✅

## Test Results with v0.57.1

### What Works:
- ✅ Client sends packets (with `InitialPacketSize: 1200`)
- ✅ Server binds correctly (using `Transport` with `udp4`)
- ✅ Code compiles and runs

### What Doesn't Work:
- ❌ Packets sent from client don't arrive at server's Tailscale interface
- ❌ Server never receives packets (verified with tcpdump on `tailscale0`)
- ❌ Handshake never completes

## Key Findings

1. **v0.43.1 works perfectly** - Sends 2504-byte packets that arrive
2. **v0.57.1 requires `InitialPacketSize: 1200`** - Without it, no packets sent
3. **v0.57.1 with `InitialPacketSize: 1200`** - Packets sent but don't arrive
4. **Routing issue** - Packets don't reach server's Tailscale interface

## Configuration That Sends Packets (but doesn't work)

**Client:**
```go
quicConfig := &quic.Config{
    InitialPacketSize:       1200, // Required to send packets
    DisablePathMTUDiscovery: true,
}
```

**Server:**
```go
udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: Port})
tr := quic.Transport{Conn: udpConn}
listener, err := tr.Listen(tlsConfig, quicConfig)

quicConfig := &quic.Config{
    InitialPacketSize:       1200, // Match client
    DisablePathMTUDiscovery: true,
}
```

## Conclusion

quic-go v0.57.1 has a fundamental incompatibility with Tailscale on Ubuntu 24 servers (kernel 6.8.0). The packets are sent but filtered/dropped before reaching the server, while v0.43.1's larger packets work fine.

**Recommendation:** Stay on v0.43.1 until quic-go fixes this issue.

