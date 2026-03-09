package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCertMaterial holds generated cert/key PEM bytes for a single entity.
type testCertMaterial struct {
	certPEM []byte
	keyPEM  []byte
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
}

// generateTestCA generates a self-signed CA certificate.
func generateTestCA(t *testing.T) *testCertMaterial {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(derBytes)
	require.NoError(t, err)

	return &testCertMaterial{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}),
		keyPEM:  ecKeyPEM(t, key),
		cert:    cert,
		key:     key,
	}
}

// generateTestCert generates a certificate signed by the given CA, with the given IP SANs.
func generateTestCert(t *testing.T, ca *testCertMaterial, ips []net.IP) *testCertMaterial {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-node"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  ips,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(derBytes)
	require.NoError(t, err)

	return &testCertMaterial{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}),
		keyPEM:  ecKeyPEM(t, key),
		cert:    cert,
		key:     key,
	}
}

func ecKeyPEM(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// writeTempFiles writes cert material to temp dir and returns (caCertPath, certPath, keyPath).
func writeTempFiles(t *testing.T, ca *testCertMaterial, node *testCertMaterial) (string, string, string) {
	t.Helper()
	dir := t.TempDir()

	caCertPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")

	require.NoError(t, os.WriteFile(caCertPath, ca.certPEM, 0600))
	require.NoError(t, os.WriteFile(certPath, node.certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, node.keyPEM, 0600))

	return caCertPath, certPath, keyPath
}

func TestBuildMTLSServerConfig_Success(t *testing.T) {
	ca := generateTestCA(t)
	node := generateTestCert(t, ca, []net.IP{net.ParseIP("127.0.0.1")})
	caCertPath, certPath, keyPath := writeTempFiles(t, ca, node)

	cfg, err := BuildMTLSServerConfig(caCertPath, certPath, keyPath)

	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
	assert.NotNil(t, cfg.ClientCAs)
	assert.Len(t, cfg.Certificates, 1)
}

func TestBuildMTLSServerConfig_InvalidCertPath(t *testing.T) {
	ca := generateTestCA(t)
	dir := t.TempDir()
	caCertPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caCertPath, ca.certPEM, 0600))

	_, err := BuildMTLSServerConfig(caCertPath, "/nonexistent/node.crt", "/nonexistent/node.key")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load node certificate/key")
}

func TestBuildMTLSServerConfig_InvalidCAPath(t *testing.T) {
	ca := generateTestCA(t)
	node := generateTestCert(t, ca, nil)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")
	require.NoError(t, os.WriteFile(certPath, node.certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, node.keyPEM, 0600))

	_, err := BuildMTLSServerConfig("/nonexistent/ca.crt", certPath, keyPath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CA certificate")
}

func TestBuildMTLSServerConfig_InvalidCAPEM(t *testing.T) {
	ca := generateTestCA(t)
	node := generateTestCert(t, ca, nil)
	dir := t.TempDir()
	caCertPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")
	require.NoError(t, os.WriteFile(caCertPath, []byte("not a valid PEM"), 0600))
	require.NoError(t, os.WriteFile(certPath, node.certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, node.keyPEM, 0600))

	_, err := BuildMTLSServerConfig(caCertPath, certPath, keyPath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid PEM blocks found")
}

func TestBuildMTLSClientConfig_Success(t *testing.T) {
	ca := generateTestCA(t)
	node := generateTestCert(t, ca, []net.IP{net.ParseIP("127.0.0.1")})
	caCertPath, certPath, keyPath := writeTempFiles(t, ca, node)

	cfg, err := BuildMTLSClientConfig(caCertPath, certPath, keyPath)

	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.NotNil(t, cfg.RootCAs)
	assert.Len(t, cfg.Certificates, 1)
	assert.False(t, cfg.InsecureSkipVerify)
}

func TestBuildMTLSClientConfig_InvalidCertPath(t *testing.T) {
	ca := generateTestCA(t)
	dir := t.TempDir()
	caCertPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, os.WriteFile(caCertPath, ca.certPEM, 0600))

	_, err := BuildMTLSClientConfig(caCertPath, "/nonexistent/node.crt", "/nonexistent/node.key")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load node certificate/key")
}

func TestBuildMTLSClientConfig_InvalidCAPath(t *testing.T) {
	ca := generateTestCA(t)
	node := generateTestCert(t, ca, nil)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")
	require.NoError(t, os.WriteFile(certPath, node.certPEM, 0600))
	require.NoError(t, os.WriteFile(keyPath, node.keyPEM, 0600))

	_, err := BuildMTLSClientConfig("/nonexistent/ca.crt", certPath, keyPath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read CA certificate")
}

// TestMTLSHandshake_Success verifies that a server built with BuildMTLSServerConfig and a
// client built with BuildMTLSClientConfig can complete a mutual TLS handshake over TCP.
// (We use crypto/tls directly here to keep the test simple and independent of QUIC.)
func TestMTLSHandshake_Success(t *testing.T) {
	ca := generateTestCA(t)
	// Both nodes share the same CA; each gets its own cert with a loopback IP SAN
	serverNode := generateTestCert(t, ca, []net.IP{net.ParseIP("127.0.0.1")})
	clientNode := generateTestCert(t, ca, []net.IP{net.ParseIP("127.0.0.1")})

	dir := t.TempDir()
	caCertPath := filepath.Join(dir, "ca.crt")
	serverCertPath := filepath.Join(dir, "server.crt")
	serverKeyPath := filepath.Join(dir, "server.key")
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")

	require.NoError(t, os.WriteFile(caCertPath, ca.certPEM, 0600))
	require.NoError(t, os.WriteFile(serverCertPath, serverNode.certPEM, 0600))
	require.NoError(t, os.WriteFile(serverKeyPath, serverNode.keyPEM, 0600))
	require.NoError(t, os.WriteFile(clientCertPath, clientNode.certPEM, 0600))
	require.NoError(t, os.WriteFile(clientKeyPath, clientNode.keyPEM, 0600))

	serverTLS, err := BuildMTLSServerConfig(caCertPath, serverCertPath, serverKeyPath)
	require.NoError(t, err)
	serverTLS.ServerName = "127.0.0.1"

	clientTLS, err := BuildMTLSClientConfig(caCertPath, clientCertPath, clientKeyPath)
	require.NoError(t, err)
	clientTLS.ServerName = "127.0.0.1"

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	require.NoError(t, err)
	defer ln.Close()

	handshakeErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			handshakeErr <- err
			return
		}
		defer conn.Close()
		// Force handshake so we can check it succeeded
		handshakeErr <- conn.(*tls.Conn).Handshake()
	}()

	clientConn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	require.NoError(t, err)
	defer clientConn.Close()

	// Verify the server's peer cert (server sees client cert, client sees server cert)
	serverPeerCerts := clientConn.ConnectionState().PeerCertificates
	require.NotEmpty(t, serverPeerCerts)
	assert.Equal(t, "test-node", serverPeerCerts[0].Subject.CommonName)

	require.NoError(t, <-handshakeErr)
}

