package validator

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
)

func rpcSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rpc secret")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func TestFileRPCSources(t *testing.T) {
	const endpoint = "https://user:password@rpc.example.com/path-key?api-key=query-key"
	for _, localhost := range []bool{false, true} {
		t.Run(fmt.Sprint(localhost), func(t *testing.T) {
			source := rpcSecretFile(t, " \r\n # comment\r\n\t"+endpoint+" \r\ninvalid later line")
			if localhost {
				source = strings.Replace(source, "file:///", "file://localhost/", 1)
			}
			resolved, labels, err := resolveClusterRPCEndpoints("custom", "invalid ignored", []string{" ", endpoint, source, "http://localhost:8899", source})
			require.NoError(t, err)
			require.Equal(t, []string{endpoint, "http://localhost:8899"}, resolved)
			require.Equal(t, map[string]string{endpoint: source}, labels)
			require.Equal(t, []string{source, "http://localhost:8899"}, clusterRPCURLsForLog(resolved, labels))
			singular, err := resolveClusterRPCURL("custom", source)
			require.NoError(t, err)
			require.Equal(t, endpoint, singular)
		})
	}
}

func TestFileRPCFailures(t *testing.T) {
	for _, tc := range []struct{ name, content, message string }{
		{"empty", "", "no non-blank"},
		{"comments", " \n # comment\n", "no non-blank"},
		{"invalid first", "bad-secret\nhttps://valid.example.com", "HTTP/HTTPS"},
		{"nested", "file:///run/secrets/other", "HTTP/HTTPS"},
		{"scheme", "ftp://rpc.example.com/secret", "HTTP/HTTPS"},
		{"host", "https:///secret", "HTTP/HTTPS"},
		{"malformed", "https://rpc.example.com/%bad-secret%", "HTTP/HTTPS"},
		{"long URL", "https://rpc.example.com/" + strings.Repeat("x", 65536), "64 KiB"},
		{"long comment", "#" + strings.Repeat("x", 65536) + "\nhttps://valid.example.com", "64 KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := rpcSecretFile(t, tc.content)
			endpoints, labels, err := resolveClusterRPCEndpoints("custom", "", []string{source})
			require.ErrorContains(t, err, "validator.cluster_rpc_urls")
			require.ErrorContains(t, err, source)
			require.ErrorContains(t, err, tc.message)
			require.NotContains(t, err.Error(), "bad-secret")
			require.Nil(t, endpoints)
			require.Nil(t, labels)
		})
	}
	for _, source := range []string{
		"file:relative", "file://relative", "file:///tmp/rpc?secret=value", "file:///tmp/rpc?",
		"file:///tmp/rpc#fragment", "file:///tmp/rpc#", "file://user:password@localhost/tmp/rpc",
		"file://remote/tmp/rpc", "file:///tmp/%00rpc", "file:///tmp/%XX", "ftp://rpc.example.com",
		"https:///no-host", "https://:8899", "https://rpc.example.com/%XX",
	} {
		t.Run(source, func(t *testing.T) {
			_, _, err := resolveClusterRPCEndpoints("custom", source, nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), source)
		})
	}
}

func TestFileRPCFilesystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	source := (&url.URL{Scheme: "file", Path: path}).String()
	_, err := resolveClusterRPCURL("custom", source)
	require.ErrorContains(t, err, "cannot stat")
	_, err = resolveClusterRPCURL("custom", (&url.URL{Scheme: "file", Path: filepath.Dir(path)}).String())
	require.ErrorContains(t, err, "regular file")
	target := rpcSecretFile(t, "https://rpc.example.com?key=secret")
	parsed, err := url.Parse(target)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(parsed.Path, path))
	endpoint, err := resolveClusterRPCURL("custom", source)
	require.NoError(t, err)
	require.Equal(t, "https://rpc.example.com?key=secret", endpoint)
	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(parsed.Path, 0000))
		_, err = resolveClusterRPCURL("custom", source)
		require.ErrorContains(t, err, "cannot open")
	}
}

func TestConfigureFileRPCStartupAndAtomicity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/path-secret", r.URL.Path)
		require.Equal(t, "query-secret", r.URL.Query().Get("api-key"))
		user, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "user-secret", user)
		require.Equal(t, "password-secret", password)
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":42}`)
	}))
	defer server.Close()
	endpoint := "http://user-secret:password-secret@" + server.Listener.Addr().String() + "/path-secret?api-key=query-secret"
	source := rpcSecretFile(t, endpoint)
	v := createTestValidator(t).Validator
	var logs bytes.Buffer
	v.logger = log.NewWithOptions(&logs, log.Options{Level: log.DebugLevel})
	require.NoError(t, v.configureRPCClientWithURLs(server.URL, "custom", "", []string{source}, "200ms"))
	require.Contains(t, logs.String(), source)
	for _, secret := range []string{endpoint, "user-secret", "password-secret", "path-secret", "query-secret"} {
		require.NotContains(t, logs.String(), secret)
	}
	original := v.solanaRPCClient
	for _, bad := range []struct {
		sources  []string
		duration string
	}{
		{[]string{source, "invalid"}, "200ms"},
		{[]string{source}, "invalid"},
	} {
		require.Error(t, v.configureRPCClientWithURLs("http://localhost:9999", "custom", "", bad.sources, bad.duration))
		require.Same(t, original, v.solanaRPCClient)
		require.Equal(t, server.URL, v.RPCAddress)
	}
	parsed, err := url.Parse(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(parsed.Path, []byte("invalid updated file"), 0600))
	slot, err := v.solanaRPCClient.GetCurrentSlotContextWithCommitment(context.Background(), "confirmed")
	require.NoError(t, err)
	require.EqualValues(t, 42, slot)
	require.NoError(t, os.Remove(parsed.Path))
	slot, err = v.solanaRPCClient.GetCurrentSlotContextWithCommitment(context.Background(), "confirmed")
	require.NoError(t, err)
	require.EqualValues(t, 42, slot)
}
