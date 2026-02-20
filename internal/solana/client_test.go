package solana

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockRPCClient is a mock implementation of the RPC client interface
type MockRPCClient struct {
	mock.Mock
}

func (m *MockRPCClient) GetClusterNodes(ctx context.Context) ([]*rpc.GetClusterNodesResult, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*rpc.GetClusterNodesResult), args.Error(1)
}

func (m *MockRPCClient) GetVoteAccounts(ctx context.Context, opts *rpc.GetVoteAccountsOpts) (*rpc.GetVoteAccountsResult, error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*rpc.GetVoteAccountsResult), args.Error(1)
}

func (m *MockRPCClient) GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	args := m.Called(ctx, commitment)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockRPCClient) GetLeaderSchedule(ctx context.Context) (rpc.GetLeaderScheduleResult, error) {
	args := m.Called(ctx)
	return args.Get(0).(rpc.GetLeaderScheduleResult), args.Error(1)
}

func (m *MockRPCClient) GetHealth(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.Get(0).(string), args.Error(1)
}

func (m *MockRPCClient) GetEpochInfo(ctx context.Context, commitment rpc.CommitmentType) (*rpc.GetEpochInfoResult, error) {
	args := m.Called(ctx, commitment)
	return args.Get(0).(*rpc.GetEpochInfoResult), args.Error(1)
}

// createTestClient creates a test client with mock RPC clients
func createTestClient() (*Client, *MockRPCClient, *MockRPCClient) {
	localMock := &MockRPCClient{}
	networkMock := &MockRPCClient{}

	client := &Client{
		localRPCClient:   localMock,
		networkRPCClient: networkMock,
		averageSlotTime:  400 * time.Millisecond,
	}

	return client, localMock, networkMock
}

func TestNewRPCClient(t *testing.T) {
	params := NewClientParams{
		LocalRPCURL:   "http://localhost:8899",
		ClusterRPCURL: "https://api.mainnet-beta.solana.com",
	}
	client := NewRPCClient(params)

	assert.NotNil(t, client)
	assert.IsType(t, &Client{}, client)
	// Default average slot time should be 400ms
	assert.Equal(t, 400*time.Millisecond, client.(*Client).averageSlotTime)
}

func TestNewRPCClient_CustomAverageSlotTime(t *testing.T) {
	params := NewClientParams{
		LocalRPCURL:     "http://localhost:8899",
		ClusterRPCURL:   "https://api.mainnet-beta.solana.com",
		AverageSlotTime: 200,
	}
	client := NewRPCClient(params)

	assert.NotNil(t, client)
	assert.Equal(t, 200*time.Millisecond, client.(*Client).averageSlotTime)
}

