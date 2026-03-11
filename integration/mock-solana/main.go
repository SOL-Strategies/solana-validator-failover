package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	activePubkey     = "5yPKjBN6cvv6AQNiPMrE9B1mijM5YMRYJbNeTmPnx3ka"
	activeVotePubkey = "5yPKjBN6cvv6AQNiPMrE9B1mijM5YMRYJbNeTmPnx3ka"
	startSlot        = uint64(290000000)
)

// validatorMeta holds the fixed metadata for each known validator in the demo network.
var validatorMeta = map[string]struct {
	publicIP      string
	passivePubkey string
}{
	"chicago": {"10.0.0.1", "6hDCmAJiCZdBP2TZzbbrUjAmXL5UDcmBLCuQ7CKq3Bq3"},
	"london":  {"10.0.0.2", "BdHyQJyt2sNb4ywKS2h5L5XVEbXxyjNJXXujhZJnCb2i"},
}

// MockSolanaServer simulates a Solana RPC node and exposes a control API.
type MockSolanaServer struct {
	mu                sync.RWMutex
	activeValidator   string          // which validator currently holds the active identity
	disconnected      map[string]bool // validators removed from gossip
	unhealthy         map[string]bool // validators whose local health check returns unhealthy
	failNextSetActive map[string]bool // validators whose next set-identity-to-active call should fail
	callingValidator  string          // populated from ?validator= query param per request
	slotCounter       atomic.Uint64
}

// NewMockSolanaServer creates a new MockSolanaServer with slot auto-increment.
func NewMockSolanaServer() *MockSolanaServer {
	s := &MockSolanaServer{
		activeValidator:   os.Getenv("ACTIVE_VALIDATOR"),
		disconnected:      make(map[string]bool),
		unhealthy:         make(map[string]bool),
		failNextSetActive: make(map[string]bool),
	}
	s.slotCounter.Store(startSlot)
	// Increment slot every 400ms to simulate real Solana slot progression.
	// waitUntilStartOfNextSlot polls until the slot changes, so this must increment.
	go func() {
		for range time.Tick(400 * time.Millisecond) {
			s.slotCounter.Add(1)
		}
	}()
	return s
}

// ── RPC types ────────────────────────────────────────────────────────────────

// ClusterNode represents a node entry returned by getClusterNodes.
type ClusterNode struct {
	Pubkey       string `json:"pubkey"`
	Gossip       string `json:"gossip"`
	TPU          string `json:"tpu"`
	RPC          string `json:"rpc"`
	Version      string `json:"version"`
	FeatureSet   int    `json:"featureSet"`
	ShredVersion int    `json:"shredVersion"`
}

// VoteAccount represents a single vote account entry.
type VoteAccount struct {
	VotePubkey       string     `json:"votePubkey"`
	NodePubkey       string     `json:"nodePubkey"`
	ActivatedStake   uint64     `json:"activatedStake"`
	EpochVoteAccount bool       `json:"epochVoteAccount"`
	Commission       uint8      `json:"commission"`
	LastVote         uint64     `json:"lastVote"`
	EpochCredits     [][]uint64 `json:"epochCredits"`
	RootSlot         uint64     `json:"rootSlot"`
}

// VoteAccountsResult holds current and delinquent vote accounts.
type VoteAccountsResult struct {
	Current    []VoteAccount `json:"current"`
	Delinquent []VoteAccount `json:"delinquent"`
}

// ── Control types ─────────────────────────────────────────────────────────────

