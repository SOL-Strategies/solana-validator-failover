package solana

import (
	"errors"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// MockClient is a mock implementation of ClientInterface for testing
type MockClient struct {
	// Node management
	mockNode       *Node
	nodeFromIP     func(ip string) (*Node, error)
	nodeFromPubkey func(pubkey string) (*Node, error)

	// Health status
	healthStatus       bool
	getLocalNodeHealth func() (string, error)
	isLocalNodeHealthy func() bool

	// Vote account methods
	getCreditRankedVoteAccountFromPubkey func(pubkey string) (*rpc.VoteAccountsResult, int, error)

	// Slot methods
	getCurrentSlot func() (uint64, error)

	// Leader schedule methods
	getTimeToNextLeaderSlotForPubkey func(pubkey solana.PublicKey) (bool, time.Duration, error)
}

// NewMockClient creates a new mock client with default behaviors
func NewMockClient() *MockClient {
	return &MockClient{
		healthStatus: true,
		mockNode: &Node{
			gossipNode: &rpc.GetClusterNodesResult{
				Pubkey:  solana.MustPublicKeyFromBase58("11111111111111111111111111111111"),
				Gossip:  stringPtr("192.168.1.100:8001"),
				Version: stringPtr("1.16.0"),
			},
		},
	}
}

// WithNodeFromIP sets a custom NodeFromIP function
func (m *MockClient) WithNodeFromIP(fn func(ip string) (*Node, error)) *MockClient {
	m.nodeFromIP = fn
	return m
}

// WithNodeFromPubkey sets a custom NodeFromPubkey function
func (m *MockClient) WithNodeFromPubkey(fn func(pubkey string) (*Node, error)) *MockClient {
	m.nodeFromPubkey = fn
	return m
}

// WithHealthStatus sets the health status
func (m *MockClient) WithHealthStatus(healthy bool) *MockClient {
	m.healthStatus = healthy
	return m
}

// WithGetLocalNodeHealth sets a custom GetLocalNodeHealth function
func (m *MockClient) WithGetLocalNodeHealth(fn func() (string, error)) *MockClient {
	m.getLocalNodeHealth = fn
	return m
}

// WithIsLocalNodeHealthy sets a custom IsLocalNodeHealthy function
func (m *MockClient) WithIsLocalNodeHealthy(fn func() bool) *MockClient {
	m.isLocalNodeHealthy = fn
	return m
}

// WithGetCreditRankedVoteAccountFromPubkey sets a custom GetCreditRankedVoteAccountFromPubkey function
func (m *MockClient) WithGetCreditRankedVoteAccountFromPubkey(fn func(pubkey string) (*rpc.VoteAccountsResult, int, error)) *MockClient {
	m.getCreditRankedVoteAccountFromPubkey = fn
	return m
}

// WithGetCurrentSlot sets a custom GetCurrentSlot function
func (m *MockClient) WithGetCurrentSlot(fn func() (uint64, error)) *MockClient {
	m.getCurrentSlot = fn
	return m
}


// WithGetTimeToNextLeaderSlotForPubkey sets a custom GetTimeToNextLeaderSlotForPubkey function
func (m *MockClient) WithGetTimeToNextLeaderSlotForPubkey(fn func(pubkey solana.PublicKey) (bool, time.Duration, error)) *MockClient {
	m.getTimeToNextLeaderSlotForPubkey = fn
	return m
}

// WithMockNode sets the mock node
func (m *MockClient) WithMockNode(node *Node) *MockClient {
	m.mockNode = node
	return m
}

// NodeFromIP implements ClientInterface.NodeFromIP
func (m *MockClient) NodeFromIP(ip string) (*Node, error) {
	if m.nodeFromIP != nil {
		return m.nodeFromIP(ip)
	}
	return m.mockNode, nil
}

// NodeFromPubkey implements ClientInterface.NodeFromPubkey
func (m *MockClient) NodeFromPubkey(pubkey string) (*Node, error) {
	if m.nodeFromPubkey != nil {
		return m.nodeFromPubkey(pubkey)
	}
	return m.mockNode, nil
}

// GetCreditRankedVoteAccountFromPubkey implements ClientInterface.GetCreditRankedVoteAccountFromPubkey
func (m *MockClient) GetCreditRankedVoteAccountFromPubkey(pubkey string) (*rpc.VoteAccountsResult, int, error) {
	if m.getCreditRankedVoteAccountFromPubkey != nil {
		return m.getCreditRankedVoteAccountFromPubkey(pubkey)
	}
	return nil, 0, nil
}

// GetCurrentSlot implements ClientInterface.GetCurrentSlot
func (m *MockClient) GetCurrentSlot() (uint64, error) {
	if m.getCurrentSlot != nil {
		return m.getCurrentSlot()
	}
	return 0, nil
}


// GetTimeToNextLeaderSlotForPubkey implements ClientInterface.GetTimeToNextLeaderSlotForPubkey
func (m *MockClient) GetTimeToNextLeaderSlotForPubkey(pubkey solana.PublicKey) (bool, time.Duration, error) {
	if m.getTimeToNextLeaderSlotForPubkey != nil {
		return m.getTimeToNextLeaderSlotForPubkey(pubkey)
	}
	return false, 0, nil
}

// GetLocalNodeHealth implements ClientInterface.GetLocalNodeHealth
func (m *MockClient) GetLocalNodeHealth() (string, error) {
	if m.getLocalNodeHealth != nil {
		return m.getLocalNodeHealth()
	}
	if m.healthStatus {
		return "ok", nil
	}
	return "", errors.New("unhealthy")
}

// IsLocalNodeHealthy implements ClientInterface.IsLocalNodeHealthy
func (m *MockClient) IsLocalNodeHealthy() bool {
	if m.isLocalNodeHealthy != nil {
		return m.isLocalNodeHealthy()
	}
	return m.healthStatus
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

// MockClientBuilder provides a fluent interface for building mock clients
type MockClientBuilder struct {
	client *MockClient
}

// NewMockClientBuilder creates a new mock client builder
func NewMockClientBuilder() *MockClientBuilder {
	return &MockClientBuilder{
		client: NewMockClient(),
	}
}

// WithActiveNode configures the mock to simulate an active node
func (b *MockClientBuilder) WithActiveNode(activePubkey string) *MockClientBuilder {
	activeKey := solana.MustPublicKeyFromBase58(activePubkey)
	b.client.mockNode = &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  activeKey,
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}
	return b
}

// WithPassiveNode configures the mock to simulate a passive node
func (b *MockClientBuilder) WithPassiveNode(passivePubkey string) *MockClientBuilder {
	passiveKey := solana.MustPublicKeyFromBase58(passivePubkey)
	b.client.mockNode = &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  passiveKey,
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}
	return b
}