func TestGossipClient_NodeFromIP_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			TPU:     stringPtr("192.168.1.100:8002"),
			Version: stringPtr("1.16.0"),
		},
		{
			Pubkey:  createTestPublicKey(2),
			Gossip:  stringPtr("192.168.1.101:8001"),
			TPU:     stringPtr("192.168.1.101:8002"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	// Test the function
	node, err := client.NodeFromIP("192.168.1.100")

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, "192.168.1.100", node.IP())
	assert.Equal(t, "11111111111111111111111111111111", node.PubKey())
	assert.Equal(t, "1.16.0", node.Version())

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromIP_NotFound(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			TPU:     stringPtr("192.168.1.100:8002"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	// Test the function
	node, err := client.NodeFromIP("192.168.1.999")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "gossip node not found for ip: 192.168.1.999")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromIP_RPCError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	networkMock.On("GetClusterNodes", mock.Anything).Return([]*rpc.GetClusterNodesResult{}, errors.New("RPC connection failed"))

	// Test the function
	node, err := client.NodeFromIP("192.168.1.100")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "RPC connection failed")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromIP_NilGossip(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations - node with nil gossip
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  nil,
			TPU:     stringPtr("192.168.1.100:8002"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	// Test the function
	node, err := client.NodeFromIP("192.168.1.100")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "gossip node not found for ip: 192.168.1.100")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromPubkey_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			TPU:     stringPtr("192.168.1.100:8002"),
			Version: stringPtr("1.16.0"),
		},
		{
			Pubkey:  createTestPublicKey(2),
			Gossip:  stringPtr("192.168.1.101:8001"),
			TPU:     stringPtr("192.168.1.101:8002"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	// Test the function
	node, err := client.NodeFromPubkey("11111111111111111111111111111111")

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, "192.168.1.100", node.IP())
	assert.Equal(t, "11111111111111111111111111111111", node.PubKey())
	assert.Equal(t, "1.16.0", node.Version())

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromPubkey_NotFound(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			TPU:     stringPtr("192.168.1.100:8002"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	// Test the function
	node, err := client.NodeFromPubkey("9999999999999999999999999999999999999999999999999999999999999999")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "gossip node not found for pubkey: 9999999999999999999999999999999999999999999999999999999999999999")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_NodeFromPubkey_RPCError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	networkMock.On("GetClusterNodes", mock.Anything).Return([]*rpc.GetClusterNodesResult{}, errors.New("RPC connection failed"))

	// Test the function
	node, err := client.NodeFromPubkey("11111111111111111111111111111111")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "RPC connection failed")

	networkMock.AssertExpectations(t)
}

func TestNode_IP(t *testing.T) {
	// Create a node with gossip address
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Gossip: stringPtr("192.168.1.100:8001"),
		},
	}

	// Test IP extraction
	assert.Equal(t, "192.168.1.100", node.IP())
}

func TestNode_IP_WithPort(t *testing.T) {
	// Create a node with gossip address that has port
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Gossip: stringPtr("10.0.0.1:12345"),
		},
	}

	// Test IP extraction
	assert.Equal(t, "10.0.0.1", node.IP())
}

func TestNode_Pubkey(t *testing.T) {
	// Create a node with pubkey
	pubkey := createTestPublicKey(1)
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey: pubkey,
		},
	}

	// Test pubkey extraction
	assert.Equal(t, "11111111111111111111111111111111", node.PubKey())
}

func TestNode_Version(t *testing.T) {
	// Create a node with version
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Version: stringPtr("1.16.0"),
		},
	}

	// Test version extraction
	assert.Equal(t, "1.16.0", node.Version())
}

func TestNode_Refresh_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Create initial node
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}

	// Setup mock expectations for refresh
	updatedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.17.0"), // Updated version
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(updatedNodes, nil)

	// Test refresh
	err := node.Refresh(client)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, "1.17.0", node.Version()) // Should have updated version

	networkMock.AssertExpectations(t)
}

func TestNode_Refresh_Error(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Create initial node
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}

	// Setup mock expectations for refresh failure
	networkMock.On("GetClusterNodes", mock.Anything).Return([]*rpc.GetClusterNodesResult{}, errors.New("RPC connection failed"))

	// Test refresh
	err := node.Refresh(client)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RPC connection failed")

	networkMock.AssertExpectations(t)
}

