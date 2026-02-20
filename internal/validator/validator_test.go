package validator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	solanapkg "github.com/sol-strategies/solana-validator-failover/internal/solana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidator embeds Validator and overrides methods for testing
type TestValidator struct {
	*Validator
	mockPublicIP     string
	mockHostname     string
	mockSolanaClient solanapkg.ClientInterface
}

// GetPublicIP overrides the method for testing
func (tv *TestValidator) GetPublicIP() (string, error) {
	return tv.mockPublicIP, nil
}

// GetHostname overrides the method for testing
func (tv *TestValidator) GetHostname() (string, error) {
	return tv.mockHostname, nil
}

// configurePublicIP overrides the method for testing to use mocked values
func (tv *TestValidator) configurePublicIP() (err error) {
	tv.PublicIP = tv.mockPublicIP
	tv.logger.Debug().
		Str("public_ip", tv.PublicIP).
		Msg("public ip set")
	return nil
}

// configureHostname overrides the method for testing to use mocked values
func (tv *TestValidator) configureHostname() (err error) {
	tv.Hostname = tv.mockHostname
	tv.logger.Debug().
		Str("hostname", tv.Hostname).
		Msg("hostname set")
	return nil
}

// configureGossipNode overrides the method for testing to use mocked client
func (tv *TestValidator) configureGossipNode() (err error) {
	tv.GossipNode, err = tv.mockSolanaClient.NodeFromIP(tv.PublicIP)
	if err != nil {
		return err
	}
	tv.logger.Debug().
		Str("public_ip", tv.GossipNode.IP()).
		Str("pubkey", tv.GossipNode.PubKey()).
		Msg("gossip node set")
	return nil
}

// NewFromConfig overrides the method for testing to use mocked configure methods
func (tv *TestValidator) NewFromConfig(cfg *Config) error {
	tv.logger.Debug().Msg("================================================")
	tv.logger.Debug().Msg("configuring...")
	defer tv.logger.Debug().Msg("================================================")
	defer tv.logger.Debug().Msg("configuration done")

	// configure solana rpc clients all in one
	err := tv.configureRPCClient(cfg.RPCAddress, cfg.Cluster, cfg.ClusterRPCURL, cfg.AverageSlotDuration)
	if err != nil {
		return err
	}

	// ensure supplied validator binary exists
	err = tv.configureBin(cfg.Bin)
	if err != nil {
		return err
	}

	// ledger dir must be valid and exist
	err = tv.configureLedgerDir(cfg.LedgerDir)
	if err != nil {
		return err
	}

	// configure identities
	err = tv.configureIdentities(cfg.Identities)
	if err != nil {
		return err
	}

	// tower file configure
	err = tv.configureTowerFile(cfg.Tower)
	if err != nil {
		return err
	}

	// set identity commands configure
	err = tv.configureSetIdenttiyCommands(cfg.Failover)
	if err != nil {
		return err
	}

	// configure hooks
	err = tv.configureHooks(cfg.Failover)
	if err != nil {
		return err
	}

	// must have at least one peer, each peer must have a valid string <host>:<port>
	err = tv.configurePeers(cfg.Failover.Peers)
	if err != nil {
		return err
	}

	// get public ip - use overridden method
	err = tv.configurePublicIP()
	if err != nil {
		return err
	}

	// get minimum time to leader slot parse and set
	err = tv.configureMinimumTimeToLeaderSlot(cfg.Failover.MinimumTimeToLeaderSlot)
	if err != nil {
		return err
	}

	// get hostname - use overridden method
	err = tv.configureHostname()
	if err != nil {
		return err
	}

	// get gossip node - use overridden method
	err = tv.configureGossipNode()
	if err != nil {
		return err
	}

	return nil
}

// NewSolanaRPCClient overrides the method for testing
func (tv *TestValidator) NewSolanaRPCClient(params solanapkg.NewClientParams) solanapkg.ClientInterface {
	return tv.mockSolanaClient
}

// EnsureBins overrides the method for testing
func (tv *TestValidator) EnsureBins(bin string) error {
	return nil // Always succeed in tests
}

// Helper function to create test key files
func createTestKeyFile(t *testing.T, tempDir, filename string) string {
	keyFile := filepath.Join(tempDir, filename)

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0o600)
	require.NoError(t, err)

	return keyFile
}

