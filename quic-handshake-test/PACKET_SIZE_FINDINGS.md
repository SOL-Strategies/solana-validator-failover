# Packet Size Testing Results

## Test Results Summary

| InitialPacketSize | Client Behavior | Server Receives? |
|-------------------|----------------|------------------|
| Not set (default) | No packets sent | ❌ |
| 1200 | Packets sent ✅ | ❌ |
| 1500 | Not tested | - |
| 2500 | No packets sent ❌ | ❌ |

## Key Finding

**v0.43.1 behavior:**
- Uses default packet sizes (no `InitialPacketSize` set)
- Sends **2504-byte packets** that **arrive at server** ✅
- Works perfectly

**v0.57.1 behavior:**
- Without `InitialPacketSize`: No packets sent ❌
- With `InitialPacketSize: 1200`: Packets sent but **don't arrive** ❌
- With `InitialPacketSize: 2500`: No packets sent (too large?) ❌

## Conclusion

The issue is **NOT** the packet size value itself. The problem is that **v0.57.1's packets (even when sent) don't make it through Tailscale**, while v0.43.1's packets do.

This suggests:
1. v0.57.1 constructs packets differently (structure, headers, etc.)
2. v0.57.1 uses different socket options or network settings
3. v0.57.1's packets are being filtered/dropped by Tailscale or the kernel
4. There's a fundamental incompatibility between v0.57.1 and the Tailscale network path

## Next Steps

1. Compare packet structure between v0.43.1 and v0.57.1 using tcpdump
2. Check socket options used by each version
3. Investigate ECN (Explicit Congestion Notification) handling differences
4. File bug report with quic-go showing this incompatibility