// ControlAction is the unified control request accepted by the /action endpoint.
type ControlAction struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func (s *MockSolanaServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	method, _ := req["method"].(string)

	// Track which validator is making the call via ?validator= query param.
	if v := r.URL.Query().Get("validator"); v != "" {
		s.mu.Lock()
		s.callingValidator = v
		s.mu.Unlock()
	}

	var result any
	switch method {
	case "getClusterNodes":
		result = s.getClusterNodes()
	case "getHealth":
		result = s.getHealth()
	case "getSlot":
		result = s.slotCounter.Load()
	case "getEpochInfo":
		result = s.getEpochInfo()
	case "getLeaderSchedule":
		// Return an empty schedule — the validator is not a leader this epoch,
		// so the failover tool proceeds without waiting for a leader slot gap.
		result = map[string]any{}
	case "getVoteAccounts":
		result = s.getVoteAccounts()
	default:
		result = map[string]any{
			"error": map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("method not found: %s", method),
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      req["id"],
		"result":  result,
	})
}

func (s *MockSolanaServer) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var action ControlAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	switch action.Action {
	case "set_active":
		s.activeValidator = action.Target
		log.Printf("[control] set_active: %q", action.Target)

	case "set_passive":
		if s.activeValidator == action.Target {
			s.activeValidator = ""
			log.Printf("[control] set_passive: %q (was active, cleared)", action.Target)
		} else {
			log.Printf("[control] set_passive: %q (already passive, no-op)", action.Target)
		}

	case "disconnect":
		s.disconnected[action.Target] = true
		if s.activeValidator == action.Target {
			s.activeValidator = ""
			log.Printf("[control] disconnect: %q (was active, cleared)", action.Target)
		} else {
			log.Printf("[control] disconnect: %q", action.Target)
		}

	case "reconnect":
		delete(s.disconnected, action.Target)
		log.Printf("[control] reconnect: %q", action.Target)

	case "set_unhealthy":
		s.unhealthy[action.Target] = true
		log.Printf("[control] set_unhealthy: %q", action.Target)

	case "set_healthy":
		delete(s.unhealthy, action.Target)
		log.Printf("[control] set_healthy: %q", action.Target)

	case "reset":
		s.disconnected = make(map[string]bool)
		s.unhealthy = make(map[string]bool)
		s.failNextSetActive = make(map[string]bool)
		s.activeValidator = action.Target
		log.Printf("[control] reset: active=%q", action.Target)

	case "fail_next_set_active":
		s.failNextSetActive[action.Target] = true
		log.Printf("[control] fail_next_set_active: %q — next set-identity-to-active call will fail", action.Target)

	default:
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("unknown action: %s", action.Action), http.StatusBadRequest)
		return
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handlePublicIP returns the public IP for a validator identified by ?validator= query param.
// Allows local demo runs to bypass external IP lookup services.
func (s *MockSolanaServer) handlePublicIP(w http.ResponseWriter, r *http.Request) {
	if v := r.URL.Query().Get("validator"); v != "" {
		if meta, ok := validatorMeta[v]; ok {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(meta.publicIP))
			return
		}
	}

	clientIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		clientIP = fwd
	}
	if i := strings.LastIndex(clientIP, ":"); i != -1 {
		clientIP = clientIP[:i]
	}

	for _, meta := range validatorMeta {
		if meta.publicIP == clientIP {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(meta.publicIP))
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("10.0.0.99"))
}

// ── RPC method implementations ────────────────────────────────────────────────

// getClusterNodes returns gossip entries for all connected validators.
// Gossip addresses use the public IPs from validatorMeta so that the failover
// tool's peer-IP matching (NodeFromIP) works correctly.
func (s *MockSolanaServer) getClusterNodes() []ClusterNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nodes []ClusterNode
	for name, meta := range validatorMeta {
		if s.disconnected[name] {
			continue
		}

		pubkey := meta.passivePubkey
		if name == s.activeValidator {
			pubkey = activePubkey
		}

		nodes = append(nodes, ClusterNode{
			Pubkey:       pubkey,
			Gossip:       fmt.Sprintf("%s:8001", meta.publicIP),
			TPU:          fmt.Sprintf("%s:8003", meta.publicIP),
			RPC:          fmt.Sprintf("%s:8899", meta.publicIP),
			Version:      "2.1.0",
			FeatureSet:   123456789,
			ShredVersion: 12345,
		})
	}
	return nodes
}

