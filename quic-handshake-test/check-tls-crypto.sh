#!/bin/bash
# Check TLS/crypto library differences that might affect quic-go

echo "=== Go Version ==="
go version
echo ""

echo "=== Go Environment (Crypto related) ==="
go env | grep -E "CGO|GOROOT|GOPATH|GOOS|GOARCH"
echo ""

echo "=== OpenSSL Version ==="
openssl version 2>&1 || echo "OpenSSL not found"
echo ""

echo "=== Crypto Libraries ==="
# Check if using system crypto or Go's crypto
echo "Checking crypto/tls package info..."
go list -m -json crypto/tls 2>/dev/null | grep -E "Path|Version" || echo "Using standard library crypto/tls"
echo ""

echo "=== TLS Certificate Support ==="
# Check TLS features
go doc crypto/tls 2>&1 | grep -E "Version|CipherSuite" | head -5
echo ""

echo "=== System Crypto Libraries ==="
# Check for system crypto libraries
ldconfig -p 2>/dev/null | grep -E "ssl|crypto" | head -10 || echo "ldconfig not available"
echo ""

echo "=== Check if CGO is enabled ==="
go env CGO_ENABLED
echo ""

echo "=== Kernel Crypto Modules ==="
lsmod | grep -E "crypto|tls" | head -10 || echo "No relevant crypto modules"
echo ""

