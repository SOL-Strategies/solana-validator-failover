# Investigation: quic-go 0.44.0+ Handshake Failure

## Key Facts

- ✅ quic-go v0.43.1 works on servers
- ❌ quic-go v0.44.0+ fails on servers  
- ✅ quic-go v0.57.1 works locally (Ubuntu 24)
- ❌ quic-go v0.57.1 fails on servers (Ubuntu 24)

**Conclusion**: Something changed in quic-go 0.44.0 that breaks compatibility with the server environment specifically.

## Environment Differences

| Aspect | Local (Works) | Server (Fails) |
|--------|---------------|----------------|
| Kernel | 6.16.3-76061603-generic | 6.8.0-88-generic |
| UFW | Inactive | Active |
| iptables rules | 29 | 84-89 |
| UDP buffers | 212992 default | 134217728 default |
| Network | Tailscale | Tailscale + doublezero0 |

## What Changed in quic-go 0.44.0?

Need to investigate:
1. UDP socket option changes
2. ECN (Explicit Congestion Notification) support
3. Network interface feature detection
4. Packet handling changes
5. MTU discovery changes

## Next Steps

1. **Check quic-go Issues**: Search for existing reports about Ubuntu 24 or v0.44.0+ handshake issues
2. **TLS/Crypto Investigation**: 
   - Check if quic-go 0.44.0+ uses different TLS features
   - Compare OpenSSL versions between local and servers
   - Check CGO settings (quic-go is pure Go, but crypto/tls might differ)
3. **Compare socket options**: Compare what 0.43.1 vs 0.44.0+ uses
4. **Test kernel versions**: Try kernel 6.16.x on servers to match local
5. **File issue with quic-go**: With detailed environment comparison and test results

## Copilot Suggestions to Investigate

1. ✅ **Firewall/Networking**: Already ruled out (0.43.1 works on same servers)
2. ⚠️ **TLS library mismatch**: Need to check - Ubuntu 24 may have different crypto libraries
3. ✅ **Go version**: Both using 1.25.4, so not the issue
4. ⚠️ **Check quic-go Issues page**: For existing Ubuntu 24 reports

## Potential Workarounds to Test

1. Disable ECN in quic.Config
2. Force specific UDP socket options
3. Use different network interface binding
4. Test with kernel 6.16.x on servers (match local)

