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

## Testing

### Option 1: Disable Kernel TLS on servers
```bash
# Check if kTLS is being used
sudo ethtool -k <interface> | grep tls

# Disable kernel TLS (if possible)
# This might require kernel recompile or module unloading
```

### Option 2: Check if quic-go can disable kTLS
- Look for socket options to disable TLS offloading
- Check if quic.Config has options to control this

### Option 3: Compare kernel versions
- Local: 6.16.3 (newer, might have different kTLS behavior)
- Servers: 6.8.0 (older, might have kTLS enabled by default)

## Next Steps

1. Check if kernel TLS can be disabled on servers
2. Check quic-go 0.44.0+ release notes for TLS/kTLS changes
3. Test with kernel TLS disabled
4. File issue with quic-go about kTLS compatibility