// WithUnhealthyNode configures the mock to simulate an unhealthy node
func (b *MockClientBuilder) WithUnhealthyNode() *MockClientBuilder {
	b.client.healthStatus = false
	return b
}

// WithHealthyNode configures the mock to simulate a healthy node
func (b *MockClientBuilder) WithHealthyNode() *MockClientBuilder {
	b.client.healthStatus = true
	return b
}

// WithVoteAccount configures the mock to return specific vote account data
func (b *MockClientBuilder) WithVoteAccount(pubkey string, rank int, credits int64) *MockClientBuilder {
	b.client.getCreditRankedVoteAccountFromPubkey = func(p string) (*rpc.VoteAccountsResult, int, error) {
		if p == pubkey {
			return &rpc.VoteAccountsResult{
				NodePubkey: solana.MustPublicKeyFromBase58(pubkey),
				EpochCredits: [][]int64{
					{1, credits, credits / 2},
				},
			}, rank, nil
		}
		return nil, 0, errors.New("vote account not found")
	}
	return b
}

// WithLeaderSchedule configures the mock to simulate leader schedule behavior
func (b *MockClientBuilder) WithLeaderSchedule(pubkey string, isOnSchedule bool, timeToNext time.Duration) *MockClientBuilder {
	b.client.getTimeToNextLeaderSlotForPubkey = func(p solana.PublicKey) (bool, time.Duration, error) {
		if p.String() == pubkey {
			return isOnSchedule, timeToNext, nil
		}
		return false, 0, nil
	}
	return b
}

// WithCurrentSlot configures the mock to return a specific current slot
func (b *MockClientBuilder) WithCurrentSlot(slot uint64) *MockClientBuilder {
	b.client.getCurrentSlot = func() (uint64, error) {
		return slot, nil
	}
	return b
}


// Build returns the configured mock client
func (b *MockClientBuilder) Build() *MockClient {
	return b.client
}

// NewMockNode creates a new mock node for testing
func NewMockNode(pubkey solana.PublicKey, version string) *Node {
	return &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  pubkey,
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr(version),
		},
	}
}
