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

	// Try explicitly binding to Tailscale interface
	// First, get the Tailscale IP
	tailscaleIP := ""
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Name == "tailscale0" {
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
							tailscaleIP = ipnet.IP.String()
							fmt.Printf("[SERVER] Found Tailscale IP: %s\n", tailscaleIP)
							break
						}
					}
				}
			}
		}
	}

	// Try binding to Tailscale IP explicitly
	var listener *quic.Listener
	if tailscaleIP != "" {
		fmt.Printf("[SERVER] Attempting to bind to %s:%d\n", tailscaleIP, Port)
		listenAddr := fmt.Sprintf("%s:%d", tailscaleIP, Port)
		listener, err = quic.ListenAddr(listenAddr, tlsConfig, quicConfig)
		if err != nil {
			fmt.Printf("[SERVER] Failed to bind to %s: %v\n", listenAddr, err)
			fmt.Printf("[SERVER] Falling back to :%d\n", Port)
			listener, err = quic.ListenAddr(fmt.Sprintf(":%d", Port), tlsConfig, quicConfig)
		}
	} else {
		fmt.Printf("[SERVER] Tailscale interface not found, binding to :%d\n", Port)
		listener, err = quic.ListenAddr(fmt.Sprintf(":%d", Port), tlsConfig, quicConfig)
	}

	if err != nil {
		panic(fmt.Sprintf("Failed to create listener: %v", err))
	}
	defer listener.Close()

	fmt.Printf("[SERVER] QUIC server listening on port %d\n", Port)
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

