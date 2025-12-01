#!/bin/bash
# Simple UDP client test

SERVER_IP="${1:-100.71.189.42}"
PORT=9898

echo "Sending UDP test packet to $SERVER_IP:$PORT..."

if command -v nc >/dev/null 2>&1; then
    echo "test message" | nc -u -w 1 "$SERVER_IP" "$PORT"
    echo "Sent!"
elif command -v socat >/dev/null 2>&1; then
    echo "test message" | socat - UDP-SENDTO:$SERVER_IP:$PORT
    echo "Sent!"
else
    echo "ERROR: Neither netcat nor socat found"
    exit 1
fi

