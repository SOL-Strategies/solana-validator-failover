package failover

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/sol-strategies/solana-validator-failover/internal/solana"
	"github.com/sol-strategies/solana-validator-failover/pkg/constants"
	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/stretchr/testify/require"
)

// These tests run both peers over actual local QUIC and standard JSON RPC.
// Validator commands model each client's history naming and identity changes;
// they do not establish real validator interoperability.
func TestHandoffProtocol(t *testing.T) {
	for _, source := range []string{"agave", "jito-solana", "firedancer"} {
		for _, destination := range []string{"agave", "jito-solana", "firedancer"} {
			t.Run(source+"_to_"+destination, func(t *testing.T) { runHandoff(t, source, destination, "confirmed") })
		}
	}
	for _, mode := range []string{"disabled", "timeout", "dry-run", "malformed", "activation-failed", "stale-history"} {
		t.Run(mode, func(t *testing.T) { runHandoff(t, "firedancer", "firedancer", mode) })
	}
}

func runHandoff(t *testing.T, sourceFamily, destinationFamily, mode string) {
	t.Helper()
	dir := t.TempDir()
	votes := make([]uint64, 5000)
	for i := range votes {
		votes[i] = uint64(i + 1)
	}
	data, active := signedHistory(t, votes, 1)
	passive := [2]string{solanago.NewWallet().PublicKey().String(), solanago.NewWallet().PublicKey().String()}
	roleFiles := [2]string{filepath.Join(dir, "source-role"), filepath.Join(dir, "destination-role")}
	require.NoError(t, os.WriteFile(roleFiles[0], []byte(active), 0600))
	require.NoError(t, os.WriteFile(roleFiles[1], []byte(passive[1]), 0600))
	var slot atomic.Uint64
	slot.Store(10000)
	var voteCalls atomic.Int64
	var activationCalls atomic.Int64
	var rpcServer *httptest.Server
	rpcServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
			ID     json.RawMessage   `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		readRole := func(i int) string { b, _ := os.ReadFile(roleFiles[i]); return string(b) }
		var result any
		switch request.Method {
		case "getVersion":
			result = map[string]any{"solana-core": "26.10.3", "feature-set": 0}
		case "getIdentity":
			i := 0
			if r.URL.Query().Get("node") == "destination" {
				i = 1
			}
			result = map[string]any{"identity": readRole(i)}
		case "getAccountInfo":
			var key string
			_ = json.Unmarshal(request.Params[0], &key)
			payload := []byte{1}
			if key == AlpenglowFeatureAccount {
				payload = make([]byte, 9)
				payload[0] = 1
			}
			result = map[string]any{"context": map[string]any{"slot": 100}, "value": map[string]any{"lamports": 1, "owner": "11111111111111111111111111111111", "data": []string{base64.StdEncoding.EncodeToString(payload), "base64"}, "executable": false, "rentEpoch": 0}}
		case "getAgGenesisCert":
			result = map[string]any{"block": map[string]any{"slot": 99}}
		case "getSlot":
			result = slot.Add(1)
		case "getEpochInfo":
			result = map[string]any{"epoch": 1, "absoluteSlot": slot.Load(), "slotIndex": 10, "slotsInEpoch": 432000, "blockHeight": 1}
		case "getLeaderSchedule":
			result = map[string]any{}
		case "getVoteAccounts":
			voteCalls.Add(1)
			last := slot.Add(1)
			if mode == "timeout" {
				last = 1
			}
			result = map[string]any{"current": []any{map[string]any{"votePubkey": active, "nodePubkey": active, "lastVote": last, "rootSlot": last, "epochVoteAccount": true, "activatedStake": 1000000, "commission": 0, "epochCredits": []any{}}}, "delinquent": []any{}}
		case "getClusterNodes":
			nodes := []any{}
			for i, ip := range []string{"127.0.0.1", "127.0.0.2"} {
				nodes = append(nodes, map[string]any{"pubkey": readRole(i), "gossip": ip + ":8001", "version": "26.10.3"})
			}
			result = nodes
		default:
			t.Errorf("unexpected RPC method %s", request.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer rpcServer.Close()
	// The command server models identity changes and native export/import semantics.
	commandServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := 0
		if r.URL.Query().Get("node") == "destination" {
			i = 1
		}
		activationCalls.Add(1)
		_ = os.WriteFile(roleFiles[i], []byte(r.URL.Query().Get("identity")), 0600)
		w.WriteHeader(200)
	}))
	defer commandServer.Close()
	var infos [2]*NodeInfo
	for i, family := range []string{sourceFamily, destinationFamily} {
		nodeDir := filepath.Join(dir, fmt.Sprint(i))
		require.NoError(t, os.MkdirAll(nodeDir, 0700))
		history := filepath.Join(nodeDir, "vote_history-"+active+".bin")
		require.NoError(t, os.WriteFile(history, data, 0600))
		if mode == "malformed" && i == 0 {
			require.NoError(t, os.WriteFile(history, []byte("invalid history"), 0600))
		}
		name := []string{"source", "destination"}[i]
		// All paths are temporary test paths; shell scripts are supplied as files.
		script := filepath.Join(nodeDir, "identity.sh")
		body := "#!/bin/sh\nset -eu\n"
		if i == 0 && family == "firedancer" {
			if mode == "stale-history" {
				stale := time.Now().Add(-time.Hour)
				require.NoError(t, os.Chtimes(history, stale, stale))
			} else {
				body += fmt.Sprintf("touch %q\n", history)
			}
			body += fmt.Sprintf("mv %q %q\n", history, history+".bak")
		}
		if i == 1 {
			if mode == "activation-failed" {
				body += "exit 1\n"
			}
			body += "if [ \"${1:-}\" = --vote-history-file ]; then cp \"$2\" " + fmt.Sprintf("%q", history) + "; fi\n"
		}
		expected := active
		if i == 0 {
			expected = passive[0]
		}
		body += fmt.Sprintf("curl -fsS %q >/dev/null\n", commandServer.URL+"?node="+name+"&identity="+expected)
		require.NoError(t, os.WriteFile(script, []byte(body), 0700))
		command := script
		importPath := filepath.Join(nodeDir, "imports", "vote_history.bin")
		if i == 1 && family == "firedancer" {
			command += " --vote-history-file " + importPath
		}
		infos[i] = &NodeInfo{SolanaValidatorFailoverVersion: constants.AppVersion, Hostname: name, PublicIP: []string{"127.0.0.1", "127.0.0.2"}[i], ClientFamily: family, IsNativeFiredancer: family == "firedancer", ClientVersionRPC: "26.10.3", ClientVersion: "26.10.3", Consensus: ConsensusAlpenglow, ConsensusMode: ConsensusAlpenglow, ConsensusGenesisSlot: 99, VoteHistoryFile: history, VoteHistoryImportFile: importPath, SetIdentityCommand: command, SetIdentityCommandTemplate: command, SetIdentityActiveCommandTemplate: `true --vote-history-file {{ .VoteHistoryImportFile }}`, RPCAddress: rpcServer.URL + "?node=" + name, Identities: &identities.Identities{Active: &identities.Identity{PubKeyStr: active}, Passive: &identities.Identity{PubKeyStr: passive[i]}}}
	}
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	port := socket.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, socket.Close())
	enabled := mode != "disabled"
	server, err := NewServerFromConfig(ServerConfig{Port: port, PassiveNodeInfo: infos[1], SolanaRPCClient: solana.NewRPCClient(solana.NewClientParams{LocalRPCURL: infos[1].RPCAddress, ClusterRPCURL: rpcServer.URL}), RPCURL: infos[1].RPCAddress, AutoConfirm: true, IsDryRunFailover: mode == "dry-run", HandoffTimeout: time.Second, HandoffPollInterval: time.Millisecond, MonitorConfig: MonitorConfig{Voting: VotingConfig{enabled, 50 * time.Millisecond, time.Millisecond}}})
	require.NoError(t, err)
	serverResult := make(chan error, 1)
	go func() { serverResult <- server.Start() }()
	client := newTestClient(solana.NewRPCClient(solana.NewClientParams{LocalRPCURL: infos[0].RPCAddress, ClusterRPCURL: rpcServer.URL}))
	client.activeNodeInfo = infos[0]
	client.rpcURL = infos[0].RPCAddress
	client.serverAddress = fmt.Sprintf("127.0.0.1:%d", port)
	client.handoffTimeout = time.Second
	client.handoffPollInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(client.ctx, 5*time.Second)
	defer cancel()
	client.ctx = ctx
	// Dial retry is bounded by the test context, without interactive spinners.
	require.Eventually(t, func() bool { return client.tryQUICConnection() == nil }, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() { client.Conn.CloseWithError(0, "test complete"); client.cancel(); server.cancel() })
	clientErr := client.Start()
	var serverErr error
	select {
	case serverErr = <-serverResult:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not return its terminal result")
	}
	if mode == "timeout" || mode == "malformed" || mode == "activation-failed" || mode == "stale-history" {
		require.Error(t, clientErr)
		require.Error(t, serverErr)
	} else {
		require.NoError(t, clientErr)
		require.NoError(t, serverErr)
	}
	sourceRole, _ := os.ReadFile(roleFiles[0])
	destinationRole, _ := os.ReadFile(roleFiles[1])
	if mode == "dry-run" {
		require.Equal(t, active, string(sourceRole))
		require.Equal(t, passive[1], string(destinationRole))
		require.Zero(t, activationCalls.Load())
	} else if mode != "malformed" {
		require.Equal(t, passive[0], string(sourceRole))
		if mode == "stale-history" {
			require.Equal(t, passive[1], string(destinationRole))
			require.ErrorContains(t, clientErr, "refusing stale vote history")
			require.ErrorContains(t, serverErr, "refusing stale vote history")
			require.True(t, server.failoverStream.message.TerminalAck)
		} else if mode != "activation-failed" {
			require.Equal(t, active, string(destinationRole))
		}
	}
	if mode == "timeout" {
		require.ErrorContains(t, clientErr, "voting unverified")
		require.ErrorContains(t, serverErr, "voting unverified")
	}
	if mode == "disabled" || mode == "dry-run" {
		require.Zero(t, voteCalls.Load())
	}
	if mode == "confirmed" {
		require.Equal(t, "confirmed", server.failoverStream.message.Voting.Status)
	}
}
