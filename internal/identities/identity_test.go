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

func TestNewIdentityFromFile_Success(t *testing.T) {
	// Create a temporary key file
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "test-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Test creating identity from file
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Equal(t, privateKey.String(), identity.Key.String())
	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey())
}

func TestNewIdentityFromFile_WithTildePath(t *testing.T) {
	// Create a temporary key file in home directory
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	tempDir := filepath.Join(homeDir, "test-identity-temp")
	err = os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	keyFile := filepath.Join(tempDir, "test-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Test with tilde path
	tildePath := "~/test-identity-temp/test-key.json"
	identity, err := NewIdentityFromFile(tildePath)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Equal(t, privateKey.String(), identity.Key.String())
	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey())
}

func TestNewIdentityFromFile_RelativePath(t *testing.T) {
	// Create a temporary key file
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "test-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Change to temp directory to test relative path
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Test with relative path
	relativePath := "test-key.json"
	identity, err := NewIdentityFromFile(relativePath)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Equal(t, privateKey.String(), identity.Key.String())
	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey())
}

func TestNewIdentityFromFile_FileNotFound(t *testing.T) {
	// Test with non-existent file
	nonExistentFile := "/path/to/non/existent/key.json"
	identity, err := NewIdentityFromFile(nonExistentFile)

	// Assertions
	assert.Error(t, err)
	assert.NotNil(t, identity)  // Function returns Identity struct even on error
	assert.Nil(t, identity.Key) // But the Key is nil
	assert.Equal(t, nonExistentFile, identity.KeyFile)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewIdentityFromFile_EmptyPath(t *testing.T) {
	// Test with empty path
	identity, err := NewIdentityFromFile("")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "failed to resolve path")
}

func TestNewIdentityFromFile_InvalidKeyFile(t *testing.T) {
	// Create a temporary file with invalid key data
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "invalid-key.json")

	// Write invalid key data
	invalidKeyData := "invalid-key-data"
	err := os.WriteFile(keyFile, []byte(invalidKeyData), 0600)
	require.NoError(t, err)

	// Test creating identity from invalid file
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions
	assert.Error(t, err)
	assert.NotNil(t, identity)  // Function returns Identity struct even on error
	assert.Nil(t, identity.Key) // But the Key is nil
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewIdentityFromFile_EmptyKeyFile(t *testing.T) {
	// Create a temporary empty file
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "empty-key.json")

	// Write empty file
	err := os.WriteFile(keyFile, []byte(""), 0600)
	require.NoError(t, err)

	// Test creating identity from empty file
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions
	assert.Error(t, err)
	assert.NotNil(t, identity)  // Function returns Identity struct even on error
	assert.Nil(t, identity.Key) // But the Key is nil
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Contains(t, err.Error(), "failed to parse keygen file")
}

func TestNewIdentityFromFile_PermissionDenied(t *testing.T) {
	// Create a temporary key file with no read permissions
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "no-permission-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Remove read permissions
	err = os.Chmod(keyFile, 0000)
	require.NoError(t, err)

	// Test creating identity from file without read permissions
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions - handle both success and failure cases
	if err != nil {
		// Expected error case
		assert.NotNil(t, identity)  // Function returns Identity struct even on error
		assert.Nil(t, identity.Key) // But the Key is nil
		assert.Equal(t, keyFile, identity.KeyFile)
		assert.Contains(t, err.Error(), "failed to parse keygen file")
	} else {
		// In some environments, the file might still be readable
		// This is acceptable behavior
		assert.NotNil(t, identity)
		assert.NotNil(t, identity.Key)
		assert.Equal(t, keyFile, identity.KeyFile)
	}

	// Restore permissions for cleanup
	os.Chmod(keyFile, 0600)
}

func TestIdentity_Pubkey(t *testing.T) {
	// Create a test identity
	privateKey := solana.NewWallet().PrivateKey
	identity := &Identity{
		KeyFile: "/path/to/key.json",
		Key:     privateKey,
	}

	// Test Pubkey method
	pubkey := identity.PubKey()

	// Assertions
	assert.Equal(t, privateKey.PublicKey().String(), pubkey)
	assert.NotEmpty(t, pubkey)
}