// getHealth returns "ok" unless the calling validator has been marked unhealthy.
func (s *MockSolanaServer) getHealth() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.unhealthy[s.callingValidator] {
		return "behind"
	}
	return "ok"
}

// getEpochInfo returns epoch info consistent with the current slot counter.
// SlotsPerEpoch is set to 432000 (mainnet-beta epoch length).
func (s *MockSolanaServer) getEpochInfo() map[string]any {
	slot := s.slotCounter.Load()
	slotsPerEpoch := uint64(432000)
	epoch := slot / slotsPerEpoch
	slotIndex := slot % slotsPerEpoch
	return map[string]any{
		"epoch":            epoch,
		"slotIndex":        slotIndex,
		"slotsInEpoch":     slotsPerEpoch,
		"absoluteSlot":     slot,
		"blockHeight":      slot - 1000,
		"transactionCount": slot * 100,
	}
}

// getVoteAccounts returns the active validator's pubkey in Current[].
// EpochCredits carries two epochs of data so the credit-rank monitor has
// material to compare against.
func (s *MockSolanaServer) getVoteAccounts() VoteAccountsResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot := s.slotCounter.Load()

	if s.activeValidator == "" || s.disconnected[s.activeValidator] {
		return VoteAccountsResult{
			Current:    []VoteAccount{},
			Delinquent: []VoteAccount{},
		}
	}

	return VoteAccountsResult{
		Current: []VoteAccount{
			{
				VotePubkey:       activeVotePubkey,
				NodePubkey:       activePubkey,
				ActivatedStake:   1_000_000_000_000,
				EpochVoteAccount: true,
				Commission:       0,
				LastVote:         slot - 2,
				EpochCredits: [][]uint64{
					{671, 350000, 0},
					{672, 12500, 350000},
				},
				RootSlot: slot - 32,
			},
			// A second validator for rank comparison
			{
				VotePubkey:       "FakeVote111111111111111111111111111111111111",
				NodePubkey:       "FakeNode111111111111111111111111111111111111",
				ActivatedStake:   500_000_000_000,
				EpochVoteAccount: true,
				Commission:       0,
				LastVote:         slot - 5,
				EpochCredits: [][]uint64{
					{671, 340000, 0},
					{672, 11000, 340000},
				},
				RootSlot: slot - 32,
			},
		},
		Delinquent: []VoteAccount{},
	}
}

// handleFailCheck is called by mock validator scripts before executing a set-identity-to-active
// command. It returns {"fail": true} (and clears the flag) when fail_next_set_active was set
// for the requesting validator, signalling the script to exit 1 and trigger rollback.
//
// Query params: ?validator=<name>&action=set_active
func (s *MockSolanaServer) handleFailCheck(w http.ResponseWriter, r *http.Request) {
	validator := r.URL.Query().Get("validator")
	action := r.URL.Query().Get("action")

	shouldFail := false
	if validator != "" && action == "set_active" {
		s.mu.Lock()
		if s.failNextSetActive[validator] {
			shouldFail = true
			delete(s.failNextSetActive, validator)
			log.Printf("[fail-check] validator=%q action=%q → FAIL (flag cleared)", validator, action)
		}
		s.mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"fail": shouldFail})
}

func main() {
	server := NewMockSolanaServer()

	http.HandleFunc("/", server.handleRPC)
	http.HandleFunc("/action", server.handleAction)
	http.HandleFunc("/public-ip", server.handlePublicIP)
	http.HandleFunc("/fail-check", server.handleFailCheck)

	port := ":8899"
	log.Printf("mock-solana starting on %s", port)
	log.Printf("initial active validator: %q", server.activeValidator)
	log.Printf("started at %s", time.Now().Format(time.RFC3339))

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
