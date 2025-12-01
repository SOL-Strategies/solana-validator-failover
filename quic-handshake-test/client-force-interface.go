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
		fmt.Println("Usage: go run client-force-interface.go <server-address>")
		fmt.Println("Example: go run client-force-interface.go 100.71.189.42:9898")
		os.Exit(1)
	}

	serverAddr := os.Args[1]

	fmt.Printf("[CLIENT] Connecting to QUIC server at %s...\n", serverAddr)
	fmt.Printf("[CLIENT] Forcing UDP connection to use Tailscale interface...\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Resolve server address
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		panic(fmt.Sprintf("Failed to resolve address: %v", err))
	}
	fmt.Printf("[CLIENT] Resolved server: %s\n", udpAddr)

	// Try to find Tailscale interface and bind to it
	var udpConn *net.UDPConn
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Name == "tailscale0" {
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
							tailscaleIP := ipnet.IP
							fmt.Printf("[CLIENT] Found Tailscale IP: %s\n", tailscaleIP)
							
							// Bind UDP socket to Tailscale interface
							localAddr := &net.UDPAddr{
								IP:   tailscaleIP,
								Port: 0, // Let OS choose port
							}
							udpConn, err = net.ListenUDP("udp4", localAddr)
							if err != nil {
								fmt.Printf("[CLIENT] Failed to bind to Tailscale IP: %v\n", err)
								fmt.Printf("[CLIENT] Falling back to any interface...\n")
								udpConn, err = net.ListenUDP("udp4", nil)
							} else {
								fmt.Printf("[CLIENT] Bound to Tailscale interface: %s\n", udpConn.LocalAddr())
							}
							break
						}
					}
				}
				break
			}
		}
	}

	if udpConn == nil {
		fmt.Printf("[CLIENT] Tailscale interface not found, using default...\n")
		udpConn, err = net.ListenUDP("udp4", nil)
		if err != nil {
			panic(fmt.Sprintf("Failed to create UDP connection: %v", err))
		}
	}
	defer udpConn.Close()

	fmt.Printf("[CLIENT] UDP connection: %s -> %s\n", udpConn.LocalAddr(), udpAddr)

	quicConfig := &quic.Config{
		HandshakeIdleTimeout:    30 * time.Second,
		MaxIdleTimeout:          60 * time.Second,
		InitialPacketSize:       1200,
		DisablePathMTUDiscovery: true,
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}

	fmt.Printf("[CLIENT] Calling quic.Dial with explicit UDP connection...\n")
	fmt.Printf("[CLIENT] Starting dial at %s\n", time.Now().Format("15:04:05.000"))

	conn, err := quic.Dial(ctx, udpConn, udpAddr, tlsConfig, quicConfig)
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

