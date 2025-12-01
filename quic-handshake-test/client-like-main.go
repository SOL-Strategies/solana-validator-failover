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
		fmt.Println("Usage: go run client-like-main.go <server-address>")
		fmt.Println("Example: go run client-like-main.go 100.71.189.42:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("[CLIENT] Connecting to QUIC server at %s...\n", serverAddr)
	fmt.Printf("[CLIENT] Using exact same method as main application (v0.43.1)...\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use EXACT same config as main application - nil config!
	// This matches: quic.DialAddr(c.ctx, c.serverAddress, &tls.Config{...}, nil)
	fmt.Printf("[CLIENT] Calling quic.DialAddr with nil config (like main app)...\n")
	fmt.Printf("[CLIENT] Starting dial at %s\n", time.Now().Format("15:04:05.000"))

	conn, err := quic.DialAddr(ctx, serverAddr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}, nil) // nil config - exactly like main application
	if err != nil {
		fmt.Printf("[CLIENT] ERROR at %s: %v\n", time.Now().Format("15:04:05.000"), err)
		panic(fmt.Sprintf("Failed to dial server: %v", err))
	}
	fmt.Printf("[CLIENT] Connection established at %s!\n", time.Now().Format("15:04:05.000"))
	defer conn.CloseWithError(0, "client done")

	fmt.Println("[CLIENT] Connected! Opening stream...")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to open stream: %v", err))
	}
	defer stream.Close()

	fmt.Println("[CLIENT] Stream opened, sending data...")

	message := []byte("Hello from client!")
	_, err = stream.Write(message)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("[CLIENT] Data sent, reading response...")

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("[CLIENT] Received %d bytes: %s\n", n, string(buf[:n]))
	fmt.Println("[CLIENT] SUCCESS!")
}

