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

	// Try with reduced InitialPacketSize to work around Tailscale MTU (1280 bytes)
	// QUIC requires ~1350 bytes, but Tailscale tunnels are often 1280 bytes
	// Based on: https://github.com/quic-go/quic-go/issues/5331#issuecomment-3313524914
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
		InitialPacketSize:     1200, // Reduced to fit in Tailscale MTU (1280 bytes)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}

	fmt.Printf("[MTU TEST] Calling quic.DialAddr with InitialPacketSize=%d...\n", quicConfig.InitialPacketSize)
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

