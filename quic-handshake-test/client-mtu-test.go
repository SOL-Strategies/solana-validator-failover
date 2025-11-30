package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	ProtocolName = "solana-validator-failover"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run client-mtu-test.go <server-address>")
		fmt.Println("Example: go run client-mtu-test.go 100.71.189.42:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("[MTU TEST] Connecting to QUIC server at %s...\n", serverAddr)
	fmt.Printf("[MTU TEST] Testing with reduced MTU/datagram size for tunnel interfaces...\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try with reduced MaxDatagramSize to work around Tailscale MTU (1280 bytes)
	// QUIC requires ~1350 bytes, but Tailscale tunnels are often 1280 bytes
	// Setting a smaller MaxDatagramSize might help quic-go work around this
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
		// Try setting MaxDatagramSize to fit within Tailscale's 1280 byte MTU
		// Accounting for IP header (20 bytes IPv4, 40 bytes IPv6) and UDP header (8 bytes)
		// 1280 - 20 - 8 = 1252 bytes max payload for IPv4
		// But QUIC needs space for headers too, so try 1200 bytes
		MaxDatagramSize: 1200, // Reduced from default to fit in Tailscale MTU
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}

	fmt.Printf("[MTU TEST] Calling quic.DialAddr with MaxDatagramSize=%d...\n", quicConfig.MaxDatagramSize)
	fmt.Printf("[MTU TEST] Starting dial at %s\n", time.Now().Format("15:04:05.000"))

	conn, err := quic.DialAddr(ctx, serverAddr, tlsConfig, quicConfig)
	if err != nil {
		fmt.Printf("[MTU TEST] ERROR at %s: %v\n", time.Now().Format("15:04:05.000"), err)
		panic(fmt.Sprintf("Failed to dial server: %v", err))
	}
	fmt.Printf("[MTU TEST] Connection established at %s!\n", time.Now().Format("15:04:05.000"))
	defer conn.CloseWithError(0, "client done")

	fmt.Println("[MTU TEST] Connected! Opening stream...")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to open stream: %v", err))
	}
	defer stream.Close()

	fmt.Println("[MTU TEST] Stream opened, sending data...")

	message := []byte("Hello from client!")
	_, err = stream.Write(message)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("[MTU TEST] Data sent, reading response...")

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("[MTU TEST] Received %d bytes: %s\n", n, string(buf[:n]))
	fmt.Println("[MTU TEST] SUCCESS! MTU workaround worked!")
}

