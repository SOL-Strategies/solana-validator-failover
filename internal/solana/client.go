package solana

import (
	"context"
	"errors"
	"fmt"
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
}

// Client implements Interface using an RPC client
type Client struct {
	localRPCClient      RPCClientInterface
	networkRPCClient    RPCClientInterface
	loggerLocal         *log.Logger
	loggerNetwork       *log.Logger
	averageSlotDuration time.Duration
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
	}
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
		return nil, wrapGetClusterNodesErr(err)
	}
	return nodes, nil
}

func wrapGetClusterNodesErr(err error) error {
	var rpcErr *jsonrpc.RPCError
	if errors.As(err, &rpcErr) && rpcErr.Code == jsonRPCMethodNotFound {
		return fmt.Errorf("%w; start the local validator with --full-rpc-api to enable getClusterNodes", err)
	}
	return err
}

func (c *Client) nodeFromIP(ip string) (node *rpc.GetClusterNodesResult, err error) {
	nodes, err := c.getClusterNodes()
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Gossip != nil {
			gossipIP := strings.Split(*node.Gossip, ":")[0]
			if gossipIP == ip {
				return node, nil
			}
		}
	}

	return nil, fmt.Errorf("gossip node not found for ip: %s", ip)
}

func (c *Client) gossipNodeFromPubkey(pubkey string) (node *rpc.GetClusterNodesResult, err error) {
	nodes, err := c.getClusterNodes()
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Pubkey.String() == pubkey {
			return node, nil
		}
	}

	return nil, fmt.Errorf("gossip node not found for pubkey: %s", pubkey)
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
	nodes, err := c.getClusterNodes()
	if err != nil {
		return nil, err
	}

	var firstMatch *rpc.GetClusterNodesResult
	for _, node := range nodes {
		if node.Gossip == nil {
			continue
		}
		gossipIP := strings.Split(*node.Gossip, ":")[0]
		if gossipIP != ip {
			continue
		}
		// Found a node at this IP — prefer the one with the expected pubkey
		if node.Pubkey.String() == expectedPubkey {
			return node, nil
		}
		if firstMatch == nil {
			firstMatch = node // stale entry — keep as fallback
		}
	}

	if firstMatch != nil {
		return firstMatch, nil
	}
	return nil, fmt.Errorf("gossip node not found for ip: %s", ip)
}

// GetCreditRankedVoteAccountFromPubkey returns the credit rank-sorted current vote accounts rank is the difference
// between current epoch credits and total credits (descending)
func (c *Client) GetCreditRankedVoteAccountFromPubkey(pubkey string) (voteAccount *rpc.VoteAccountsResult, creditRank int, err error) {
	// fetch all vote accounts
	voteAccounts, err := c.networkRPCClient.GetVoteAccounts(
		context.Background(),
		&rpc.GetVoteAccountsOpts{
			Commitment: rpc.CommitmentConfirmed,
		},
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get vote account from pubkey %s: %w", pubkey, err)
	}

	// select current (non-delinquent) vote accounts
	currentVoteAccounts := voteAccounts.Current

	// sort validators by the difference between current epoch credits and total credits (descending)
	sort.SliceStable(currentVoteAccounts, func(i, j int) bool {
		// calculate the difference between current epoch credits and total credits
		var iDiff, jDiff int64
		if len(currentVoteAccounts[i].EpochCredits) > 0 {
			lastIndex := len(currentVoteAccounts[i].EpochCredits) - 1
			currentCredits := currentVoteAccounts[i].EpochCredits[lastIndex][1]
			totalCredits := currentVoteAccounts[i].EpochCredits[lastIndex][2]
			iDiff = currentCredits - totalCredits
		}
		if len(currentVoteAccounts[j].EpochCredits) > 0 {
			lastIndex := len(currentVoteAccounts[j].EpochCredits) - 1
			currentCredits := currentVoteAccounts[j].EpochCredits[lastIndex][1]
			totalCredits := currentVoteAccounts[j].EpochCredits[lastIndex][2]
			jDiff = currentCredits - totalCredits
		}
		return iDiff > jDiff
	})

	for i, account := range currentVoteAccounts {
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