func TestNode_Refresh_NodeNotFound(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Create initial node
	node := &Node{
		gossipNode: &rpc.GetClusterNodesResult{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}

	// Setup mock expectations - return different nodes
	updatedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(2),
			Gossip:  stringPtr("192.168.1.101:8001"),
			Version: stringPtr("1.17.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(updatedNodes, nil)

	// Test refresh
	err := node.Refresh(client)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gossip node not found for ip: 192.168.1.100")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCreditRankedVoteAccountFromPubkey_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedVoteAccounts := &rpc.GetVoteAccountsResult{
		Current: []rpc.VoteAccountsResult{
			{
				NodePubkey: createTestPublicKey(1),
				EpochCredits: [][]int64{
					{1, 1000, 500}, // epoch, current credits, total credits
				},
			},
			{
				NodePubkey: createTestPublicKey(2),
				EpochCredits: [][]int64{
					{1, 800, 400}, // lower credits, should be ranked lower
				},
			},
		},
	}

	networkMock.On("GetVoteAccounts", mock.Anything, mock.Anything).Return(expectedVoteAccounts, nil)

	// Test the function
	voteAccount, rank, err := client.GetCreditRankedVoteAccountFromPubkey("11111111111111111111111111111111")

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, voteAccount)
	assert.Equal(t, 1, rank) // Should be ranked first due to higher credits
	assert.Equal(t, "11111111111111111111111111111111", voteAccount.NodePubkey.String())

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCreditRankedVoteAccountFromPubkey_NotFound(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedVoteAccounts := &rpc.GetVoteAccountsResult{
		Current: []rpc.VoteAccountsResult{
			{
				NodePubkey: createTestPublicKey(1),
				EpochCredits: [][]int64{
					{1, 1000, 500},
				},
			},
		},
	}

	networkMock.On("GetVoteAccounts", mock.Anything, mock.Anything).Return(expectedVoteAccounts, nil)

	// Test the function
	voteAccount, rank, err := client.GetCreditRankedVoteAccountFromPubkey("9999999999999999999999999999999999999999999999999999999999999999")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, voteAccount)
	assert.Equal(t, 0, rank)
	assert.Contains(t, err.Error(), "vote account not found for pubkey: 9999999999999999999999999999999999999999999999999999999999999999")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCreditRankedVoteAccountFromPubkey_RPCError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	networkMock.On("GetVoteAccounts", mock.Anything, mock.Anything).Return((*rpc.GetVoteAccountsResult)(nil), errors.New("RPC connection failed"))

	// Test the function
	voteAccount, rank, err := client.GetCreditRankedVoteAccountFromPubkey("11111111111111111111111111111111")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, voteAccount)
	assert.Equal(t, 0, rank)
	assert.Contains(t, err.Error(), "RPC connection failed")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCreditRankedVoteAccountFromPubkey_Sorting(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations with multiple vote accounts to test sorting
	expectedVoteAccounts := &rpc.GetVoteAccountsResult{
		Current: []rpc.VoteAccountsResult{
			{
				NodePubkey: createTestPublicKey(1),
				EpochCredits: [][]int64{
					{1, 500, 1000}, // diff: 500 - 1000 = -500
				},
			},
			{
				NodePubkey: createTestPublicKey(2),
				EpochCredits: [][]int64{
					{1, 800, 400}, // diff: 800 - 400 = 400 (highest, should be rank 1)
				},
			},
			{
				NodePubkey: createTestPublicKey(3),
				EpochCredits: [][]int64{
					{1, 600, 300}, // diff: 600 - 300 = 300 (should be rank 2)
				},
			},
		},
	}

	networkMock.On("GetVoteAccounts", mock.Anything, mock.Anything).Return(expectedVoteAccounts, nil)

	// Test ranking for the highest credit difference
	voteAccount, rank, err := client.GetCreditRankedVoteAccountFromPubkey("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, voteAccount)
	assert.Equal(t, 1, rank) // Should be ranked first due to highest credit difference
	assert.Equal(t, "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", voteAccount.NodePubkey.String())

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCurrentSlot_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	expectedSlot := uint64(123456789)
	networkMock.On("GetSlot", mock.Anything, rpc.CommitmentConfirmed).Return(expectedSlot, nil)

	// Test the function
	slot, err := client.GetCurrentSlot()

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, expectedSlot, slot)

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetCurrentSlot_RPCError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	networkMock.On("GetSlot", mock.Anything, rpc.CommitmentConfirmed).Return(uint64(0), errors.New("RPC connection failed"))

	// Test the function
	slot, err := client.GetCurrentSlot()

	// Assertions
	assert.Error(t, err)
	assert.Equal(t, uint64(0), slot)
	assert.Contains(t, err.Error(), "RPC connection failed")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetLocalNodeHealth_Success(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations
	expectedHealth := "ok"
	localMock.On("GetHealth", mock.Anything).Return(expectedHealth, nil)

	// Test the function
	health, err := client.GetLocalNodeHealth()

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, expectedHealth, health)

	localMock.AssertExpectations(t)
}