// Helper function to create a dummy agave-validator binary
func createDummyAgaveValidator(t *testing.T) string {
	// Create a temporary directory for the dummy binary
	tempDir := t.TempDir()
	dummyBin := filepath.Join(tempDir, "agave-validator")

	// Create a simple executable file
	err := os.WriteFile(dummyBin, []byte("#!/bin/sh\necho 'dummy agave-validator'"), 0o755)
	require.NoError(t, err)

	// Add the temp directory to PATH
	oldPath := os.Getenv("PATH")
	newPath := tempDir + ":" + oldPath
	os.Setenv("PATH", newPath)

	return dummyBin
}

// Helper function to create a test validator
func createTestValidator(t *testing.T) *TestValidator {
	return &TestValidator{
		Validator: &Validator{
			logger: log.With().Str("component", "validator").Logger(),
		},
		mockPublicIP:     "192.168.1.100",
		mockHostname:     "test-validator",
		mockSolanaClient: solanapkg.NewMockClient(),
	}
}

// ============================================================================
// Tests for configureRPCClient
// ============================================================================

func TestConfigureRPCClient_Success(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureRPCClient("http://localhost:8899", "testnet", "", "400ms")

	assert.NoError(t, err)
	assert.NotNil(t, validator.solanaRPCClient)
}

func TestConfigureRPCClient_EmptyCluster(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureRPCClient("http://localhost:8899", "", "", "400ms")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cluster is required")
}

func TestConfigureRPCClient_InvalidRPCAddress(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureRPCClient("invalid-address", "testnet", "", "400ms")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid rpc address")
}

func TestConfigureRPCClient_CustomCluster_Success(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureRPCClient("http://localhost:8899", "custom-mainnet", "https://mainnet.custom.io", "400ms")

	assert.NoError(t, err)
	assert.NotNil(t, validator.solanaRPCClient)
}

func TestConfigureRPCClient_CustomCluster_MissingClusterRPCURL(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureRPCClient("http://localhost:8899", "custom-mainnet", "", "400ms")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cluster_rpc_url is required")
}

// ============================================================================
// Tests for configureBin
// ============================================================================

func TestConfigureBin_Success(t *testing.T) {
	createDummyAgaveValidator(t)
	validator := createTestValidator(t)

	err := validator.configureBin("agave-validator")

	assert.NoError(t, err)
	assert.Equal(t, "agave-validator", validator.Bin)
}

func TestConfigureBin_BinaryNotFound(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureBin("non-existent-binary")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-existent-binary not found")
}

// ============================================================================
// Tests for configureLedgerDir
// ============================================================================

