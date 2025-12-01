package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	ProtocolName = "solana-validator-failover"
	Port         = 9898
)

func generateTLSConfig() (*tls.Config, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{ProtocolName},
	}, nil
}

func main() {
	tlsConfig, err := generateTLSConfig()
	if err != nil {
		panic(fmt.Sprintf("Failed to generate TLS config: %v", err))
	}

	// Reproduce exact pattern from https://github.com/quic-go/quic-go/issues/5331
	// Use Transport with explicit UDP binding to udp4
	fmt.Printf("[SERVER] Reproducing issue #5331 pattern: Transport with udp4...\n")
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: Port})
	if err != nil {
		panic(fmt.Sprintf("Failed to create UDP listener: %v", err))
	}
	defer udpConn.Close()

	fmt.Printf("[SERVER] UDP listener created: %s\n", udpConn.LocalAddr())

	tr := quic.Transport{
		Conn: udpConn,
	}

	// Try different InitialPacketSize values - server might need different setting
	// Option 1: Don't set InitialPacketSize on server (let it accept client's size)
	// Option 2: Try 1280 (Tailscale MTU) or other values
	// Let's try without InitialPacketSize first - server should accept client's packets
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:       60 * time.Second,
		KeepAlivePeriod:      5 * time.Second,
		// Don't set InitialPacketSize - server should accept whatever client sends
		DisablePathMTUDiscovery: true,
	}

	fmt.Printf("[SERVER] Creating QUIC listener from Transport...\n")
	listener, err := tr.Listen(tlsConfig, quicConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to create listener: %v", err))
	}
	defer listener.Close()

	fmt.Printf("[SERVER] QUIC server listening on port %d\n", Port)
	fmt.Printf("[SERVER] Using InitialPacketSize: %d (required for tunnel interfaces)\n", quicConfig.InitialPacketSize)
	fmt.Printf("[SERVER] DisablePathMTUDiscovery: %v\n", quicConfig.DisablePathMTUDiscovery)

	// Print what address we're actually listening on
	if addr := listener.Addr(); addr != nil {
		fmt.Printf("[SERVER] Actually listening on: %s\n", addr.String())
	}

	fmt.Println("[SERVER] Waiting for client connection...")

	ctx := context.Background()
	fmt.Println("[SERVER] Waiting for Accept()...")
	conn, err := listener.Accept(ctx)
	if err != nil {
		fmt.Printf("[SERVER] Accept() failed: %v\n", err)
		panic(fmt.Sprintf("Failed to accept connection: %v", err))
	}
	fmt.Printf("[SERVER] Accept() succeeded! Connection from %s\n", conn.RemoteAddr())

	// Accept a stream
	fmt.Println("[SERVER] Waiting for AcceptStream()...")
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		fmt.Printf("[SERVER] AcceptStream() failed: %v\n", err)
		panic(fmt.Sprintf("Failed to accept stream: %v", err))
	}
	fmt.Printf("[SERVER] AcceptStream() succeeded!\n")

	fmt.Println("[SERVER] Reading data from stream...")

	// Read data from stream
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("[SERVER] Received %d bytes: %s\n", n, string(buf[:n]))

	// Send response
	response := []byte("Hello from server!")
	_, err = stream.Write(response)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("[SERVER] Sent response, closing stream...")
	stream.Close()

	fmt.Println("[SERVER] Server completed successfully!")
}