func TestGossipClient_GetLocalNodeHealth_Error(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations
	localMock.On("GetHealth", mock.Anything).Return("", errors.New("node unhealthy"))

	// Test the function
	health, err := client.GetLocalNodeHealth()

	// Assertions
	assert.Error(t, err)
	assert.Equal(t, "node unhealthy", health) // The method returns err.Error() as the string
	assert.Contains(t, err.Error(), "failed to get local node health")

	localMock.AssertExpectations(t)
}

func TestGossipClient_GetLocalNodeHealth_UnhealthyResponse(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations - simulate unhealthy node response
	// This would typically be a JSON RPC error, but we're testing the string response
	localMock.On("GetHealth", mock.Anything).Return("", errors.New("node is behind trusted validators"))

	// Test the function
	health, err := client.GetLocalNodeHealth()

	// Assertions
	assert.Error(t, err)
	assert.Equal(t, "node is behind trusted validators", health) // The method returns err.Error() as the string
	assert.Contains(t, err.Error(), "failed to get local node health")

	localMock.AssertExpectations(t)
}

func TestGossipClient_IsLocalNodeHealthy_True(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations - healthy node
	localMock.On("GetHealth", mock.Anything).Return("ok", nil)

	// Test the function
	isHealthy := client.IsLocalNodeHealthy()

	// Assertions
	assert.True(t, isHealthy)

	localMock.AssertExpectations(t)
}

func TestGossipClient_IsLocalNodeHealthy_False(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations - unhealthy node
	localMock.On("GetHealth", mock.Anything).Return("", errors.New("node unhealthy"))

	// Test the function
	isHealthy := client.IsLocalNodeHealthy()

	// Assertions
	assert.False(t, isHealthy)

	localMock.AssertExpectations(t)
}

func TestGossipClient_IsLocalNodeHealthy_NonOkResponse(t *testing.T) {
	// Create test client with mocks
	client, localMock, _ := createTestClient()

	// Setup mock expectations - non-ok response
	localMock.On("GetHealth", mock.Anything).Return("unhealthy", nil)

	// Test the function
	isHealthy := client.IsLocalNodeHealthy()

	// Assertions
	assert.False(t, isHealthy)

	localMock.AssertExpectations(t)
}

// Helper function to create public keys from base58 strings
func mustPublicKeyFromBase58(s string) solana.PublicKey {
	pubkey, err := solana.PublicKeyFromBase58(s)
	if err != nil {
		panic(err)
	}
	return pubkey
}

// Helper function to create valid test public keys
func createTestPublicKey(index int) solana.PublicKey {
	// Create a deterministic test public key based on index
	// Using known valid Solana public keys
	switch index {
	case 1:
		// System Program ID
		return mustPublicKeyFromBase58("11111111111111111111111111111111")
	case 2:
		// Token Program ID
		return mustPublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	default:
		// Associated Token Account Program ID
		return mustPublicKeyFromBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")
	}
}

// Integration test with real RPC client (can be skipped in CI)
func TestIntegration_WithRealRPCClient(t *testing.T) {
	t.Skip("Skipping integration test - requires real RPC endpoint")

	// This test would use a real RPC client to test against an actual Solana cluster
	// Useful for integration testing but should be skipped in CI environments

	// Example:
	// params := NewClientParams{
	//     LocalRPC:   "http://localhost:8899",
	//     NetworkRPC: "https://api.mainnet-beta.solana.com",
	// }
	// client := NewRPCClient(params)
	// node, err := client.NodeFromIP("some-real-ip")
	// require.NoError(t, err)
	// assert.NotNil(t, node)
}

