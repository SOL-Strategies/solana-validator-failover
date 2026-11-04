package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/solana-foundation/solana-go/v2/rpc"
	"github.com/solana-foundation/solana-go/v2/rpc/jsonrpc"
	"github.com/stretchr/testify/require"
)

func TestFileRPCSecretDiagnostics(t *testing.T) {
	for _, mode := range []string{"http", "rpc", "method unavailable", "transport"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/path-secret", r.URL.Path)
				require.Equal(t, "query-secret", r.URL.Query().Get("api-key"))
				user, password, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "user-secret", user)
				require.Equal(t, "password-secret", password)
				message := "user-secret password-secret path-secret query-secret response-secret"
				if mode == "http" {
					http.Error(w, message, http.StatusForbidden)
					return
				}
				var req struct {
					ID any `json:"id"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				code := -32000
				if mode == "method unavailable" {
					code = -32601
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": code, "message": message}}))
			}))
			defer server.Close()
			endpoint := "http://user-secret:password-secret@" + server.Listener.Addr().String() + "/path-secret?api-key=query-secret"
			if mode == "transport" {
				server.Close()
			}
			const label = "file:///run/secrets/rpc"
			urls := []string{endpoint}
			labels := map[string]string{endpoint: label}
			c := NewRPCClient(NewClientParams{LocalRPCURL: server.URL, ClusterRPCURLs: urls, ClusterRPCURLLabels: labels}).(*Client)
			labels[endpoint] = "modified"
			urls[0] = "modified"
			var logs bytes.Buffer
			c.loggerNetwork = log.NewWithOptions(&logs, log.Options{Level: log.DebugLevel})
			check := func(err error) {
				t.Helper()
				require.ErrorContains(t, err, label)
				for _, secret := range []string{endpoint, "user-secret", "password-secret", "path-secret", "query-secret", "response-secret", "modified"} {
					require.NotContains(t, err.Error(), secret)
				}
			}
			_, err := c.networkGetSlot(context.Background(), rpc.CommitmentConfirmed)
			check(err)
			_, err = c.networkGetVoteAccounts(context.Background(), nil)
			check(err)
			_, err = c.networkGetClusterNodes(context.Background())
			check(err)
			_, err = c.networkGetEpochInfo(context.Background(), rpc.CommitmentConfirmed)
			check(err)
			_, err = c.networkGetLeaderSchedule(context.Background())
			check(err)
			// Force discovery through the cluster to exercise its error logging too.
			c.localRPCClient = &MockRPCClient{}
			local := c.localRPCClient.(*MockRPCClient)
			local.On("GetClusterNodes", context.Background()).Return([]*rpc.GetClusterNodesResult{}, nil)
			_, err = c.NodeFromPubkey("missing-peer")
			check(err)
			require.Contains(t, logs.String(), label)
			for _, secret := range []string{endpoint, "user-secret", "password-secret", "path-secret", "query-secret", "response-secret"} {
				require.NotContains(t, logs.String(), secret)
			}
		})
	}
}

func TestFileRPCSecretFallback(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "response-secret", 403) }))
	defer failing.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":42}`) }))
	defer healthy.Close()
	secret := failing.URL + "/path-secret?api-key=query-secret"
	c := NewRPCClient(NewClientParams{ClusterRPCURLs: []string{secret, healthy.URL}, ClusterRPCURLLabels: map[string]string{secret: "file:///run/secrets/rpc"}}).(*Client)
	slot, err := c.networkGetSlot(context.Background(), rpc.CommitmentConfirmed)
	require.NoError(t, err)
	require.EqualValues(t, 42, slot)
}

func TestFileRPCSafeErrorCategories(t *testing.T) {
	const endpoint = "https://rpc.example.com/secret"
	c := NewRPCClient(NewClientParams{ClusterRPCURL: endpoint, ClusterRPCURLLabels: map[string]string{endpoint: "file:///secret"}}).(*Client)
	for _, original := range []error{context.Canceled, context.DeadlineExceeded} {
		safe := c.networkEndpointError(0, fmt.Errorf("response-secret: %w", original))
		require.ErrorIs(t, safe, original)
		require.NotContains(t, safe.Error(), "response-secret")
	}
	raw := &jsonrpc.RPCError{Code: -32601, Message: "response-secret", Data: "response-secret"}
	safe := c.networkEndpointError(0, raw)
	require.ErrorContains(t, safe, "-32601")
	require.NotContains(t, safe.Error(), "response-secret")
	var exposed *jsonrpc.RPCError
	require.False(t, errors.As(safe, &exposed))
	inline := NewRPCClient(NewClientParams{ClusterRPCURL: endpoint}).(*Client)
	require.ErrorIs(t, inline.networkEndpointError(0, raw), raw)
}
