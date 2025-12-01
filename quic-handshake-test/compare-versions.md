# Comparing quic-go v0.43.1 vs v0.44.0+ Behavior

## Critical Finding

- ✅ **v0.43.1**: Works perfectly
- ❌ **v0.44.0+**: Packets sent but don't reach server (not in tcpdump)
- ❌ **Basic UDP (netcat)**: Packets sent but don't reach server (not in tcpdump)

## What This Means

v0.43.1 must be doing something fundamentally different that makes it work, while v0.44.0+ uses standard UDP behavior (like netcat) which doesn't work in this environment.

## Possible Differences in v0.43.1

1. **Different source interface/IP selection**
   - v0.43.1 might select a different source IP
   - v0.44.0+ might use default routing (which doesn't work)

2. **Different socket binding**
   - v0.43.1 might bind to a specific interface
   - v0.44.0+ might use default binding

3. **Different connection method**
   - v0.43.1 might use `Dial()` with explicit UDP connection
   - v0.44.0+ might use `DialAddr()` which selects source differently

4. **Different packet structure**
   - Unlikely, but possible

## Next Steps

1. **Test v0.43.1 with tcpdump** to see:
   - What source IP it uses
   - What interface it sends from
   - How packets differ

2. **Check routing tables** to understand why packets from default source don't work

3. **Compare socket options** between versions

4. **File bug report** with quic-go showing that v0.43.1 works but v0.44.0+ doesn't

## Workaround

Since v0.43.1 works, **stay on v0.43.1** until this is resolved. The security vulnerabilities in newer versions are mitigated by:
- Private network (Tailscale)
- Internal validator communication only
- Network-level protections