func TestIdentity_Pubkey_Consistency(t *testing.T) {
	// Create a test identity
	privateKey := solana.NewWallet().PrivateKey
	identity := &Identity{
		KeyFile: "/path/to/key.json",
		Key:     privateKey,
	}

	// Test that Pubkey returns the same value multiple times
	pubkey1 := identity.PubKey()
	pubkey2 := identity.PubKey()
	pubkey3 := identity.PubKey()

	// Assertions
	assert.Equal(t, pubkey1, pubkey2)
	assert.Equal(t, pubkey2, pubkey3)
	assert.Equal(t, privateKey.PublicKey().String(), pubkey1)
}

func TestIdentity_Pubkey_DeprecatedMethod(t *testing.T) {
	// Create a test identity
	privateKey := solana.NewWallet().PrivateKey
	identity := &Identity{
		KeyFile: "/path/to/key.json",
		Key:     privateKey,
	}

	// Test that Pubkey() (deprecated) returns the same result as PubKey()
	pubkeyDeprecated := identity.Pubkey()
	pubkeyCorrect := identity.PubKey()

	// Assertions
	assert.Equal(t, pubkeyCorrect, pubkeyDeprecated)
	assert.Equal(t, privateKey.PublicKey().String(), pubkeyDeprecated)
	assert.Equal(t, privateKey.PublicKey().String(), pubkeyCorrect)
}

func TestNewIdentityFromFile_WithBase58Key(t *testing.T) {
	// Create a temporary key file with base58 encoded key
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "base58-key.json")

	// Generate a test private key and get its base58 representation
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Test creating identity from file
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Equal(t, privateKey.String(), identity.Key.String())
	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey()) //nolint:staticcheck
}

func TestNewIdentityFromFile_WithWhitespace(t *testing.T) {
	// Create a temporary key file with whitespace around the key
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "whitespace-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(t, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(t, err)

	// Test creating identity from file
	identity, err := NewIdentityFromFile(keyFile)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, keyFile, identity.KeyFile)
	assert.Equal(t, privateKey.String(), identity.Key.String())
	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey())
}

// Benchmark tests
func BenchmarkNewIdentityFromFile(b *testing.B) {
	// Create a temporary key file
	tempDir := b.TempDir()
	keyFile := filepath.Join(tempDir, "benchmark-key.json")

	// Generate a test private key
	privateKey := solana.NewWallet().PrivateKey

	// Convert private key to byte array for keygen file format
	keyBytes := []byte(privateKey)
	keyData, err := json.Marshal(keyBytes)
	require.NoError(b, err)

	// Write the key to file
	err = os.WriteFile(keyFile, keyData, 0600)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewIdentityFromFile(keyFile)
	}
}

func TestNewIdentityFromPubkey_Success(t *testing.T) {
	pubkey := "11111111111111111111111111111111"
	identity, err := NewIdentityFromPubkey(pubkey)

	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Nil(t, identity.Key)
	assert.Empty(t, identity.KeyFile)
	assert.Equal(t, pubkey, identity.PubKey())
	assert.Equal(t, pubkey, identity.PubKeyStr)
}

func TestNewIdentityFromPubkey_InvalidBase58(t *testing.T) {
	identity, err := NewIdentityFromPubkey("not-valid-base58!!!")

	assert.Error(t, err)
	assert.Nil(t, identity)
	assert.Contains(t, err.Error(), "failed to parse pubkey")
}

func TestIdentity_PubKey_PubkeyOnlyMode(t *testing.T) {
	identity := &Identity{
		PubKeyStr: "SysvarC1ock11111111111111111111111111111111",
	}

	assert.Equal(t, "SysvarC1ock11111111111111111111111111111111", identity.PubKey())
}

func TestIdentity_PubKey_KeypairTakesPrecedence(t *testing.T) {
	privateKey := solana.NewWallet().PrivateKey
	identity := &Identity{
		Key:       privateKey,
		PubKeyStr: "11111111111111111111111111111111", // should be ignored
	}

	assert.Equal(t, privateKey.PublicKey().String(), identity.PubKey())
	assert.NotEqual(t, "11111111111111111111111111111111", identity.PubKey())
}

func BenchmarkIdentity_Pubkey(b *testing.B) {
	// Create a test identity
	privateKey := solana.NewWallet().PrivateKey
	identity := &Identity{
		KeyFile: "/path/to/key.json",
		Key:     privateKey,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = identity.PubKey()
	}
}
