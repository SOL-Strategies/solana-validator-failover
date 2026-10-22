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
        --version|version)
            echo "26.10.3 (44f9f393d167138abe1c819f7424990a56e1913e)"
            exit 0
            ;;
        set-identity)
            shift
            # Match native Firedancer's CLI contract: set-identity does not
            # accept Agave's --force option.
            if printf '%s\n' "$@" | grep -qx -- "--force"; then
                echo "fdctl-mock set-identity: unknown option --force" >&2
                exit 1
            fi
            # Notify mock-solana of the identity change if MOCK_SOLANA_URL and VALIDATOR_NAME are set.
            # The fixtures use distinct active/passive keypair file names.
            if [ -n "${MOCK_SOLANA_URL:-}" ] && [ -n "${VALIDATOR_NAME:-}" ]; then
                if printf '%s\n' "$@" | grep -q -- "active-identity"; then
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
                # Model 26.10.3's export/import lifecycle used by the demos.
                HISTORY_DIR="./integration/test-ledgers/${VALIDATOR_NAME}"
                ACTIVE_PUBKEY="5yPKjBN6cvv6AQNiPMrE9B1mijM5YMRYJbNeTmPnx3ka"
                HISTORY_PATH="${HISTORY_DIR}/vote_history-${ACTIVE_PUBKEY}.bin"
                if [ "$ACTION" = set_passive ]; then
                    # Native Firedancer writes the final history before renaming it.
                    touch -- "$HISTORY_PATH" || exit 1
                    mv -- "$HISTORY_PATH" "${HISTORY_PATH}.bak" || exit 1
                else
                    history_file=""
                    previous=""
                    for arg in "$@"; do
                        if [ "$previous" = --vote-history-file ]; then history_file="$arg"; fi
                        previous="$arg"
                    done
                    if [ -z "$history_file" ]; then
                        echo "mock activation requires --vote-history-file" >&2
                        exit 1
                    fi
                    cp -- "$history_file" "$HISTORY_PATH" || exit 1
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
