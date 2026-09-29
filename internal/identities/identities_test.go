package identities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/solana-foundation/solana-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromConfig_Success(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "active-key.json")
	passiveKeyFile := filepath.Join(tempDir, "passive-key.json")

	// Generate two different private keys
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	// Ensure they are different
	require.NotEqual(t, activeKey.String(), passiveKey.String())

	// Create keygen files
	activeKeyBytes := []byte(activeKey)
	activeKeyData, err := json.Marshal(activeKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(activeKeyFile, activeKeyData, 0600)
	require.NoError(t, err)

	passiveKeyBytes := []byte(passiveKey)
	passiveKeyData, err := json.Marshal(passiveKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, passiveKeyData, 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identities)
	assert.NotNil(t, identities.Active)
	assert.NotNil(t, identities.Passive)
	assert.Equal(t, activeKeyFile, identities.Active.KeyFile)
	assert.Equal(t, passiveKeyFile, identities.Passive.KeyFile)
	assert.Equal(t, activeKey.String(), identities.Active.Key.String())
	assert.Equal(t, passiveKey.String(), identities.Passive.Key.String())
	assert.Equal(t, activeKey.PublicKey().String(), identities.Active.PubKey())
	assert.Equal(t, passiveKey.PublicKey().String(), identities.Passive.PubKey())
	assert.Equal(t, activeKey.PublicKey().String(), identities.Active.PubKeyStr)
	assert.Equal(t, passiveKey.PublicKey().String(), identities.Passive.PubKeyStr)
}

