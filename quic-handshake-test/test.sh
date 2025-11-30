#!/bin/bash
# Simple test script for QUIC handshake test

set -e

echo "Building test programs..."
go mod tidy
go build -o server server.go
go build -o client client.go

echo ""
echo "Build complete!"
echo ""
echo "To test:"
echo "  On server 1: ./server"
echo "  On server 2: ./client <server1-address>:9898"
echo ""
echo "Or run directly:"
echo "  On server 1: go run server.go"
echo "  On server 2: go run client.go <server1-address>:9898"