// Benchmark tests
func BenchmarkGossipClient_NodeFromIP(b *testing.B) {
	client, _, networkMock := createTestClient()
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.NodeFromIP("192.168.1.100")
	}
}

func BenchmarkGossipClient_NodeFromPubkey(b *testing.B) {
	client, _, networkMock := createTestClient()
	expectedNodes := []*rpc.GetClusterNodesResult{
		{
			Pubkey:  createTestPublicKey(1),
			Gossip:  stringPtr("192.168.1.100:8001"),
			Version: stringPtr("1.16.0"),
		},
	}

	networkMock.On("GetClusterNodes", mock.Anything).Return(expectedNodes, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.NodeFromPubkey("11111111111111111111111111111111")
	}
}

func BenchmarkGossipClient_GetCreditRankedVoteAccountFromPubkey(b *testing.B) {
	client, _, networkMock := createTestClient()
	expectedVoteAccounts := &rpc.GetVoteAccountsResult{
		Current: []rpc.VoteAccountsResult{
			{
				NodePubkey: createTestPublicKey(1),
				EpochCredits: [][]int64{
					{1, 1000, 500},
				},
			},
			{
				NodePubkey: createTestPublicKey(2),
				EpochCredits: [][]int64{
					{1, 800, 400},
				},
			},
		},
	}

	networkMock.On("GetVoteAccounts", mock.Anything, mock.Anything).Return(expectedVoteAccounts, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = client.GetCreditRankedVoteAccountFromPubkey("11111111111111111111111111111111")
	}
}

func BenchmarkGossipClient_GetCurrentSlot(b *testing.B) {
	client, _, networkMock := createTestClient()
	expectedSlot := uint64(123456789)

	networkMock.On("GetSlot", mock.Anything, rpc.CommitmentConfirmed).Return(expectedSlot, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetCurrentSlot()
	}
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_Success(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	currentSlot := uint64(1000)
	relativeSlot := uint64(50) // relative slot in leader schedule
	pubkey := createTestPublicKey(1)

	// Mock epoch info
	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100, // current slot index within epoch
		AbsoluteSlot: currentSlot,
	}

	leaderSchedule := rpc.GetLeaderScheduleResult{
		pubkey: []uint64{relativeSlot, 150, 250}, // relative slots in leader schedule
	}

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	networkMock.On("GetLeaderSchedule", mock.Anything).Return(leaderSchedule, nil)

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	require.NoError(t, err)
	assert.True(t, isOnSchedule)
	assert.Greater(t, timeToNext, time.Duration(0))
	// Should be approximately 50 slots * 400ms = 20 seconds
	expectedTime := time.Duration(50) * 400 * time.Millisecond
	assert.Equal(t, expectedTime, timeToNext)

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_CustomSlotTime(t *testing.T) {
	// Create test client with custom average slot time
	localMock := &MockRPCClient{}
	networkMock := &MockRPCClient{}
	client := &Client{
		localRPCClient:   localMock,
		networkRPCClient: networkMock,
		averageSlotTime:  200 * time.Millisecond,
	}

	// Setup mock expectations
	currentSlot := uint64(1000)
	relativeSlot := uint64(50)
	pubkey := createTestPublicKey(1)

	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100,
		AbsoluteSlot: currentSlot,
	}

	leaderSchedule := rpc.GetLeaderScheduleResult{
		pubkey: []uint64{relativeSlot, 150, 250},
	}

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	networkMock.On("GetLeaderSchedule", mock.Anything).Return(leaderSchedule, nil)

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	require.NoError(t, err)
	assert.True(t, isOnSchedule)
	assert.Greater(t, timeToNext, time.Duration(0))
	// Should be 50 slots * 200ms = 10 seconds (custom slot time)
	expectedTime := time.Duration(50) * 200 * time.Millisecond
	assert.Equal(t, expectedTime, timeToNext)

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_NotOnSchedule(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	currentSlot := uint64(1000)
	pubkey := createTestPublicKey(1)

	// Mock epoch info
	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100,
		AbsoluteSlot: currentSlot,
	}

	leaderSchedule := rpc.GetLeaderScheduleResult{
		// pubkey not in schedule
	}

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	networkMock.On("GetLeaderSchedule", mock.Anything).Return(leaderSchedule, nil)

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	require.NoError(t, err)
	assert.False(t, isOnSchedule)
	assert.Equal(t, time.Duration(0), timeToNext)

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_NoFutureSlots(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	currentSlot := uint64(1000)
	pubkey := createTestPublicKey(1)

	// Mock epoch info
	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100,
		AbsoluteSlot: currentSlot,
	}

	leaderSchedule := rpc.GetLeaderScheduleResult{
		pubkey: []uint64{50, 80, 90}, // all past slots (relative to current slot 1000)
	}

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	networkMock.On("GetLeaderSchedule", mock.Anything).Return(leaderSchedule, nil)

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	require.NoError(t, err)
	assert.False(t, isOnSchedule)
	assert.Equal(t, time.Duration(0), timeToNext)

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_GetEpochInfoError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	pubkey := createTestPublicKey(1)

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return((*rpc.GetEpochInfoResult)(nil), errors.New("RPC connection failed"))

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	assert.Error(t, err)
	assert.False(t, isOnSchedule)
	assert.Equal(t, time.Duration(0), timeToNext)
	assert.Contains(t, err.Error(), "failed to get epoch info")

	networkMock.AssertExpectations(t)
}