func TestConfigureLedgerDir_Success(t *testing.T) {
	validator := createTestValidator(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

	err := os.MkdirAll(ledgerDir, 0o755)
	require.NoError(t, err)

	err = validator.configureLedgerDir(ledgerDir)

	assert.NoError(t, err)
	assert.Equal(t, ledgerDir, validator.LedgerDir)
}

func TestConfigureLedgerDir_DirectoryNotFound(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureLedgerDir("/non/existent/directory")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a valid directory")
}

func TestConfigureLedgerDir_NotADirectory(t *testing.T) {
	validator := createTestValidator(t)
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "not-a-dir")

	err := os.WriteFile(filePath, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	err = validator.configureLedgerDir(filePath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a valid directory")
}

// ============================================================================
// Tests for configureIdentities
// ============================================================================

func TestConfigureIdentities_Success(t *testing.T) {
	validator := createTestValidator(t)
	tempDir := t.TempDir()
	activeKeyFile := createTestKeyFile(t, tempDir, "active-key.json")
	passiveKeyFile := createTestKeyFile(t, tempDir, "passive-key.json")

	identitiesConfig := identities.Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	err := validator.configureIdentities(identitiesConfig)

	assert.NoError(t, err)
	assert.NotNil(t, validator.Identities)
	assert.NotNil(t, validator.Identities.Active)
	assert.NotNil(t, validator.Identities.Passive)
}

func TestConfigureIdentities_ActiveFileNotFound(t *testing.T) {
	validator := createTestValidator(t)
	tempDir := t.TempDir()
	passiveKeyFile := createTestKeyFile(t, tempDir, "passive-key.json")

	identitiesConfig := identities.Config{
		Active:  "/non/existent/active.json",
		Passive: passiveKeyFile,
	}

	err := validator.configureIdentities(identitiesConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}

func TestConfigureIdentities_PassiveFileNotFound(t *testing.T) {
	validator := createTestValidator(t)
	tempDir := t.TempDir()
	activeKeyFile := createTestKeyFile(t, tempDir, "active-key.json")

	identitiesConfig := identities.Config{
		Active:  activeKeyFile,
		Passive: "/non/existent/passive.json",
	}

	err := validator.configureIdentities(identitiesConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}

// ============================================================================
// Tests for configurePeers
// ============================================================================

func TestConfigurePeers_Success(t *testing.T) {
	validator := createTestValidator(t)

	peersConfig := PeersConfig{
		"peer1": {Address: "192.168.1.100:9898"},
		"peer2": {Address: "192.168.1.101:9898"},
	}

	err := validator.configurePeers(peersConfig)

	assert.NoError(t, err)
	assert.Len(t, validator.Peers, 2)
	assert.Equal(t, "192.168.1.100:9898", validator.Peers["peer1"].Address)
	assert.Equal(t, "192.168.1.101:9898", validator.Peers["peer2"].Address)
}

func TestConfigurePeers_NoPeers(t *testing.T) {
	validator := createTestValidator(t)

	peersConfig := PeersConfig{}

	err := validator.configurePeers(peersConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have at least one peer")
}

func TestConfigurePeers_InvalidPeerAddress(t *testing.T) {
	validator := createTestValidator(t)

	peersConfig := PeersConfig{
		"peer1": {Address: "invalid-peer-address"},
	}

	err := validator.configurePeers(peersConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid peer address")
}

func TestConfigurePeers_InvalidPeerAddressNoPort(t *testing.T) {
	validator := createTestValidator(t)

	peersConfig := PeersConfig{
		"peer1": {Address: "192.168.1.100"},
	}

	err := validator.configurePeers(peersConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid peer address")
}

// ============================================================================
// Tests for configureMinimumTimeToLeaderSlot
// ============================================================================

func TestConfigureMinimumTimeToLeaderSlot_Success(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureMinimumTimeToLeaderSlot("5m")

	assert.NoError(t, err)
	assert.Equal(t, 5*time.Minute, validator.MinimumTimeToLeaderSlot)
}

func TestConfigureMinimumTimeToLeaderSlot_InvalidDuration(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureMinimumTimeToLeaderSlot("invalid-duration")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse minimum time to leader slot")
}

// ============================================================================
// Tests for configurePublicIP
// ============================================================================

func TestConfigurePublicIP_Success(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configurePublicIP()

	assert.NoError(t, err)
	assert.Equal(t, "192.168.1.100", validator.PublicIP)
}

// ============================================================================
// Tests for configureHostname
// ============================================================================

func TestConfigureHostname_Success(t *testing.T) {
	validator := createTestValidator(t)

	err := validator.configureHostname()

	assert.NoError(t, err)
	assert.Equal(t, "test-validator", validator.Hostname)
}

// ============================================================================
// Tests for configureGossipNode
// ============================================================================

func TestConfigureGossipNode_Success(t *testing.T) {
	validator := createTestValidator(t)
	// Set up the public IP first
	validator.PublicIP = "192.168.1.100"

	// Create a mock node
	mockNode := solanapkg.NewMockNode(solana.NewWallet().PrivateKey.PublicKey(), "1.16.0")
	validator.mockSolanaClient = solanapkg.NewMockClient().WithMockNode(mockNode)

	err := validator.configureGossipNode()

	assert.NoError(t, err)
	assert.NotNil(t, validator.GossipNode)
}

func TestConfigureGossipNode_NodeNotFound(t *testing.T) {
	validator := createTestValidator(t)
	// Set up the public IP first
	validator.PublicIP = "192.168.1.100"

	// Create a mock client that returns error for NodeFromIP
	validator.mockSolanaClient = solanapkg.NewMockClient().WithNodeFromIP(func(ip string) (*solanapkg.Node, error) {
		return nil, errors.New("node not found")
	})

	err := validator.configureGossipNode()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

// ============================================================================
// Tests for configureHooks
// ============================================================================

func TestConfigureHooks_Success(t *testing.T) {
	validator := createTestValidator(t)

	failoverConfig := FailoverConfig{
		Hooks: hooks.FailoverHooks{
			Pre:  hooks.PreHooks{WhenActive: []hooks.Hook{{Name: "test-hook", Command: "echo", Args: []string{"test"}}}},
			Post: hooks.PostHooks{WhenActive: []hooks.Hook{{Name: "test-hook", Command: "echo", Args: []string{"test"}}}},
		},
	}

	err := validator.configureHooks(failoverConfig)

	assert.NoError(t, err)
	assert.Len(t, validator.Hooks.Pre.WhenActive, 1)
	assert.Len(t, validator.Hooks.Post.WhenActive, 1)
	assert.Equal(t, "test-hook", validator.Hooks.Pre.WhenActive[0].Name)
}

// ============================================================================
// Legacy tests for backward compatibility
// ============================================================================

func TestNewFromConfig_Success(t *testing.T) {
	// Create temporary directories and files
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")
	towerDir := filepath.Join(tempDir, "tower")

	// Create directories
	err := os.MkdirAll(ledgerDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(towerDir, 0o755)
	require.NoError(t, err)

	// create test agave-validator binary
	agaveValidatorBin := createDummyAgaveValidator(t)

	// Create test key files
	activeKeyFile := createTestKeyFile(t, tempDir, "active-key.json")
	passiveKeyFile := createTestKeyFile(t, tempDir, "passive-key.json")

	// Create config
	cfg := &Config{
		Bin:        agaveValidatorBin,
		Cluster:    "testnet",
		RPCAddress: "http://localhost:8899",
		LedgerDir:  ledgerDir,
		Identities: identities.Config{
			Active:  activeKeyFile,
			Passive: passiveKeyFile,
		},
		Tower: TowerConfig{
			Dir:                  towerDir,
			FileNameTemplate:     "tower-{{ .Identities.Active.PubKey }}.bin",
			AutoEmptyWhenPassive: true,
		},
		Failover: FailoverConfig{
			MinimumTimeToLeaderSlot:       "5m",
			SetIdentityActiveCmdTemplate:  "{{ .Bin }} set-identity {{ .Identities.Active.KeyFile }}",
			SetIdentityPassiveCmdTemplate: "{{ .Bin }} set-identity {{ .Identities.Passive.KeyFile }}",
			Peers: map[string]struct {
				Address string `mapstructure:"address"`
			}{
				"peer1": {Address: "192.168.1.100:9898"},
				"peer2": {Address: "192.168.1.101:9898"},
			},
		},
	}

	// Create test validator with mocked dependencies
	testValidator := &TestValidator{
		Validator: &Validator{
			logger: log.With().Str("component", "validator").Logger(),
		},
		mockPublicIP:     "192.168.1.100",
		mockHostname:     "test-validator",
		mockSolanaClient: solanapkg.NewMockClient().WithHealthStatus(true),
	}

	// Call NewFromConfig on the test validator
	err = testValidator.NewFromConfig(cfg)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, agaveValidatorBin, testValidator.Bin)
	assert.Equal(t, ledgerDir, testValidator.LedgerDir)
	assert.Equal(t, "192.168.1.100", testValidator.PublicIP)
	assert.Equal(t, "test-validator", testValidator.Hostname)
	assert.True(t, testValidator.TowerFileAutoDeleteWhenPassive)
	assert.Len(t, testValidator.Peers, 2)
	assert.Equal(t, "192.168.1.100:9898", testValidator.Peers["peer1"].Address)
	assert.Equal(t, "192.168.1.101:9898", testValidator.Peers["peer2"].Address)
	assert.Equal(t, 5*time.Minute, testValidator.MinimumTimeToLeaderSlot)
}

// ============================================================================
// Other existing tests
// ============================================================================

func TestValidator_IsActive(t *testing.T) {
	// Create test identities
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	activeIdentity := &identities.Identity{
		KeyFile: "/path/to/active.json",
		Key:     activeKey,
	}
	passiveIdentity := &identities.Identity{
		KeyFile: "/path/to/passive.json",
		Key:     passiveKey,
	}

	identities := &identities.Identities{
		Active:  activeIdentity,
		Passive: passiveIdentity,
	}

	// Create validator with mock gossip node that matches active pubkey
	validator := &Validator{
		Identities: identities,
		GossipNode: solanapkg.NewMockNode(activeKey.PublicKey(), "1.16.0"),
	}

	// Test IsActive
	assert.True(t, validator.IsActive())
	assert.False(t, validator.IsPassive())
}

func TestValidator_IsPassive(t *testing.T) {
	// Create test identities
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	activeIdentity := &identities.Identity{
		KeyFile: "/path/to/active.json",
		Key:     activeKey,
	}
	passiveIdentity := &identities.Identity{
		KeyFile: "/path/to/passive.json",
		Key:     passiveKey,
	}

	identities := &identities.Identities{
		Active:  activeIdentity,
		Passive: passiveIdentity,
	}

	// Create validator with mock gossip node that matches passive pubkey
	validator := &Validator{
		Identities: identities,
		GossipNode: solanapkg.NewMockNode(passiveKey.PublicKey(), "1.16.0"),
	}

	// Test IsPassive
	assert.True(t, validator.IsPassive())
	assert.False(t, validator.IsActive())
}

func TestValidator_IsNeitherActiveNorPassive(t *testing.T) {
	// Create test identities
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	activeIdentity := &identities.Identity{
		KeyFile: "/path/to/active.json",
		Key:     activeKey,
	}
	passiveIdentity := &identities.Identity{
		KeyFile: "/path/to/passive.json",
		Key:     passiveKey,
	}

	identities := &identities.Identities{
		Active:  activeIdentity,
		Passive: passiveIdentity,
	}

	// Create validator with mock gossip node that has different pubkey
	otherKey := solana.NewWallet().PrivateKey
	validator := &Validator{
		Identities: identities,
		GossipNode: solanapkg.NewMockNode(otherKey.PublicKey(), "1.16.0"),
	}

	// Test neither active nor passive
	assert.False(t, validator.IsActive())
	assert.False(t, validator.IsPassive())
}

func TestValidator_BasicProperties(t *testing.T) {
	// Test basic validator properties
	validator := &Validator{
		Bin:       "/usr/local/bin/agave-validator",
		LedgerDir: "/mnt/ledger",
		PublicIP:  "192.168.1.100",
		Hostname:  "test-validator",
		BinMetadata: BinMetadata{
			Client:  "agave-validator",
			Version: "1.16.0",
		},
		MinimumTimeToLeaderSlot:        5 * time.Minute,
		TowerFileAutoDeleteWhenPassive: true,
	}

	assert.Equal(t, "/usr/local/bin/agave-validator", validator.Bin)
	assert.Equal(t, "/mnt/ledger", validator.LedgerDir)
	assert.Equal(t, "192.168.1.100", validator.PublicIP)
	assert.Equal(t, "test-validator", validator.Hostname)
	assert.Equal(t, "agave-validator", validator.BinMetadata.Client)
	assert.Equal(t, "1.16.0", validator.BinMetadata.Version)
	assert.Equal(t, 5*time.Minute, validator.MinimumTimeToLeaderSlot)
	assert.True(t, validator.TowerFileAutoDeleteWhenPassive)
}

func TestValidator_CommandTemplates(t *testing.T) {
	// Test command template properties
	validator := &Validator{
		SetIdentityActiveCommand:  "agave-validator set-identity /path/to/active.json",
		SetIdentityPassiveCommand: "agave-validator set-identity /path/to/passive.json",
	}

	assert.Equal(t, "agave-validator set-identity /path/to/active.json", validator.SetIdentityActiveCommand)
	assert.Equal(t, "agave-validator set-identity /path/to/passive.json", validator.SetIdentityPassiveCommand)
}

func TestValidator_PeersOperations(t *testing.T) {
	// Test validator peers operations
	validator := &Validator{
		Peers: make(Peers),
	}

	// Add peers
	validator.Peers["peer1"] = Peer{
		Name:    "peer1",
		Address: "192.168.1.100:9898",
	}
	validator.Peers["peer2"] = Peer{
		Name:    "peer2",
		Address: "192.168.1.101:9898",
	}

	// Test peer operations
	assert.Len(t, validator.Peers, 2)
	assert.Equal(t, "192.168.1.100:9898", validator.Peers["peer1"].Address)
	assert.Equal(t, "192.168.1.101:9898", validator.Peers["peer2"].Address)

	// Test peer string representation
	assert.Equal(t, "peer1", validator.Peers["peer1"].Name)
	assert.Equal(t, "peer2", validator.Peers["peer2"].Name)
}

func TestFailoverParams_DefaultValues(t *testing.T) {
	params := FailoverParams{}

	assert.False(t, params.NotADrill)
	assert.False(t, params.NoWaitForHealthy)
	assert.False(t, params.NoMinTimeToLeaderSlot)
	assert.Equal(t, time.Duration(0), params.MinTimeToLeaderSlot)
}

func TestFailoverParams_WithValues(t *testing.T) {
	params := FailoverParams{
		NotADrill:             true,
		NoWaitForHealthy:      true,
		NoMinTimeToLeaderSlot: true,
		MinTimeToLeaderSlot:   10 * time.Minute,
	}

	assert.True(t, params.NotADrill)
	assert.True(t, params.NoWaitForHealthy)
	assert.True(t, params.NoMinTimeToLeaderSlot)
	assert.Equal(t, 10*time.Minute, params.MinTimeToLeaderSlot)
}

func TestPeer_StringRepresentation(t *testing.T) {
	peer := Peer{
		Name:    "test-peer",
		Address: "192.168.1.100:9898",
	}

	assert.Equal(t, "test-peer", peer.Name)
	assert.Equal(t, "192.168.1.100:9898", peer.Address)
}

func TestPeers_MapOperations(t *testing.T) {
	peers := make(Peers)

	// Add peers
	peers["peer1"] = Peer{Name: "peer1", Address: "192.168.1.100:9898"}
	peers["peer2"] = Peer{Name: "peer2", Address: "192.168.1.101:9898"}

	// Test map operations
	assert.Len(t, peers, 2)
	assert.Equal(t, "192.168.1.100:9898", peers["peer1"].Address)
	assert.Equal(t, "192.168.1.101:9898", peers["peer2"].Address)

	// Test deletion
	delete(peers, "peer1")
	assert.Len(t, peers, 1)
	assert.Equal(t, "192.168.1.101:9898", peers["peer2"].Address)
}

func TestBinMetadata_StringRepresentation(t *testing.T) {
	metadata := BinMetadata{
		Client:  "agave-validator",
		Version: "1.16.0",
	}

	assert.Equal(t, "agave-validator", metadata.Client)
	assert.Equal(t, "1.16.0", metadata.Version)
}

func BenchmarkValidator_IsActive(b *testing.B) {
	// Create test identities
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	activeIdentity := &identities.Identity{
		KeyFile: "/path/to/active.json",
		Key:     activeKey,
	}
	passiveIdentity := &identities.Identity{
		KeyFile: "/path/to/passive.json",
		Key:     passiveKey,
	}

	identities := &identities.Identities{
		Active:  activeIdentity,
		Passive: passiveIdentity,
	}

	// Create validator with mock gossip node that matches active pubkey
	validator := &Validator{
		Identities: identities,
		GossipNode: solanapkg.NewMockNode(activeKey.PublicKey(), "1.16.0"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.IsActive()
	}
}

func BenchmarkValidator_IsPassive(b *testing.B) {
	// Create test identities
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	activeIdentity := &identities.Identity{
		KeyFile: "/path/to/active.json",
		Key:     activeKey,
	}
	passiveIdentity := &identities.Identity{
		KeyFile: "/path/to/passive.json",
		Key:     passiveKey,
	}

	identities := &identities.Identities{
		Active:  activeIdentity,
		Passive: passiveIdentity,
	}

	// Create validator with mock gossip node that matches passive pubkey
	validator := &Validator{
		Identities: identities,
		GossipNode: solanapkg.NewMockNode(passiveKey.PublicKey(), "1.16.0"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.IsPassive()
	}
}
