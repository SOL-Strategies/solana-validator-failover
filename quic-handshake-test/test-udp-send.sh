#!/bin/bash
# Simple UDP packet sender to test connectivity

SERVER_IP="${1:-100.71.189.42}"
PORT="${2:-9898}"

echo "Sending UDP test packet to $SERVER_IP:$PORT..."

# Method 1: Using netcat (nc)
if command -v nc >/dev/null 2>&1; then
    echo "Using netcat..."
    echo "UDP test packet from $(hostname) at $(date)" | nc -u -w 1 "$SERVER_IP" "$PORT"
    echo "Packet sent via netcat"
fi

# Method 2: Using socat (if available)
if command -v socat >/dev/null 2>&1; then
    echo "Using socat..."
    echo "UDP test packet from $(hostname) at $(date)" | socat - UDP-SENDTO:$SERVER_IP:$PORT
    echo "Packet sent via socat"
fi

# Method 3: Using Python (if available)
if command -v python3 >/dev/null 2>&1; then
    echo "Using Python..."
    python3 << EOF
import socket
import sys
sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
message = f"UDP test packet from {socket.gethostname()}"
sock.sendto(message.encode(), ("$SERVER_IP", $PORT))
print(f"Packet sent: {message}")
sock.close()
EOF
fi

# Method 4: Using Go (if go is available)
if command -v go >/dev/null 2>&1; then
    echo "Using Go..."
    go run << 'EOF' "$SERVER_IP" "$PORT"
package main
import (
    "fmt"
    "net"
    "os"
)
func main() {
    if len(os.Args) < 3 {
        return
    }
    serverIP := os.Args[1]
    port := os.Args[2]
    addr, _ := net.ResolveUDPAddr("udp", net.JoinHostPort(serverIP, port))
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    defer conn.Close()
    message := fmt.Sprintf("UDP test packet from Go")
    _, err = conn.Write([]byte(message))
    if err != nil {
        fmt.Printf("Error writing: %v\n", err)
    } else {
        fmt.Printf("Packet sent: %s\n", message)
    }
}
EOF
fi

echo ""
echo "Check server tcpdump to see if packet arrived!"