func TestNewFromConfig_ActiveFileNotFound(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "non-existent-active.json")
	passiveKeyFile := filepath.Join(tempDir, "passive-key.json")

	// Generate a private key for passive
	passiveKey := solana.NewWallet().PrivateKey
	passiveKeyBytes := []byte(passiveKey)
	passiveKeyData, err := json.Marshal(passiveKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, passiveKeyData, 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewFromConfig_PassiveFileNotFound(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "active-key.json")
	passiveKeyFile := filepath.Join(tempDir, "non-existent-passive.json")

	// Generate a private key for active
	activeKey := solana.NewWallet().PrivateKey
	activeKeyBytes := []byte(activeKey)
	activeKeyData, err := json.Marshal(activeKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(activeKeyFile, activeKeyData, 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewFromConfig_SameIdentities(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "same-key.json")
	passiveKeyFile := filepath.Join(tempDir, "same-key-copy.json")

	// Generate a single private key
	sameKey := solana.NewWallet().PrivateKey
	sameKeyBytes := []byte(sameKey)
	sameKeyData, err := json.Marshal(sameKeyBytes)
	require.NoError(t, err)

	// Write the same key to both files
	err = os.WriteFile(activeKeyFile, sameKeyData, 0600)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, sameKeyData, 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "active and passive identities must be different")
}

func TestNewFromConfig_InvalidActiveKeyFile(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "invalid-active.json")
	passiveKeyFile := filepath.Join(tempDir, "passive-key.json")

	// Create invalid key file
	invalidKeyData := "invalid-key-data"
	err := os.WriteFile(activeKeyFile, []byte(invalidKeyData), 0600)
	require.NoError(t, err)

	// Generate a valid private key for passive
	passiveKey := solana.NewWallet().PrivateKey
	passiveKeyBytes := []byte(passiveKey)
	passiveKeyData, err := json.Marshal(passiveKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, passiveKeyData, 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewFromConfig_InvalidPassiveKeyFile(t *testing.T) {
	// Create temporary key files
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "active-key.json")
	passiveKeyFile := filepath.Join(tempDir, "invalid-passive.json")

	// Generate a valid private key for active
	activeKey := solana.NewWallet().PrivateKey
	activeKeyBytes := []byte(activeKey)
	activeKeyData, err := json.Marshal(activeKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(activeKeyFile, activeKeyData, 0600)
	require.NoError(t, err)

	// Create invalid key file
	invalidKeyData := "invalid-key-data"
	err = os.WriteFile(passiveKeyFile, []byte(invalidKeyData), 0600)
	require.NoError(t, err)

	// Create config
	cfg := &Config{
		Active:  activeKeyFile,
		Passive: passiveKeyFile,
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewFromConfig_WithTildePaths(t *testing.T) {
	// Create temporary key files in home directory
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	tempDir := filepath.Join(homeDir, "test-identities-temp")
	err = os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	activeKeyFile := filepath.Join(tempDir, "active-key.json")
	passiveKeyFile := filepath.Join(tempDir, "passive-key.json")

	// Generate two different private keys
	activeKey := solana.NewWallet().PrivateKey
	passiveKey := solana.NewWallet().PrivateKey

	// Ensure they are different
	require.NotEqual(t, activeKey.String(), passiveKey.String())

	// Create keygen files
	activeKeyBytes := []byte(activeKey)
	activeKeyData, err := json.Marshal(activeKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(activeKeyFile, activeKeyData, 0600)
	require.NoError(t, err)

	passiveKeyBytes := []byte(passiveKey)
	passiveKeyData, err := json.Marshal(passiveKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, passiveKeyData, 0600)
	require.NoError(t, err)

	// Create config with tilde paths
	cfg := &Config{
		Active:  "~/test-identities-temp/active-key.json",
		Passive: "~/test-identities-temp/passive-key.json",
	}

	// Test NewFromConfig
	identities, err := NewFromConfig(cfg)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identities)
	assert.NotNil(t, identities.Active)
	assert.NotNil(t, identities.Passive)
	assert.Equal(t, activeKeyFile, identities.Active.KeyFile)
	assert.Equal(t, passiveKeyFile, identities.Passive.KeyFile)
	assert.Equal(t, activeKey.String(), identities.Active.Key.String())
	assert.Equal(t, passiveKey.String(), identities.Passive.Key.String())
}

func TestNewFromConfig_PubkeyOnly(t *testing.T) {
	activePubkey := "11111111111111111111111111111111"
	passivePubkey := "SysvarC1ock11111111111111111111111111111111"

	cfg := &Config{
		ActivePubkey:  activePubkey,
		PassivePubkey: passivePubkey,
	}

	identities, err := NewFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, identities)
	assert.Nil(t, identities.Active.Key)
	assert.Nil(t, identities.Passive.Key)
	assert.Empty(t, identities.Active.KeyFile)
	assert.Empty(t, identities.Passive.KeyFile)
	assert.Equal(t, activePubkey, identities.Active.PubKey())
	assert.Equal(t, passivePubkey, identities.Passive.PubKey())
}

func TestNewFromConfig_MixedMode(t *testing.T) {
	// Active from pubkey, passive from keypair file
	tempDir := t.TempDir()
	passiveKeyFile := filepath.Join(tempDir, "passive-key.json")

	passiveKey := solana.NewWallet().PrivateKey
	passiveKeyBytes := []byte(passiveKey)
	passiveKeyData, err := json.Marshal(passiveKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(passiveKeyFile, passiveKeyData, 0600)
	require.NoError(t, err)

	cfg := &Config{
		ActivePubkey: "11111111111111111111111111111111",
		Passive:      passiveKeyFile,
	}

	identities, err := NewFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, identities)
	assert.Nil(t, identities.Active.Key)
	assert.NotNil(t, identities.Passive.Key)
	assert.Equal(t, "11111111111111111111111111111111", identities.Active.PubKey())
	assert.Equal(t, passiveKey.PublicKey().String(), identities.Passive.PubKey())
}

func TestNewFromConfig_PubkeyOnlySameIdentities(t *testing.T) {
	cfg := &Config{
		ActivePubkey:  "11111111111111111111111111111111",
		PassivePubkey: "11111111111111111111111111111111",
	}

	identities, err := NewFromConfig(cfg)

	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "active and passive identities must be different")
}

func TestNewFromConfig_InvalidActivePubkey(t *testing.T) {
	cfg := &Config{
		ActivePubkey:  "not-valid-base58",
		PassivePubkey: "SysvarC1ock11111111111111111111111111111111",
	}

	identities, err := NewFromConfig(cfg)

	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to load active identity")
}

func TestNewFromConfig_InvalidPassivePubkey(t *testing.T) {
	cfg := &Config{
		ActivePubkey:  "11111111111111111111111111111111",
		PassivePubkey: "not-valid-base58",
	}

	identities, err := NewFromConfig(cfg)

	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "failed to load passive identity")
}

func TestNewFromConfig_NeitherActiveNorPubkey(t *testing.T) {
	cfg := &Config{
		PassivePubkey: "SysvarC1ock11111111111111111111111111111111",
	}

	identities, err := NewFromConfig(cfg)

	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "either identities.active")
}

func TestNewFromConfig_NeitherPassiveNorPubkey(t *testing.T) {
	cfg := &Config{
		ActivePubkey: "11111111111111111111111111111111",
	}

	identities, err := NewFromConfig(cfg)

	assert.Error(t, err)
	assert.Nil(t, identities)
	assert.Contains(t, err.Error(), "either identities.passive")
}

func TestNewFromConfig_KeypairFileOverridesPubkey(t *testing.T) {
	// When both keypair file and pubkey are set, file takes precedence
	tempDir := t.TempDir()
	activeKeyFile := filepath.Join(tempDir, "active-key.json")

	activeKey := solana.NewWallet().PrivateKey
	activeKeyBytes := []byte(activeKey)
	activeKeyData, err := json.Marshal(activeKeyBytes)
	require.NoError(t, err)
	err = os.WriteFile(activeKeyFile, activeKeyData, 0600)
	require.NoError(t, err)

	cfg := &Config{
		Active:        activeKeyFile,
		ActivePubkey:  "11111111111111111111111111111111", // should be ignored
		PassivePubkey: "SysvarC1ock11111111111111111111111111111111",
	}

	identities, err := NewFromConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, identities)
	// Should use the key from file, not the pubkey string
	assert.Equal(t, activeKey.PublicKey().String(), identities.Active.PubKey())
	assert.NotEqual(t, "11111111111111111111111111111111", identities.Active.PubKey())
}
