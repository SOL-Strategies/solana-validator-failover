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
            echo "fdctl-mock set-identity: $@"
            exit 1
            ;;
        *)
            echo "fdctl-mock exec: $@"
            exit 0
            ;;
    esac
done
