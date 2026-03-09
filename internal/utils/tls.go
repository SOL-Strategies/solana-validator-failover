package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// BuildMTLSServerConfig builds a *tls.Config for the QUIC server with mTLS.
// The server presents the cert at nodeCertPath/nodeKeyPath and requires connecting
// clients to present a certificate signed by the CA at caCertPath.
// NextProtos must be set by the caller before use.
func BuildMTLSServerConfig(caCertPath, nodeCertPath, nodeKeyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(nodeCertPath, nodeKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node certificate/key: %w", err)
	}

	caPool, err := loadCACertPool(caCertPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}, nil
}

// BuildMTLSClientConfig builds a *tls.Config for the QUIC client with mTLS.
// The client presents the cert at nodeCertPath/nodeKeyPath and verifies the server's
// certificate against the CA at caCertPath.
// NextProtos must be set by the caller before use.
func BuildMTLSClientConfig(caCertPath, nodeCertPath, nodeKeyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(nodeCertPath, nodeKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load node certificate/key: %w", err)
	}

	caPool, err := loadCACertPool(caCertPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
	}, nil
}

func loadCACertPool(caCertPath string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("failed to parse CA certificate at %s: no valid PEM blocks found", caCertPath)
	}
	return pool, nil
}
