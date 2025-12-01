#!/bin/bash

# print args received
case $1 in
    --force-identity)
        shift
        echo "forcing identity to $1"
        exit 0
        ;;
    *)
        echo "invalid argument: $1"
        exit 1
        ;;
esac