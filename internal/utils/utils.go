package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/constants"
)

// ResolvePath converts a path that might contain ~ to an absolute path
func ResolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}

	// Handle ~ at the start of the path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return absPath, nil
}

// IsValidURLWithPort checks if the url is a valid url with a port
func IsValidURLWithPort(urlIn string) bool {
	// Add default scheme if none is present
	if !strings.Contains(urlIn, "://") {
		urlIn = "http://" + urlIn
	}

	parsedURL, err := url.Parse(urlIn)
	if err != nil {
		return false
	}

	if parsedURL.Host == "" || parsedURL.Port() == "" {
		return false
	}

	return true
}

// GetPublicIP returns the public IP address of the current machine
func GetPublicIP() (string, error) {
	log.Debug().Msg("getting public IP...")

	// Multiple IP services for redundancy
	services := []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ident.me",
		"https://checkip.amazonaws.com",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var lastErr error
	for _, service := range services {
		ip, err := getIPFromService(client, service)
		if err != nil {
			lastErr = err
			log.Debug().Err(err).Str("service", service).Msg("failed to get IP from service")
			continue
		}

		if isValidIP(ip) {
			log.Debug().
				Str("ip", ip).
				Str("service", service).
				Msg("public IP collected")
			return ip, nil
		}

		log.Debug().Str("ip", ip).Str("service", service).Msg("invalid IP received")
	}

	return "", fmt.Errorf("failed to get public IP from all services: %w", lastErr)
}

func getIPFromService(client *http.Client, service string) (string, error) {
	resp, err := client.Get(service)
	if err != nil {
		return "", fmt.Errorf("failed to get IP from %s: %w", service, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service %s returned status %d", service, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response from %s: %w", service, err)
	}

	// Remove any whitespace/newlines
	ip := strings.TrimSpace(string(body))
	return ip, nil
}

func isValidIP(ip string) bool {
	// Basic IP validation
	if net.ParseIP(ip) == nil {
		return false
	}

	// Reject private/local IPs
	if strings.HasPrefix(ip, "10.") ||
		strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "172.") ||
		ip == "127.0.0.1" {
		return false
	}

	return true
}

// FileExists checks if the file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists checks path is directory and it exists
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// RemoveFile removes the file
func RemoveFile(path string) error {
	return os.Remove(path)
}

// RunCommandParams represents the parameters for running a command
type RunCommandParams struct {
	CommandSlice []string
	DryRun       bool
	LogDebug     bool
}

// RunCommand runs a command and returns the output
func RunCommand(params RunCommandParams) error {
	if params.DryRun {
		log.Debug().Msgf("dry run: %s", strings.Join(params.CommandSlice, " "))
		return nil
	}

	// don't use up cycles unless we need to so that commands run faster
	if params.LogDebug {
		log.Debug().
			Str("command", strings.Join(params.CommandSlice, " ")).
			Msgf("running command")
	}

	cmd := exec.Command(params.CommandSlice[0], params.CommandSlice[1:]...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Str("command", strings.Join(params.CommandSlice, " ")).
			Str("output", string(output)).
			Err(err).
			Msgf("command failed")
		return err
	}

	log.Debug().Msgf("output: %s", string(output))
	return nil
}

// FileSize returns the size of the file
func FileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// FileChecksum returns the checksum of the file
func FileChecksum(path string) (string, error) {
	// read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// calculate the checksum
	return fmt.Sprintf("%x", sha256.Sum256(content)), nil
}

// GenerateTLSCertificate generates a TLS certificate
func GenerateTLSCertificate() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tlsCert, fmt.Errorf("failed to create TLS certificate: %w", err)
	}
	return tlsCert, nil
}

// EnsureBins ensures that the bins are installed
func EnsureBins(bins ...string) (err error) {
	for _, bin := range bins {
		_, err = exec.LookPath(bin)
		if err != nil {
			return fmt.Errorf("%s not found: %w", bin, err)
		}
	}
	return nil
}

// IsKnownCluster returns true if the cluster is a well-known Solana cluster
func IsKnownCluster(cluster string) bool {
	_, ok := constants.SolanaClusters[cluster]
	return ok
}

// ResolveAndValidateDir resolves the path and validates that the directory exists
func ResolveAndValidateDir(dir string) (resolvedDir string, err error) {
	resolvedDir, err = ResolvePath(dir)
	if err != nil {
		return "", fmt.Errorf("invalid dir: %s, must be a valid directory: %w", dir, err)
	}
	if !DirExists(resolvedDir) {
		return "", fmt.Errorf("invalid dir: %s, must be a valid directory", dir)
	}
	return resolvedDir, nil
}

// SortStringMap sorts a map by key
func SortStringMap(m map[string]string) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	ret := map[string]string{}
	for _, k := range keys {
		ret[k] = m[k]
	}
	return ret
}

// SafeCloseFile closes a file handle if it's not nil, ignoring any errors
func SafeCloseFile(f *os.File) {
	if f != nil {
		f.Close() // ignore error
	}
}
