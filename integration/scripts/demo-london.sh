#!/bin/bash
# demo-london.sh
# Runs london (passive validator) receiving a failover from chicago (active).
# VHS captures london's terminal — showing the plan, confirmation, and execution.
#
# Requires: make demo (mock-solana running on localhost:8899) and make build.

set -euo pipefail

# Always run from the project root regardless of how this script is invoked.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../.."

MOCK_URL="http://localhost:8899"
BINARY="./bin/solana-validator-failover-dev-linux-amd64"

# Reset: chicago starts as active.
curl -sf -X POST -H "Content-Type: application/json" \
    -d '{"action":"reset","target":"chicago"}' \
    "$MOCK_URL/action" >/dev/null

# Start chicago (active) in the background after a brief delay so london's
# QUIC server is ready to accept the connection when chicago connects.
(sleep 3 && "$BINARY" run --config integration/configs/demo-chicago.yaml \
    --to-peer london --yes) >/dev/null 2>&1 &
disown $!

# Exec london (passive) — replaces this shell so VHS records its output directly.
exec "$BINARY" run --config integration/configs/demo-london.yaml --not-a-drill