// TestMTLSHandshake_RejectsUntrustedClient confirms the server rejects a client that
// presents a certificate signed by a different (untrusted) CA.
func TestMTLSHandshake_RejectsUntrustedClient(t *testing.T) {
	trustedCA := generateTestCA(t)
	untrustedCA := generateTestCA(t)
	serverNode := generateTestCert(t, trustedCA, []net.IP{net.ParseIP("127.0.0.1")})
	// Client cert signed by a different CA that the server doesn't trust
	clientNode := generateTestCert(t, untrustedCA, []net.IP{net.ParseIP("127.0.0.1")})

	dir := t.TempDir()
	trustedCACertPath := filepath.Join(dir, "trusted-ca.crt")
	untrustedCACertPath := filepath.Join(dir, "untrusted-ca.crt")
	serverCertPath := filepath.Join(dir, "server.crt")
	serverKeyPath := filepath.Join(dir, "server.key")
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")

	require.NoError(t, os.WriteFile(trustedCACertPath, trustedCA.certPEM, 0600))
	require.NoError(t, os.WriteFile(untrustedCACertPath, untrustedCA.certPEM, 0600))
	require.NoError(t, os.WriteFile(serverCertPath, serverNode.certPEM, 0600))
	require.NoError(t, os.WriteFile(serverKeyPath, serverNode.keyPEM, 0600))
	require.NoError(t, os.WriteFile(clientCertPath, clientNode.certPEM, 0600))
	require.NoError(t, os.WriteFile(clientKeyPath, clientNode.keyPEM, 0600))

	serverTLS, err := BuildMTLSServerConfig(trustedCACertPath, serverCertPath, serverKeyPath)
	require.NoError(t, err)
	serverTLS.ServerName = "127.0.0.1"

	// Client trusts the untrusted CA (for its own server verification) but server won't trust it
	clientTLS, err := BuildMTLSClientConfig(trustedCACertPath, clientCertPath, clientKeyPath)
	require.NoError(t, err)
	clientTLS.ServerName = "127.0.0.1"

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	require.NoError(t, err)
	defer ln.Close()

	// Capture the server-side handshake result.
	// In TLS 1.3, client-cert verification completes server-side after the client's
	// Handshake() returns, so tls.Dial may succeed on the client side while the server
	// sends a post-handshake alert. We assert on the server's error to confirm rejection.
	serverHandshakeErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverHandshakeErr <- err
			return
		}
		defer conn.Close()
		serverHandshakeErr <- conn.(*tls.Conn).Handshake()
	}()

	clientConn, _ := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	if clientConn != nil {
		clientConn.Close()
	}

	// The server must reject the untrusted client certificate.
	assert.Error(t, <-serverHandshakeErr, "server must reject untrusted client cert")
}
