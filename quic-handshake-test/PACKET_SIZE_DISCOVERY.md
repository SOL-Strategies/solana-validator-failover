# Critical Discovery: Packet Size Matters!

## Finding

**v0.43.1 (working):**
- Sends **2504 byte** and **1252 byte** packets
- Packets **arrive** at server (visible in tcpdump)
- Connection succeeds

**v0.44.0+ with InitialPacketSize: 1200 (broken):**
- Sends **1200 byte** packets
- Packets **don't arrive** at server (not in tcpdump)
- Connection fails

## Key Insight

**Larger packets work, smaller packets don't!**

This is counterintuitive - we set `InitialPacketSize: 1200` to fit in Tailscale's 1280 byte MTU, but v0.43.1 sends 2504 byte packets which are WAY larger than the MTU!

## What This Means

1. **Tailscale handles fragmentation** - The 2504 byte packets get fragmented and reassembled correctly
2. **Smaller packets might trigger a different code path** that doesn't work
3. **v0.44.0+ might handle fragmentation differently** than v0.43.1

## Solution

**Don't set InitialPacketSize: 1200!** Let quic-go use its default packet sizes (like v0.43.1 does).

The server should accept whatever packet size the client sends, so we might not need any special server config.

## Test

Try removing `InitialPacketSize: 1200` from both client and server, and let quic-go use default packet sizes (which should be similar to v0.43.1's 2504/1252 bytes).

