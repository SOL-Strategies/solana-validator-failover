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
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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

	// Try different config options to see what works
	quicConfig := &quic.Config{
		HandshakeIdleTimeout: 30 * time.Second,
		MaxIdleTimeout:        60 * time.Second,
		KeepAlivePeriod:       5 * time.Second,
	}

	listener, err := quic.ListenAddr(
		fmt.Sprintf(":%d", Port),
		tlsConfig,
		quicConfig,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create listener: %v", err))
	}
	defer listener.Close()

	fmt.Printf("QUIC server listening on port %d\n", Port)
	fmt.Println("Waiting for client connection...")

	ctx := context.Background()
	conn, err := listener.Accept(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to accept connection: %v", err))
	}

	fmt.Printf("Accepted connection from %s\n", conn.RemoteAddr())

	// Accept a stream
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		panic(fmt.Sprintf("Failed to accept stream: %v", err))
	}

	fmt.Println("Accepted stream, reading data...")

	// Read data from stream
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("Received %d bytes: %s\n", n, string(buf[:n]))

	// Send response
	response := []byte("Hello from server!")
	_, err = stream.Write(response)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("Sent response, closing stream...")
	stream.Close()

	fmt.Println("Server completed successfully!")
}

