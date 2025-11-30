#!/bin/bash
# Script to test disabling kernel TLS to see if it fixes quic-go 0.44.0+ handshake

echo "=== Current Kernel TLS Module Status ==="
lsmod | grep -E "^tls\s"
echo ""

echo "=== Attempting to unload kernel TLS module ==="
echo "WARNING: This may affect other applications using kernel TLS"
echo ""

# Check what's using the tls module
echo "Checking what's using the tls module..."
lsmod | grep tls
echo ""

# Try to unload
echo "Attempting: sudo modprobe -r tls"
echo ""
echo "After unloading, test the QUIC connection again."
echo "If it works, we've found the issue!"
echo ""
echo "To reload later: sudo modprobe tls"

