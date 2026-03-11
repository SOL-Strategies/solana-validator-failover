#!/bin/bash

# Function to print usage
usage() {
    echo "Usage: $0 [--version | <mock command>]"
    exit 1
}

# Check if no arguments provided
if [ $# -eq 0 ]; then
    usage
fi

# Process arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --version)
            echo "0.505.20216 (44f9f393d167138abe1c819f7424990a56e1913e)"
            exit 0
            ;;
        set-identity)
            shift
            # Notify mock-solana of the identity change if MOCK_SOLANA_URL and VALIDATOR_NAME are set.
            # --force means setting to active; absence means passive.
            if [ -n "${MOCK_SOLANA_URL:-}" ] && [ -n "${VALIDATOR_NAME:-}" ]; then
                if echo "$@" | grep -q -- "--force"; then
                    # Check if this set-identity-to-active call should be simulated as failing.
                    FAIL_CHECK=$(curl -sf "${MOCK_SOLANA_URL}/fail-check?validator=${VALIDATOR_NAME}&action=set_active" 2>/dev/null || echo '{"fail":false}')
                    if echo "$FAIL_CHECK" | grep -q '"fail":true'; then
                        echo "fdctl-mock set-identity: SIMULATED FAILURE for ${VALIDATOR_NAME} (fail_next_set_active was set)" >&2
                        exit 1
                    fi
                    ACTION="set_active"
                else
                    ACTION="set_passive"
                fi
                curl -sf -X POST -H "Content-Type: application/json" \
                    -d "{\"action\":\"${ACTION}\",\"target\":\"${VALIDATOR_NAME}\"}" \
                    "${MOCK_SOLANA_URL}/action" >/dev/null 2>&1 || true
            fi
            echo "fdctl-mock set-identity: $@"
            exit 0
            ;;
        *)
            echo "fdctl-mock exec: $@"
            exit 0
            ;;
    esac
done
