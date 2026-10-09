#!/bin/bash
# demo-rollback.sh
# Demonstrates conservative recovery after a mocked activation failure.
# London reasserts passive identity, while Chicago remains passive because an
# activation attempt could have signed votes. Source recovery is manual.
# Requires: make demo and make build. This demo skips signed history transfer.

set -euo pipefail

# Always run from the project root regardless of how this script is invoked.
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../.."

MOCK_URL="http://localhost:8899"
BINARY="./bin/solana-validator-failover-dev-linux-amd64"

# MOCK_SOLANA_URL must be set so the mock validator scripts can call /fail-check.
export MOCK_SOLANA_URL="$MOCK_URL"

# Reset: chicago starts as active.
curl -sf -X POST -H "Content-Type: application/json" \
    -d '{"action":"reset","target":"chicago"}' \
    "$MOCK_URL/action" >/dev/null
echo "mock reset: chicago is active"

# Arm the failure: london's next set-identity-to-active call will return exit 1.
curl -sf -X POST -H "Content-Type: application/json" \
    -d '{"action":"fail_next_set_active","target":"london"}' \
    "$MOCK_URL/action" >/dev/null
echo "mock armed: london's next set-identity-to-active will fail → destination demotion and manual source recovery"

# Start chicago (active) in the background with VALIDATOR_NAME=chicago so the mock
# validator script can identify itself to /fail-check and /action.
(VALIDATOR_NAME=chicago "$BINARY" run \
    --config integration/configs/demo-chicago.yaml \
    --to-peer london --yes) 2>&1 | sed 's/^/[chicago] /' &
disown $!

# Run london (passive) — auto-confirm with --yes so we focus on the rollback output.
# VALIDATOR_NAME=london is needed so fdctl-mock.sh identifies itself to /fail-check.
VALIDATOR_NAME=london exec "$BINARY" run \
    --config integration/configs/demo-london.yaml \
    --not-a-drill --yes --skip-history-transfer
