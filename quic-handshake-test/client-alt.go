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
		fmt.Println("Usage: go run client-alt.go <server-address>")
		fmt.Println("Example: go run client-alt.go localhost:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("Connecting to QUIC server at %s (using explicit UDP connection)...\n", serverAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try using Dial with explicit UDP connection instead of DialAddr
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		panic(fmt.Sprintf("Failed to resolve address: %v", err))
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create UDP connection: %v", err))
	}
	defer udpConn.Close()

	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
	}

	conn, err := quic.Dial(ctx, udpConn, udpAddr, &tls.Config{
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

