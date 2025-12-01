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

	quicConfig := &quic.Config{
		HandshakeIdleTimeout:     30 * time.Second,
		MaxIdleTimeout:           60 * time.Second,
		KeepAlivePeriod:          5 * time.Second,
		InitialPacketSize:        1200,
		DisablePathMTUDiscovery:  true,
	}

	fmt.Printf("[SERVER] Creating UDP listener on port %d...\n", Port)
	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: Port})
	if err != nil {
		panic(fmt.Sprintf("Failed to create UDP listener: %v", err))
	}
	defer udpConn.Close()
	
	fmt.Printf("[SERVER] UDP listener created: %s\n", udpConn.LocalAddr())
	
	// First, test if we can receive raw UDP packets
	fmt.Println("[SERVER] Testing raw UDP packet reception...")
	go func() {
		buf := make([]byte, 1500)
		udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, addr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("[SERVER] Raw UDP read failed (expected if no packets): %v\n", err)
		} else {
			fmt.Printf("[SERVER] Received raw UDP packet: %d bytes from %s\n", n, addr)
			fmt.Printf("[SERVER] First 50 bytes: %x\n", buf[:min(n, 50)])
		}
	}()
	
	time.Sleep(2 * time.Second) // Give it time to try reading
	
	// Now try with quic-go Transport
	fmt.Printf("[SERVER] Creating QUIC listener from Transport...\n")
	tr := quic.Transport{
		Conn: udpConn,
	}
	
	listener, err := tr.Listen(tlsConfig, quicConfig)
	if err != nil {
		panic(fmt.Sprintf("Failed to create QUIC listener: %v", err))
	}
	defer listener.Close()

	fmt.Printf("[SERVER] QUIC server listening on port %d\n", Port)
	fmt.Printf("[SERVER] Using InitialPacketSize: %d\n", quicConfig.InitialPacketSize)
	fmt.Printf("[SERVER] DisablePathMTUDiscovery: %v\n", quicConfig.DisablePathMTUDiscovery)
	
	if addr := listener.Addr(); addr != nil {
		fmt.Printf("[SERVER] Actually listening on: %s\n", addr.String())
	}
	
	fmt.Println("[SERVER] Waiting for client connection...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	fmt.Println("[SERVER] Waiting for Accept()...")
	conn, err := listener.Accept(ctx)
	if err != nil {
		fmt.Printf("[SERVER] Accept() failed: %v\n", err)
		panic(fmt.Sprintf("Failed to accept connection: %v", err))
	}
	fmt.Printf("[SERVER] Accept() succeeded! Connection from %s\n", conn.RemoteAddr())

	fmt.Println("[SERVER] Waiting for AcceptStream()...")
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		fmt.Printf("[SERVER] AcceptStream() failed: %v\n", err)
		panic(fmt.Sprintf("Failed to accept stream: %v", err))
	}
	fmt.Printf("[SERVER] AcceptStream() succeeded!\n")

	fmt.Println("[SERVER] Reading data from stream...")
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		panic(fmt.Sprintf("Failed to read from stream: %v", err))
	}

	fmt.Printf("[SERVER] Received %d bytes: %s\n", n, string(buf[:n]))

	response := []byte("Hello from server!")
	_, err = stream.Write(response)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to stream: %v", err))
	}

	fmt.Println("[SERVER] Sent response, closing stream...")
	stream.Close()

	fmt.Println("[SERVER] Server completed successfully!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

