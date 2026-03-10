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
            echo "agave-validator 1.0.0 (src:7ac65892; feat:798020478, client:Mock)"
            exit 0
            ;;
        set-identity)
            shift
            # Notify mock-solana of the identity change if MOCK_SOLANA_URL and VALIDATOR_NAME are set.
            # --require-tower means setting to active; absence means passive.
            if [ -n "${MOCK_SOLANA_URL:-}" ] && [ -n "${VALIDATOR_NAME:-}" ]; then
                if echo "$@" | grep -q -- "--require-tower"; then
                    ACTION="set_active"
                else
                    ACTION="set_passive"
                fi
                curl -sf -X POST -H "Content-Type: application/json" \
                    -d "{\"action\":\"${ACTION}\",\"target\":\"${VALIDATOR_NAME}\"}" \
                    "${MOCK_SOLANA_URL}/action" >/dev/null 2>&1 || true
            fi
            echo "agave-validator-mock set-identity: $@"
            exit 0
            ;;
        *)
            echo "agave-validator-mock exec: $@"
            exit 0
            ;;
    esac
done
