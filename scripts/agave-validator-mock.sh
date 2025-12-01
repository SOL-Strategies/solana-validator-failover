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
        --ledger)
            echo "agave-validator-mock --ledger: $@"
            exit 0
            ;;
        *)
            echo "agave-validator-mock exec: $@"
            exit 0
            ;;
    esac
done
