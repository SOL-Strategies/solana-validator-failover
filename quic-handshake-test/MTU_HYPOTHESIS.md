# MTU Hypothesis - Tailscale Tunnel Interface

## Discovery

Based on [quic-go issue #5331](https://github.com/quic-go/quic-go/issues/5331) which mentions tunnel interfaces with MTU issues, and the fact that your servers use Tailscale (a tunnel interface), this could be the root cause.

## The Problem

**QUIC MTU Requirements:**
- IPv6: Minimum 1350 bytes
- IPv4: Minimum 1370 bytes

**Tailscale Default MTU:**
- Typically 1280 bytes (insufficient for QUIC)

**What Happens:**
- quic-go 0.44.0+ may perform MTU discovery during `DialAddr()` initialization
- On Tailscale tunnels with 1280 byte MTU, this discovery may hang or fail
- This would explain why no packets are sent (failure during initialization)
- v0.43.1 might not perform the same MTU discovery, which is why it works

## Evidence

1. ✅ Issue #5331 describes similar tunnel interface problems
2. ✅ Your servers use Tailscale (tunnel interface)
3. ✅ No packets sent (fails during initialization, possibly during MTU discovery)
4. ✅ v0.43.1 works (may not have the same MTU discovery code)
5. ✅ Works locally (no Tailscale tunnel, standard MTU)

## Testing

### Step 1: Check MTU Settings

Run `check-mtu.sh` on both local and server to compare:

```bash
./check-mtu.sh
```

Look for:
- Tailscale interface MTU (likely 1280)
- Path MTU to server
- Other tunnel interfaces

### Step 2: Test with InitialPacketSize

Try `client-mtu-test.go` which sets `InitialPacketSize: 1200` to fit within Tailscale's MTU (based on [issue #5331 comment](https://github.com/quic-go/quic-go/issues/5331#issuecomment-3313524914)):

```bash
# On server 1 (run server)
go run server.go

# On server 2 (run client)
go run client-mtu-test.go <server-ip>:9898
```

### Step 3: Increase Tailscale MTU (if test works)

If the InitialPacketSize workaround works, you can try increasing Tailscale's MTU:

```bash
# Check current MTU
ip link show tailscale0 | grep mtu

# Increase MTU (requires root)
sudo ip link set tailscale0 mtu 1420

# Verify
ip link show tailscale0 | grep mtu
```

**Note:** Tailscale may reset this on restart. You may need to configure it in Tailscale settings or use a systemd service.

## References

- [quic-go issue #5331](https://github.com/quic-go/quic-go/issues/5331) - Tunnel interface MTU issues
- [Tailscale issue #2633](https://github.com/tailscale/tailscale/issues/2633) - QUIC/H3 fails over Tailscale due to MTU
- QUIC RFC 9000: Minimum MTU requirements

## Next Steps

1. Run `check-mtu.sh` to confirm MTU differences
2. Test `client-mtu-test.go` to see if MaxDatagramSize helps
3. If it works, update the main application to use MaxDatagramSize
4. Consider increasing Tailscale MTU if possible
5. File issue with quic-go referencing #5331 and MTU discovery on tunnel interfaces

