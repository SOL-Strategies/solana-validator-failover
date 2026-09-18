package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
)

// jsonRPCMethodNotFound is the standard JSON-RPC 2.0 error code for "Method not found".
// See: https://www.jsonrpc.org/specification#error_object
const jsonRPCMethodNotFound = -32601

// RPCClientInterface defines the interface for RPC client operations - a solana rpc client interface
type RPCClientInterface interface {
	GetClusterNodes(ctx context.Context) ([]*rpc.GetClusterNodesResult, error)
	GetVoteAccounts(ctx context.Context, opts *rpc.GetVoteAccountsOpts) (*rpc.GetVoteAccountsResult, error)
	GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
	GetLeaderSchedule(ctx context.Context) (rpc.GetLeaderScheduleResult, error)
	GetHealth(ctx context.Context) (string, error)
	GetEpochInfo(ctx context.Context, commitment rpc.CommitmentType) (*rpc.GetEpochInfoResult, error)
	GetVersion(ctx context.Context) (*rpc.GetVersionResult, error)
}

// ClientInterface defines the interface for solana rpc operations - just simple wrappers around the rpc client
type ClientInterface interface {
	// NodeFromIP returns a Node from an IP address
	NodeFromIP(ip string) (*Node, error)
	// NodeFromPubkey returns a Node from a pubkey
	NodeFromPubkey(pubkey string) (*Node, error)
	// NodeFromIPWithExpectedPubkey returns a Node from an IP address, preferring the entry
	// whose pubkey matches expectedPubkey. During a gossip identity transition, both the old
	// and new CRDS entries for a node briefly coexist; this method returns the expected entry
	// if present, falling back to the first entry for the IP otherwise. Returns an error only
	// if no entry exists for the IP at all.
	NodeFromIPWithExpectedPubkey(ip, expectedPubkey string) (*Node, error)
	// GetCreditRankedVoteAccountFromPubkey returns the credit rank-sorted current vote accounts rank is the difference
	// between current epoch credits and total credits (descending)
	GetCreditRankedVoteAccountFromPubkey(pubkey string) (*rpc.VoteAccountsResult, int, error)
	// GetCurrentSlot returns the current slot
	GetCurrentSlot() (slot uint64, err error)
	// GetTimeToNextLeaderSlotForPubkey returns the time to the next leader slot for the given pubkey
	GetTimeToNextLeaderSlotForPubkey(pubkey solanago.PublicKey) (isOnLeaderSchedule bool, timeToNextLeaderSlot time.Duration, err error)
	// GetLocalNodeHealth returns the health of the local node
	GetLocalNodeHealth() (string, error)
	// IsLocalNodeHealthy returns true if the local node is healthy
	IsLocalNodeHealthy() bool
	// GetLocalNodeVersion returns the solana-core version string from the local validator's getVersion RPC call.
	// This may differ from the gossip-reported version for clients like jito-solana or firedancer.
	// Returns an empty string and an error if the call fails.
	GetLocalNodeVersion() (string, error)
	GetLocalIdentity(ctx context.Context) (string, error)
	GetVoteAccountState(ctx context.Context, votePubkey string, local bool, commitment rpc.CommitmentType) (*rpc.VoteAccountsResult, error)
}

// IdentityTransitionStatus describes the optional Agave/Jito RPC extension
// used when handing an Agave-derived validator to native Firedancer.
type IdentityTransitionStatus struct {
	Version         uint64  `json:"version"`
	Sequence        uint64  `json:"sequence"`
	State           string  `json:"state"`
	Consensus       string  `json:"consensus"`
	CurrentIdentity string  `json:"currentIdentity"`
	FromIdentity    string  `json:"fromIdentity"`
	ToIdentity      string  `json:"toIdentity"`
	VoteAccount     string  `json:"voteAccount"`
	LastVoteSlot    uint64  `json:"lastVoteSlot"`
	TowerRootSlot   uint64  `json:"towerRootSlot"`
	Error           *string `json:"error"`
}