func TestGossipClient_GetTimeToNextLeaderSlotForPubkey_GetLeaderScheduleError(t *testing.T) {
	// Create test client with mocks
	client, _, networkMock := createTestClient()

	// Setup mock expectations
	currentSlot := uint64(1000)
	pubkey := createTestPublicKey(1)

	// Mock epoch info
	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100,
		AbsoluteSlot: currentSlot,
	}

	networkMock.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	networkMock.On("GetLeaderSchedule", mock.Anything).Return(rpc.GetLeaderScheduleResult{}, errors.New("leader schedule not available"))

	// Test the function
	isOnSchedule, timeToNext, err := client.GetTimeToNextLeaderSlotForPubkey(pubkey)

	// Assertions
	assert.Error(t, err)
	assert.False(t, isOnSchedule)
	assert.Equal(t, time.Duration(0), timeToNext)
	assert.Contains(t, err.Error(), "failed to get leader schedule")

	networkMock.AssertExpectations(t)
}

func BenchmarkGossipClient_GetLocalNodeHealth(b *testing.B) {
	client, localMock, _ := createTestClient()
	expectedHealth := "ok"

	localMock.On("GetHealth", mock.Anything).Return(expectedHealth, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetLocalNodeHealth()
	}
}

func BenchmarkGossipClient_GetTimeToNextLeaderSlotForPubkey(b *testing.B) {
	mockClient := &MockRPCClient{}
	currentSlot := uint64(1000)
	relativeSlot := uint64(50)
	pubkey := createTestPublicKey(1)

	// Mock epoch info
	epochInfo := &rpc.GetEpochInfoResult{
		Epoch:        1,
		SlotIndex:    100,
		AbsoluteSlot: currentSlot,
	}

	leaderSchedule := rpc.GetLeaderScheduleResult{
		pubkey: []uint64{relativeSlot, 150, 250},
	}

	mockClient.On("GetEpochInfo", mock.Anything, rpc.CommitmentConfirmed).Return(epochInfo, nil)
	mockClient.On("GetLeaderSchedule", mock.Anything).Return(leaderSchedule, nil)

	gossipClient := NewRPCClient(NewClientParams{
		LocalRPCURL:   "http://localhost:8899",
		ClusterRPCURL: "https://api.mainnet-beta.solana.com",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = gossipClient.GetTimeToNextLeaderSlotForPubkey(pubkey)
	}
}
