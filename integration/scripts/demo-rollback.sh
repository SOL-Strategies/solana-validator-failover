#!/bin/bash
# demo-rollback.sh
# Demonstrates the rollback feature: london (passive) fails to set its identity to active,
# signals chicago to revert to active, and re-asserts its own passive identity.
#
# Requires: make demo (mock-solana running on localhost:8899) and make build.
#
# What to observe:
#   - chicago switches to passive (set-identity succeeds)
#   - tower file syncs to london
#   - london attempts set-identity-to-active → SIMULATED FAILURE
#   - london signals chicago: rollback required
#   - london runs rollback.to_passive hooks + command
#   - chicago receives rollback signal, runs rollback.to_active hooks + command
#   - both nodes revert to original roles

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
echo "mock armed: london's next set-identity-to-active will fail → rollback will trigger"

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
    --not-a-drill --yes