// IdentityTransitionClient is optional. Its absence means the peer is an
// unpatched Agave/Jito validator and requires the explicitly confirmed slot
// fallback for Agave-derived to native Firedancer handoffs.
type IdentityTransitionClient interface {
	GetIdentityTransitionStatus(context.Context) (*IdentityTransitionStatus, error)
	ProbeIdentityTransitionStatus(context.Context) (bool, error)
}

// ContextSlotClient exposes a cancellable network slot request for long
// compatibility waits. Existing implementations remain valid through the
// optional interface.
type ContextSlotClient interface {
	GetCurrentSlotContext(context.Context) (uint64, error)
}

// ContextSlotCommitmentClient exposes a cancellable slot request with an
// explicit commitment, used by the finalized fallback barrier.
type ContextSlotCommitmentClient interface {
	GetCurrentSlotContextWithCommitment(context.Context, rpc.CommitmentType) (uint64, error)
}

// ContextVoteAccountClient is implemented by RPC clients that can bound
// automatic vote-account discovery with a caller-provided deadline.
type ContextVoteAccountClient interface {
	GetCreditRankedVoteAccountFromPubkeyContext(context.Context, string) (*rpc.VoteAccountsResult, int, error)
}

// Client implements Interface using an RPC client
type Client struct {
	localRPCClient      RPCClientInterface
	networkRPCClient    RPCClientInterface
	loggerLocal         *log.Logger
	loggerNetwork       *log.Logger
	averageSlotDuration time.Duration
	localRPCURL         string
	networkRPCURL       string
}

// NewClientParams is the parameters for creating a new client
type NewClientParams struct {
	LocalRPCURL         string
	ClusterRPCURL       string
	AverageSlotDuration time.Duration // average slot duration, defaults to 400ms
}

// NewRPCClient creates a new client for the given solana cluster
func NewRPCClient(params NewClientParams) ClientInterface {
	avgSlotDuration := params.AverageSlotDuration
	if avgSlotDuration <= 0 {
		avgSlotDuration = 400 * time.Millisecond
	}
	return &Client{
		localRPCClient:      rpc.New(params.LocalRPCURL),
		networkRPCClient:    rpc.New(params.ClusterRPCURL),
		loggerLocal:         log.With("rpc_client", "local"),
		loggerNetwork:       log.With("rpc_client", "network"),
		averageSlotDuration: avgSlotDuration,
		localRPCURL:         params.LocalRPCURL,
		networkRPCURL:       params.ClusterRPCURL,
	}
}

type identityTransitionRPCResponse struct {
	JSONRPC string                    `json:"jsonrpc"`
	Result  *IdentityTransitionStatus `json:"result"`
	Error   *jsonrpc.RPCError         `json:"error"`
}

func (c *Client) GetIdentityTransitionStatus(ctx context.Context) (*IdentityTransitionStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "identityTransitionStatus", "params": []any{}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.localRPCURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("identityTransitionStatus RPC request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("identityTransitionStatus RPC returned HTTP %s", resp.Status)
	}
	var decoded identityTransitionRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode identityTransitionStatus response: %w", err)
	}
	if decoded.Error != nil {
		return nil, decoded.Error
	}
	if decoded.Result == nil {
		return nil, fmt.Errorf("identityTransitionStatus RPC returned no result")
	}
	return decoded.Result, nil
}

func (c *Client) ProbeIdentityTransitionStatus(ctx context.Context) (bool, error) {
	status, err := c.GetIdentityTransitionStatus(ctx)
	if err != nil {
		var rpcErr *jsonrpc.RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == jsonRPCMethodNotFound {
			return false, nil
		}
		return false, err
	}
	return status != nil, nil
}

func (c *Client) GetCurrentSlotContext(ctx context.Context) (uint64, error) {
	return c.GetCurrentSlotContextWithCommitment(ctx, rpc.CommitmentConfirmed)
}

func (c *Client) GetCurrentSlotContextWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return c.networkRPCClient.GetSlot(ctx, commitment)
}

// GetLocalNodeHealth returns the health of the local node
func (c *Client) GetLocalNodeHealth() (string, error) {
	result, err := c.localRPCClient.GetHealth(context.Background())
	if err != nil {
		return err.Error(), fmt.Errorf("failed to get local node health: %w", err)
	}
	return string(result), nil
}

// IsLocalNodeHealthy returns true if the local node is healthy
func (c *Client) IsLocalNodeHealthy() bool {
	result, err := c.GetLocalNodeHealth()
	if err != nil {
		c.loggerLocal.Debug("failed to get local node health", "err", err)
		return false
	}
	isHealthy := result == rpc.HealthOk
	if !isHealthy {
		c.loggerLocal.Debug("local node health", "result", result)
	}
	return isHealthy
}

// GetLocalNodeVersion returns the solana-core version string from the local validator's getVersion RPC call.
func (c *Client) GetLocalNodeVersion() (string, error) {
	result, err := c.localRPCClient.GetVersion(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get local node version: %w", err)
	}
	return result.SolanaCore, nil
}

// GetLocalIdentity returns the identity reported by the validator's local RPC.
func (c *Client) GetLocalIdentity(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	identityClient, ok := c.localRPCClient.(interface {
		GetIdentity(context.Context) (*rpc.GetIdentityResult, error)
	})
	if !ok {
		return "", fmt.Errorf("local RPC client does not support getIdentity")
	}
	result, err := identityClient.GetIdentity(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get local identity: %w", err)
	}
	return result.Identity.String(), nil
}

