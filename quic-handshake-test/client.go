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
		fmt.Println("Usage: go run client.go <server-address>")
		fmt.Println("Example: go run client.go localhost:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("Connecting to QUIC server at %s...\n", serverAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try different config options to see what works
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
	}

	conn, err := quic.DialAddr(ctx, serverAddr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}, quicConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to dial server: %v", err))
	}
	defer conn.CloseWithError(0, "client done")

	fmt.Println("Connected! Opening stream...")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to open stream: %v", err))
	}
	defer stream.Close()

	fmt.Println("Stream opened, sending data...")

	// Send data
	message := []byte("Hello from client!")
	_, err = stream.Write(message)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("Data sent, reading response...")

	// Read response
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("Received %d bytes: %s\n", n, string(buf[:n]))
	fmt.Println("Client completed successfully!")
}

