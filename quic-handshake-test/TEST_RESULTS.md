# Test Results: quic-go on Ubuntu 24

## Test Date
2025-12-01

## Test Environment
- OS: Ubuntu 24.04 LTS (fresh install)
- Kernel: 6.16.3-76061603-generic
- Go: 1.25.4
- Network: Tailscale mesh network (UDP)
- Servers: 
  - solana-testnet-pengu-london-latitude (server)
  - solana-testnet-pengu-chicago-latitude (client)

## Test Results

### quic-go v0.43.1 ✅
- **Status**: WORKS
- **Method**: `quic.DialAddr()` with `nil` config
- **Result**: Connection established successfully, handshake completes

### quic-go v0.57.1 ❌
- **Status**: FAILS
- **Method 1**: `quic.DialAddr()` with explicit config
  - **Result**: "context deadline exceeded" after 30 seconds
  - **Error**: Handshake never completes
  
- **Method 2**: `quic.Dial()` with explicit UDP connection
  - **Result**: Hangs at "Connecting to QUIC server..."
  - **Error**: Handshake never completes

## Conclusion

quic-go versions 0.44.0+ are incompatible with Ubuntu 24's network stack. The handshake process fails regardless of:
- Connection method (`DialAddr` vs `Dial`)
- Configuration (timeouts, keepalive, etc.)
- Network setup (tested on Tailscale mesh)

## Recommendation

**Stay on quic-go v0.43.1** until:
1. quic-go releases a fix for Ubuntu 24 compatibility
2. Ubuntu 24 network stack is updated to be compatible
3. An alternative workaround is found

## Next Actions

1. File issue with quic-go: https://github.com/quic-go/quic-go/issues
2. Monitor quic-go releases for Ubuntu 24 fixes
3. Re-test periodically with newer versions

