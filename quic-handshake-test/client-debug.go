package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	ProtocolName = "solana-validator-failover"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run client-debug.go <server-address>")
		fmt.Println("Example: go run client-debug.go localhost:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("[DEBUG] Connecting to QUIC server at %s...\n", serverAddr)
	fmt.Printf("[DEBUG] Step 1: Resolving address...\n")

	// Resolve address first to see if that works
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		panic(fmt.Sprintf("Failed to resolve address: %v", err))
	}
	fmt.Printf("[DEBUG] Resolved to: %s\n", udpAddr)

	fmt.Printf("[DEBUG] Step 2: Creating context with 30s timeout...\n")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("[DEBUG] Step 3: Creating quic.Config...\n")
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
	}

	fmt.Printf("[DEBUG] Step 4: Creating TLS config...\n")
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}

	fmt.Printf("[DEBUG] Step 5: Calling quic.DialAddr (this is where it hangs/fails)...\n")
	fmt.Printf("[DEBUG] Starting dial at %s\n", time.Now().Format("15:04:05.000"))
	
	conn, err := quic.DialAddr(ctx, serverAddr, tlsConfig, quicConfig)
	if err != nil {
		fmt.Printf("[DEBUG] ERROR at %s: %v\n", time.Now().Format("15:04:05.000"), err)
		panic(fmt.Sprintf("Failed to dial server: %v", err))
	}
	fmt.Printf("[DEBUG] Connection established at %s!\n", time.Now().Format("15:04:05.000"))
	defer conn.CloseWithError(0, "client done")

	fmt.Println("[DEBUG] Connected! Opening stream...")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to open stream: %v", err))
	}
	defer stream.Close()

	fmt.Println("[DEBUG] Stream opened, sending data...")

	// Send data
	message := []byte("Hello from client!")
	_, err = stream.Write(message)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("[DEBUG] Data sent, reading response...")

	// Read response
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("[DEBUG] Received %d bytes: %s\n", n, string(buf[:n]))
	fmt.Println("[DEBUG] Client completed successfully!")
}

