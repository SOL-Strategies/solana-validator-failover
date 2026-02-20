package solana

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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
	localRPCClient   RPCClientInterface
	networkRPCClient RPCClientInterface
	loggerLocal      zerolog.Logger
	loggerNetwork    zerolog.Logger
	averageSlotTime  time.Duration
}

// NewClientParams is the parameters for creating a new client
type NewClientParams struct {
	LocalRPCURL     string
	ClusterRPCURL   string
	AverageSlotTime int // average slot time in milliseconds, defaults to 400
}

// NewRPCClient creates a new client for the given solana cluster
func NewRPCClient(params NewClientParams) ClientInterface {
	avgSlotTime := params.AverageSlotTime
	if avgSlotTime <= 0 {
		avgSlotTime = 400
	}
	return &Client{
		localRPCClient:   rpc.New(params.LocalRPCURL),
		networkRPCClient: rpc.New(params.ClusterRPCURL),
		loggerLocal:      log.Logger.With().Str("rpc_client", "local").Logger(),
		loggerNetwork:    log.Logger.With().Str("rpc_client", "network").Logger(),
		averageSlotTime:  time.Duration(avgSlotTime) * time.Millisecond,
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
		c.loggerLocal.Debug().Err(err).Msg("failed to get local node health")
		return false
	}
	isHealthy := result == rpc.HealthOk
	if !isHealthy {
		c.loggerLocal.Debug().Str("result", result).Msg("local node health")
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

func (c *Client) nodeFromIP(ip string) (node *rpc.GetClusterNodesResult, err error) {
	nodes, err := c.networkRPCClient.GetClusterNodes(context.Background())
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
	nodes, err := c.networkRPCClient.GetClusterNodes(context.Background())
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

	c.loggerNetwork.Debug().
		Uint64("epoch", epochInfo.Epoch).
		Uint64("slotIndex", epochInfo.SlotIndex).
		Uint64("absoluteSlot", epochInfo.AbsoluteSlot).
		Msg("Epoch info retrieved")

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
		c.loggerNetwork.Debug().Str("pubkey", pubkey.String()).Msg("Pubkey not found in leader schedule")
		return false, time.Duration(0), nil
	}

	c.loggerNetwork.Debug().Str("pubkey", pubkey.String()).Int("slotsCount", len(leaderSlotIndexes)).Msg("Found slots in leader schedule")

	// calculate the first slot of the epoch (zero-based indexing)
	// epochInfo.AbsoluteSlot is the "current" slot for when we fetched the epoch info
	firstSlotOfEpoch := epochInfo.AbsoluteSlot - epochInfo.SlotIndex
	var nextLeaderSlot uint64

	// Find the next future leader schedule slot for pubkey - leaderSlotIndex is relative to the first slot of the epoch
	for _, leaderSlotIndex := range leaderSlotIndexes {
		leaderSlot := firstSlotOfEpoch + leaderSlotIndex

		c.loggerNetwork.Debug().
			Uint64("leaderSlotIndex", leaderSlotIndex).
			Uint64("leaderSlot", leaderSlot).
			Uint64("currentSlot", epochInfo.AbsoluteSlot).
			Str("pubkey", pubkey.String()).
			Msg("Checking slot")

		if leaderSlot >= epochInfo.AbsoluteSlot {
			nextLeaderSlot = leaderSlot
			break
		}
	}

	// didn't find future slots for the pubkey
	if nextLeaderSlot == 0 {
		c.loggerNetwork.Debug().Str("pubkey", pubkey.String()).Msg("No future leader slots found for pubkey")
		return false, time.Duration(0), nil
	}

	// Calculate time to next leader slot using slot difference and average slot time
	slotDifference := nextLeaderSlot - epochInfo.AbsoluteSlot
	timeToNextLeaderSlot = time.Duration(slotDifference) * c.averageSlotTime

	c.loggerNetwork.Debug().
		Uint64("nextLeaderSlot", nextLeaderSlot).
		Uint64("currentSlot", epochInfo.AbsoluteSlot).
		Msgf("Next leader slot in %s", timeToNextLeaderSlot.String())

	return true, timeToNextLeaderSlot, nil
}
