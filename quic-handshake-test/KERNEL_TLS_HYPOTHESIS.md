# Kernel TLS Hypothesis

## Key Finding

**Servers (failing):**
- Kernel TLS modules loaded: `tls`, `crypto_simd`, `cryptd`
- Kernel: 6.8.0-88-generic

**Local (working):**
- Kernel TLS modules: Not loaded/visible
- Kernel: 6.16.3-76061603-generic

## Hypothesis

**Kernel TLS (kTLS) offloading** may be interfering with quic-go 0.44.0+'s TLS handshake handling.

Kernel TLS is a feature that offloads TLS operations to the kernel for performance. If quic-go 0.44.0+ changed how it handles TLS, it might conflict with kernel TLS offloading.

## Why This Makes Sense

1. **0.43.1 works** - Might not use features that conflict with kTLS
2. **0.44.0+ fails** - Might use TLS features that conflict with kTLS
3. **Local works** - No kTLS modules loaded, so no conflict
4. **Servers fail** - kTLS modules loaded, causing conflict

## Testing Results

**Local (works):**
- No kernel TLS modules loaded
- No TLS sysctl settings

**Servers (fail):**
- Kernel TLS module `tls` is loaded (155648 bytes)
- `net.ipv4.tcp_available_ulp = espintcp mptcp tls` (TLS available as TCP ULP)
- TLS hardware offload: off (so it's software-based kernel TLS)

**Conclusion**: The loaded `tls` kernel module is likely interfering with quic-go 0.44.0+.

### Test: Unload Kernel TLS Module

```bash
# Check current status
lsmod | grep tls

# Unload the module
sudo modprobe -r tls

# Test QUIC connection again
# If it works, we've confirmed the issue!

# To reload later (if needed)
sudo modprobe tls
```

### Option 2: Check if quic-go can disable kTLS
- Look for socket options to disable TLS offloading
- Check if quic.Config has options to control this

### Option 3: Compare kernel versions
- Local: 6.16.3 (newer, might have different kTLS behavior)
- Servers: 6.8.0 (older, might have kTLS enabled by default)

## Test Result: ❌ NOT THE ISSUE

**Tested**: Unloaded kernel TLS module (`modprobe -r tls`) on servers
**Result**: QUIC handshake still fails with quic-go 0.57.1

So kernel TLS module is **NOT** the cause. Back to investigating other differences.

## Remaining Differences to Investigate

1. **Kernel version**: 6.16.3 (local) vs 6.8.0 (servers)
2. **Network interfaces**: Servers have `doublezero0` interface
3. **iptables rules**: More complex ruleset on servers
4. **Something else in quic-go 0.44.0+** that interacts poorly with server environment

