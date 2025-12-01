#!/bin/bash
# Run server with GSO disabled
# GSO can cause issues with quic-go 0.44.0+ on some network interfaces

echo "Running server with QUIC_GO_DISABLE_GSO=true"
echo "This disables Generic Segmentation Offload which can cause packet issues"
echo ""

QUIC_GO_DISABLE_GSO=true go run server.go

