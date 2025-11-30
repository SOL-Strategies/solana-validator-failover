# Known Issue: quic-go 0.44.0+ on Ubuntu 24

## Problem

QUIC handshake fails with quic-go versions 0.44.0 and later on Ubuntu 24 servers. The handshake times out with "timeout: no recent network activity" or "context deadline exceeded" errors.

## Affected Versions

- **Working**: quic-go v0.43.1 ✅
- **Broken**: quic-go v0.44.0 through v0.57.1 ❌

## Symptoms

- Client cannot establish QUIC connection to server
- Handshake never completes
- Timeout errors: "timeout: no recent network activity" or "context deadline exceeded"
- Server is listening and ready, but client cannot connect
- Affects both `DialAddr()` and `Dial()` with explicit UDP connections

## Test Results

Tested on Ubuntu 24 servers:
- ✅ quic-go v0.43.1: Works perfectly
- ❌ quic-go v0.44.0: Handshake timeout
- ❌ quic-go v0.49.1: Handshake timeout  
- ❌ quic-go v0.57.1: Handshake timeout

Tested connection methods:
- ❌ `quic.DialAddr()`: Fails
- ❌ `quic.Dial()` with explicit UDP connection: Fails
- ❌ Various timeout configurations: All fail

## Root Cause

**UPDATE**: This is NOT a general Ubuntu 24 compatibility issue. The same quic-go version works locally on Ubuntu 24 but fails on servers.

This suggests an **environment-specific configuration issue** on the servers, such as:
- Network stack configuration differences
- Firewall/iptables rules
- UDP buffer sizes
- Network interface settings
- Containerization or network namespace differences
- Kernel module differences

The handshake packets appear to be sent but never received/acknowledged, suggesting a network configuration or UDP handling issue specific to the server environment.

## Workarounds

**Current workaround**: Stay on quic-go v0.43.1

**Security implications**: 
- CVE-2024-53259 (fixed in 0.48.2): ICMP Packet Too Large injection attack
- CVE-2025-59530 (fixed in 0.49.1): DoS via premature HANDSHAKE_DONE frame

These vulnerabilities are mitigated by:
- Using private network connections between validators
- Network-level protections (firewalls, private networks)
- The application's specific use case (internal validator communication)

## Next Steps

1. **File issue with quic-go**: Report this Ubuntu 24 compatibility issue
2. **Monitor quic-go releases**: Check for fixes in future versions
3. **Test periodically**: Re-test newer quic-go versions as they're released
4. **Consider alternatives**: If critical, consider:
   - Using a different QUIC library
   - Running in a container with different network stack
   - Using a different OS version

## References

- quic-go GitHub: https://github.com/quic-go/quic-go
- Ubuntu 24 Release Notes: https://wiki.ubuntu.com/NobleNumbat/ReleaseNotes
- CVE-2024-53259: https://github.com/advisories/GHSA-xxxxx
- CVE-2025-59530: https://github.com/advisories/GHSA-47m2-4cr7-mhcw

## Test Environment

- OS: Ubuntu 24.04 LTS
- Kernel: 6.x
- Go: 1.25.4
- Network: Tailscale mesh network (UDP)
- Servers: Multiple Ubuntu 24 instances