// GetVoteAccountState returns one vote account from either local or network RPC.
func (c *Client) GetVoteAccountState(ctx context.Context, votePubkey string, local bool, commitment rpc.CommitmentType) (*rpc.VoteAccountsResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	votePubkeyValue, err := solanago.PublicKeyFromBase58(votePubkey)
	if err != nil {
		return nil, fmt.Errorf("invalid vote account pubkey %q: %w", votePubkey, err)
	}
	client := c.networkRPCClient
	if local {
		client = c.localRPCClient
	}
	accounts, err := client.GetVoteAccounts(ctx, &rpc.GetVoteAccountsOpts{
		Commitment: commitment,
		VotePubkey: &votePubkeyValue,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get vote accounts: %w", err)
	}
	if accounts == nil {
		return nil, fmt.Errorf("vote account RPC returned no result")
	}
	for _, account := range append(accounts.Current, accounts.Delinquent...) {
		if account.VotePubkey.String() == votePubkey {
			return &account, nil
		}
	}
	return nil, fmt.Errorf("vote account not found: %s", votePubkey)
}

// NodeFromIP returns a Node from an IP address
func (c *Client) NodeFromIP(ip string) (*Node, error) {
	gossipNode, err := c.nodeFromIP(ip)
	if err != nil {
		return nil, err
	}
	return &Node{gossipNode: gossipNode}, nil
}

// NodeFromPubkey returns a Node from a pubkey
func (c *Client) NodeFromPubkey(pubkey string) (*Node, error) {
	gossipNode, err := c.gossipNodeFromPubkey(pubkey)
	if err != nil {
		return nil, err
	}
	return &Node{gossipNode: gossipNode}, nil
}

func (c *Client) getClusterNodes() ([]*rpc.GetClusterNodesResult, error) {
	nodes, err := c.localRPCClient.GetClusterNodes(context.Background())
	if err != nil {
		return nil, wrapLocalGetClusterNodesErr(err)
	}
	return nodes, nil
}

func wrapLocalGetClusterNodesErr(err error) error {
	var rpcErr *jsonrpc.RPCError
	if errors.As(err, &rpcErr) && rpcErr.Code == jsonRPCMethodNotFound {
		return fmt.Errorf("%w; start the local validator with --full-rpc-api to enable getClusterNodes", err)
	}
	return err
}

func (c *Client) nodeFromIP(ip string) (node *rpc.GetClusterNodesResult, err error) {
	localNodes, localErr := c.getClusterNodes()
	if localErr == nil {
		if node := findGossipNodeFromIP(localNodes, ip); node != nil {
			c.loggerLocal.Debug("gossip peer found by ip", "ip", ip, "node_count", len(localNodes))
			return node, nil
		}
		c.loggerLocal.Debug("gossip peer not found by ip", "ip", ip, "node_count", len(localNodes))
	} else {
		c.loggerLocal.Debug("failed to query gossip peer by ip", "ip", ip, "err", localErr)
	}

	clusterNodes, clusterErr := c.networkRPCClient.GetClusterNodes(context.Background())
	if clusterErr == nil {
		if node := findGossipNodeFromIP(clusterNodes, ip); node != nil {
			c.loggerNetwork.Debug("gossip peer found by ip", "ip", ip, "node_count", len(clusterNodes))
			return node, nil
		}
		c.loggerNetwork.Debug("gossip peer not found by ip", "ip", ip, "node_count", len(clusterNodes))
	} else {
		c.loggerNetwork.Debug("failed to query gossip peer by ip", "ip", ip, "err", clusterErr)
	}

	return nil, gossipNodeFromIPError(ip, len(localNodes), localErr, len(clusterNodes), clusterErr)
}

func findGossipNodeFromIP(nodes []*rpc.GetClusterNodesResult, ip string) *rpc.GetClusterNodesResult {
	for _, node := range nodes {
		if node != nil && node.Gossip != nil && strings.Split(*node.Gossip, ":")[0] == ip {
			return node
		}
	}
	return nil
}

func (c *Client) gossipNodeFromPubkey(pubkey string) (node *rpc.GetClusterNodesResult, err error) {
	localNodes, localErr := c.getClusterNodes()
	if localErr == nil {
		if node := findGossipNodeFromPubkey(localNodes, pubkey); node != nil {
			c.loggerLocal.Debug("gossip peer found by pubkey", "pubkey", pubkey, "node_count", len(localNodes))
			return node, nil
		}
		c.loggerLocal.Debug("gossip peer not found by pubkey", "pubkey", pubkey, "node_count", len(localNodes))
	} else {
		c.loggerLocal.Debug("failed to query gossip peer by pubkey", "pubkey", pubkey, "err", localErr)
	}

	clusterNodes, clusterErr := c.networkRPCClient.GetClusterNodes(context.Background())
	if clusterErr == nil {
		if node := findGossipNodeFromPubkey(clusterNodes, pubkey); node != nil {
			c.loggerNetwork.Debug("gossip peer found by pubkey", "pubkey", pubkey, "node_count", len(clusterNodes))
			return node, nil
		}
		c.loggerNetwork.Debug("gossip peer not found by pubkey", "pubkey", pubkey, "node_count", len(clusterNodes))
	} else {
		c.loggerNetwork.Debug("failed to query gossip peer by pubkey", "pubkey", pubkey, "err", clusterErr)
	}

	return nil, gossipNodeFromPubkeyError(pubkey, len(localNodes), localErr, len(clusterNodes), clusterErr)
}

func findGossipNodeFromPubkey(nodes []*rpc.GetClusterNodesResult, pubkey string) *rpc.GetClusterNodesResult {
	for _, node := range nodes {
		if node != nil && node.Pubkey.String() == pubkey {
			return node
		}
	}
	return nil
}

func gossipNodeFromPubkeyError(pubkey string, localCount int, localErr error, clusterCount int, clusterErr error) error {
	localResult := fmt.Sprintf("returned %d nodes without a match", localCount)
	if localErr != nil {
		localResult = fmt.Sprintf("failed: %v", localErr)
	}
	clusterResult := fmt.Sprintf("returned %d nodes without a match", clusterCount)
	if clusterErr != nil {
		clusterResult = fmt.Sprintf("failed: %v", clusterErr)
	}
	return fmt.Errorf(
		"gossip node not found for pubkey: %s (local RPC %s; cluster RPC %s)",
		pubkey,
		localResult,
		clusterResult,
	)
}

func gossipNodeFromIPError(ip string, localCount int, localErr error, clusterCount int, clusterErr error) error {
	localResult := fmt.Sprintf("returned %d nodes without a match", localCount)
	if localErr != nil {
		localResult = fmt.Sprintf("failed: %v", localErr)
	}
	clusterResult := fmt.Sprintf("returned %d nodes without a match", clusterCount)
	if clusterErr != nil {
		clusterResult = fmt.Sprintf("failed: %v", clusterErr)
	}
	return fmt.Errorf(
		"gossip node not found for ip: %s (local RPC %s; cluster RPC %s)",
		ip,
		localResult,
		clusterResult,
	)
}

// NodeFromIPWithExpectedPubkey returns a Node from an IP address, preferring the entry
// whose pubkey matches expectedPubkey. During a gossip identity transition, both the old
// and new CRDS entries for a node briefly coexist; this method returns the expected entry
// if present, falling back to the first entry for the IP otherwise. Returns an error only
// if no entry exists for the IP at all.
func (c *Client) NodeFromIPWithExpectedPubkey(ip, expectedPubkey string) (*Node, error) {
	gossipNode, err := c.nodeFromIPWithExpectedPubkey(ip, expectedPubkey)
	if err != nil {
		return nil, err
	}
	return &Node{gossipNode: gossipNode}, nil
}

func (c *Client) nodeFromIPWithExpectedPubkey(ip, expectedPubkey string) (*rpc.GetClusterNodesResult, error) {
	localNodes, localErr := c.getClusterNodes()
	var localFirstMatch *rpc.GetClusterNodesResult
	if localErr == nil {
		localExactMatch, firstMatch := findGossipNodeFromIPWithExpectedPubkey(localNodes, ip, expectedPubkey)
		if localExactMatch != nil {
			c.loggerLocal.Debug("gossip peer found by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "node_count", len(localNodes))
			return localExactMatch, nil
		}
		localFirstMatch = firstMatch
		c.loggerLocal.Debug("gossip peer not found by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "node_count", len(localNodes), "ip_match", firstMatch != nil)
	} else {
		c.loggerLocal.Debug("failed to query gossip peer by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "err", localErr)
	}

	clusterNodes, clusterErr := c.networkRPCClient.GetClusterNodes(context.Background())
	var clusterFirstMatch *rpc.GetClusterNodesResult
	if clusterErr == nil {
		clusterExactMatch, firstMatch := findGossipNodeFromIPWithExpectedPubkey(clusterNodes, ip, expectedPubkey)
		if clusterExactMatch != nil {
			c.loggerNetwork.Debug("gossip peer found by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "node_count", len(clusterNodes))
			return clusterExactMatch, nil
		}
		clusterFirstMatch = firstMatch
		c.loggerNetwork.Debug("gossip peer not found by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "node_count", len(clusterNodes), "ip_match", firstMatch != nil)
	} else {
		c.loggerNetwork.Debug("failed to query gossip peer by ip and expected pubkey", "ip", ip, "pubkey", expectedPubkey, "err", clusterErr)
	}

	if localFirstMatch != nil {
		c.loggerLocal.Debug("using stale gossip peer found by ip", "ip", ip, "expected_pubkey", expectedPubkey, "actual_pubkey", localFirstMatch.Pubkey.String())
		return localFirstMatch, nil
	}
	if clusterFirstMatch != nil {
		c.loggerNetwork.Debug("using stale gossip peer found by ip", "ip", ip, "expected_pubkey", expectedPubkey, "actual_pubkey", clusterFirstMatch.Pubkey.String())
		return clusterFirstMatch, nil
	}

	return nil, gossipNodeFromIPError(ip, len(localNodes), localErr, len(clusterNodes), clusterErr)
}

func findGossipNodeFromIPWithExpectedPubkey(nodes []*rpc.GetClusterNodesResult, ip, expectedPubkey string) (exactMatch, firstMatch *rpc.GetClusterNodesResult) {
	for _, node := range nodes {
		if node == nil || node.Gossip == nil {
			continue
		}
		gossipIP := strings.Split(*node.Gossip, ":")[0]
		if gossipIP != ip {
			continue
		}
		// Found a node at this IP — prefer the one with the expected pubkey
		if node.Pubkey.String() == expectedPubkey {
			return node, firstMatch
		}
		if firstMatch == nil {
			firstMatch = node // stale entry — keep as fallback
		}
	}
	return nil, firstMatch
}

// GetCreditRankedVoteAccountFromPubkey returns the credit rank-sorted current vote accounts rank is the difference
// between current epoch credits and total credits (descending)
func (c *Client) GetCreditRankedVoteAccountFromPubkey(pubkey string) (voteAccount *rpc.VoteAccountsResult, creditRank int, err error) {
	return c.getCreditRankedVoteAccount(context.Background(), pubkey, false)
}

// GetCreditRankedVoteAccountFromPubkeyContext is the context-aware version of
// GetCreditRankedVoteAccountFromPubkey used by time-bounded handoff setup.
func (c *Client) GetCreditRankedVoteAccountFromPubkeyContext(ctx context.Context, pubkey string) (voteAccount *rpc.VoteAccountsResult, creditRank int, err error) {
	return c.getCreditRankedVoteAccount(ctx, pubkey, true)
}

func (c *Client) getCreditRankedVoteAccount(ctx context.Context, pubkey string, includeDelinquent bool) (voteAccount *rpc.VoteAccountsResult, creditRank int, err error) {
	// fetch all vote accounts
	voteAccounts, err := c.networkRPCClient.GetVoteAccounts(
		ctx,
		&rpc.GetVoteAccountsOpts{
			Commitment: rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get vote account from pubkey %s: %w", pubkey, err)
	}

	if includeDelinquent {
		// A native validator can be delinquent while still having a valid vote
		// account, so discovery must search both RPC result sets. Multiple
		// matches are unsafe because the metrics watermark identifies neither
		// account, so force the operator to configure vote_account explicitly.
		matches := make([]rpc.VoteAccountsResult, 0, 2)
		for _, account := range append(voteAccounts.Current, voteAccounts.Delinquent...) {
			if account.NodePubkey.String() == pubkey {
				matches = append(matches, account)
			}
		}
		if len(matches) != 1 {
			return nil, 0, fmt.Errorf("expected exactly one vote account for node %s, found %d; configure validator.client.vote_account explicitly", pubkey, len(matches))
		}
		return &matches[0], 0, nil
	}

	// Existing credit-ranking callers intentionally rank current validators
	// only; delinquent accounts must not change those semantics.
	allVoteAccounts := voteAccounts.Current

	// Sort current validators by the difference between current epoch credits
	// and total credits (descending).
	sort.SliceStable(allVoteAccounts, func(i, j int) bool {
		// calculate the difference between current epoch credits and total credits
		var iDiff, jDiff int64
		if len(allVoteAccounts[i].EpochCredits) > 0 {
			lastIndex := len(allVoteAccounts[i].EpochCredits) - 1
			currentCredits := allVoteAccounts[i].EpochCredits[lastIndex][1]
			totalCredits := allVoteAccounts[i].EpochCredits[lastIndex][2]
			iDiff = currentCredits - totalCredits
		}
		if len(allVoteAccounts[j].EpochCredits) > 0 {
			lastIndex := len(allVoteAccounts[j].EpochCredits) - 1
			currentCredits := allVoteAccounts[j].EpochCredits[lastIndex][1]
			totalCredits := allVoteAccounts[j].EpochCredits[lastIndex][2]
			jDiff = currentCredits - totalCredits
		}
		return iDiff > jDiff
	})

	for i, account := range allVoteAccounts {
		if account.NodePubkey.String() == pubkey {
			creditRank = i + 1 // rank is 1-indexed
			return &account, creditRank, nil
		}
	}

	return nil, 0, fmt.Errorf("vote account not found for pubkey: %s", pubkey)
}

// GetCurrentSlot returns the current slot
func (c *Client) GetCurrentSlot() (slot uint64, err error) {
	slot, err = c.networkRPCClient.GetSlot(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return 0, fmt.Errorf("failed to get slot: %w", err)
	}
	return slot, nil
}

// GetTimeToNextLeaderSlotForPubkey returns the time to the next leader slot for the given pubkey
func (c *Client) GetTimeToNextLeaderSlotForPubkey(pubkey solanago.PublicKey) (isOnLeaderSchedule bool, timeToNextLeaderSlot time.Duration, err error) {
	// get epoch information, includes the current slot (absolute slot) and its offset from the first slot of the epoch
	epochInfo, err := c.networkRPCClient.GetEpochInfo(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return false, time.Duration(0), fmt.Errorf("failed to get epoch info: %w", err)
	}

	c.loggerNetwork.Debug("Epoch info retrieved",
		"epoch", epochInfo.Epoch,
		"slotIndex", epochInfo.SlotIndex,
		"absoluteSlot", epochInfo.AbsoluteSlot,
	)

	// get the leader schedule - returns a map of pubkey:[]uint64 - where values are a slice of slot indexes
	// relaative to the first slot of epochInfo result
	leaderSchedule, err := c.networkRPCClient.GetLeaderSchedule(context.Background())
	if err != nil {
		return false, time.Duration(0), fmt.Errorf("failed to get leader schedule: %w", err)
	}

	// get current epoch leader slot indexes for the pubkey
	leaderSlotIndexes, ok := leaderSchedule[pubkey]

	// pubkey not in leader schedule
	if !ok {
		c.loggerNetwork.Debug("Pubkey not found in leader schedule", "pubkey", pubkey.String())
		return false, time.Duration(0), nil
	}

	c.loggerNetwork.Debug("Found slots in leader schedule", "pubkey", pubkey.String(), "slotsCount", len(leaderSlotIndexes))

	// calculate the first slot of the epoch (zero-based indexing)
	// epochInfo.AbsoluteSlot is the "current" slot for when we fetched the epoch info
	firstSlotOfEpoch := epochInfo.AbsoluteSlot - epochInfo.SlotIndex
	var nextLeaderSlot uint64

	// Find the next future leader schedule slot for pubkey - leaderSlotIndex is relative to the first slot of the epoch
	for _, leaderSlotIndex := range leaderSlotIndexes {
		leaderSlot := firstSlotOfEpoch + leaderSlotIndex

		c.loggerNetwork.Debug("Checking slot",
			"leaderSlotIndex", leaderSlotIndex,
			"leaderSlot", leaderSlot,
			"currentSlot", epochInfo.AbsoluteSlot,
			"pubkey", pubkey.String(),
		)

		if leaderSlot >= epochInfo.AbsoluteSlot {
			nextLeaderSlot = leaderSlot
			break
		}
	}

	// didn't find future slots for the pubkey
	if nextLeaderSlot == 0 {
		c.loggerNetwork.Debug("No future leader slots found for pubkey", "pubkey", pubkey.String())
		return false, time.Duration(0), nil
	}

	// Calculate time to next leader slot using slot difference and average slot time
	slotDifference := nextLeaderSlot - epochInfo.AbsoluteSlot
	timeToNextLeaderSlot = time.Duration(slotDifference) * c.averageSlotDuration

	c.loggerNetwork.Debug("Next leader slot",
		"duration", timeToNextLeaderSlot.String(),
		"nextLeaderSlot", nextLeaderSlot,
		"currentSlot", epochInfo.AbsoluteSlot,
	)

	return true, timeToNextLeaderSlot, nil
}
