package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromFile_WithValidConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
validator:
  bin: test-validator
  cluster: testnet
  rpc_address: http://localhost:8899
  ledger_dir: "/tmp/ledger"
  failover:
    server:
      port: 9999
      heartbeat_interval: 10s
      stream_timeout: 10m
    min_time_to_leader_slot: 10s
    monitor:
      credit_samples:
        count: 10
        interval: 10s
    peers:
      peer1:
        address: localhost:8001
      peer2:
        address: localhost:8002
  tower:
    file_name_template: "tower-test-{{ .Identities.Active.PubKey }}.bin"
  identities:
    active: /path/to/active/key.json
    passive: /path/to/passive/key.json
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test NewFromFile
	cfg, err := NewFromFile(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify the configuration was loaded correctly
	assert.Equal(t, "test-validator", cfg.Validator.Bin)
	assert.Equal(t, DefaultLogLevel, cfg.Log.Level)
	assert.Equal(t, DefaultLogFormat, cfg.Log.Format)
	assert.Equal(t, "testnet", cfg.Validator.Cluster)
	assert.Equal(t, "tower", cfg.Validator.Failover.Consensus.Mode)
	assert.Equal(t, DefaultSetIdentityActiveCmdTemplate, cfg.Validator.Failover.SetIdentityActiveCmdTemplate)
	assert.Equal(t, "http://localhost:8899", cfg.Validator.RPCAddress)
	assert.Equal(t, "/tmp/ledger", cfg.Validator.LedgerDir)
	assert.Equal(t, 9999, cfg.Validator.Failover.Server.Port)
	assert.Equal(t, "10s", cfg.Validator.Failover.Server.HeartbeatInterval)
	assert.Equal(t, "10m", cfg.Validator.Failover.Server.StreamTimeout)
	assert.Equal(t, "10s", cfg.Validator.Failover.MinimumTimeToLeaderSlot)
	assert.Equal(t, 10, cfg.Validator.Failover.Monitor.CreditSamples.Count)
	assert.Equal(t, "10s", cfg.Validator.Failover.Monitor.CreditSamples.Interval)
	assert.Equal(t, "tower-test-{{ .Identities.Active.PubKey }}.bin", cfg.Validator.Tower.FileNameTemplate)
	assert.Equal(t, "/path/to/active/key.json", cfg.Validator.Identities.Active)
	assert.Equal(t, "/path/to/passive/key.json", cfg.Validator.Identities.Passive)

	// Verify peers
	assert.Len(t, cfg.Validator.Failover.Peers, 2)
	assert.Equal(t, "localhost:8001", cfg.Validator.Failover.Peers["peer1"].Address)
	assert.Equal(t, "localhost:8002", cfg.Validator.Failover.Peers["peer2"].Address)
}

func TestAlpenglowDefaultIdentityCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("validator:\n  failover:\n    consensus:\n      mode: alpenglow\n"), 0600))
	cfg, err := NewFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, "alpenglow", cfg.Validator.Failover.Consensus.Mode)
	assert.NotContains(t, cfg.Validator.Failover.SetIdentityActiveCmdTemplate, "--require-tower")
}

func TestNewFromFile_WithEmptyConfigPath(t *testing.T) {
	// This should use the default config path, which will fail
	// since the default path doesn't exist
	cfg, err := NewFromFile("")
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestNewFromFile_WithNonExistentFile(t *testing.T) {
	nonExistentPath := "/non/existent/config.yaml"
	cfg, err := NewFromFile(nonExistentPath)
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoadFromConfigFile_WithValidConfig(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
validator:
  bin: custom-validator
  cluster: mainnet-beta
  rpc_address: https://api.mainnet-beta.solana.com
  ledger_dir: /mnt/ledger
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test LoadFromConfigFile
	cfg := &SolanaValidatorFailover{}
	err = cfg.LoadFromConfigFile(configPath)
	require.NoError(t, err)

	// Verify the configuration was loaded correctly
	assert.Equal(t, "custom-validator", cfg.Validator.Bin)
	assert.Equal(t, "mainnet-beta", cfg.Validator.Cluster)
	assert.Equal(t, "https://api.mainnet-beta.solana.com", cfg.Validator.RPCAddress)
	assert.Equal(t, "/mnt/ledger", cfg.Validator.LedgerDir)
}

func TestLoadFromConfigFile_WithDefaults(t *testing.T) {
	// Create a minimal config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "minimal-config.yaml")

	configContent := `
validator:
  cluster: testnet
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test LoadFromConfigFile
	cfg := &SolanaValidatorFailover{}
	err = cfg.LoadFromConfigFile(configPath)
	require.NoError(t, err)

	// Verify defaults are set correctly
	assert.Equal(t, DefaultBin, cfg.Validator.Bin)                                                                      // default
	assert.Equal(t, DefaultLogLevel, cfg.Log.Level)                                                                     // default
	assert.Equal(t, DefaultLogFormat, cfg.Log.Format)                                                                   // default
	assert.Equal(t, DefaultCluster, cfg.Validator.Cluster)                                                              // from config
	assert.Equal(t, DefaultFailoverServerPort, cfg.Validator.Failover.Server.Port)                                      // default
	assert.Equal(t, DefaultFailoverServerHeartbeatInterval, cfg.Validator.Failover.Server.HeartbeatInterval)            // default
	assert.Equal(t, DefaultFailoverServerStreamTimeout, cfg.Validator.Failover.Server.StreamTimeout)                    // default
	assert.Equal(t, DefaultFailoverMinimumTimeToLeaderSlot, cfg.Validator.Failover.MinimumTimeToLeaderSlot)             // default
	assert.Equal(t, DefaultFailoverMonitorCreditSamplesCount, cfg.Validator.Failover.Monitor.CreditSamples.Count)       // default
	assert.Equal(t, DefaultFailoverMonitorCreditSamplesInterval, cfg.Validator.Failover.Monitor.CreditSamples.Interval) // default
	assert.Equal(t, DefaultTowerFileNameTemplate, cfg.Validator.Tower.FileNameTemplate)                                 // default
}

func TestLoadFromConfigFile_WithLogConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "log-config.yaml")
	configContent := `
log:
  level: debug
  format: json
validator:
  cluster: testnet
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	cfg, err := NewFromFile(configPath)

	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestLoadFromConfigFile_InvalidLogLevel(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid-log-level.yaml")
	configContent := `
log:
  level: verbose
validator:
  cluster: testnet
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	_, err := NewFromFile(configPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "log.level must be one of debug, info, warn, error, fatal")
}

func TestLoadFromConfigFile_InvalidLogFormat(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid-log-format.yaml")
	configContent := `
log:
  format: xml
validator:
  cluster: testnet
`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	_, err := NewFromFile(configPath)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "log.format must be one of text, json, logfmt")
}

func TestLoadFromConfigFile_WithInvalidYAML(t *testing.T) {
	// Create a config file with invalid YAML
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid-config.yaml")

	configContent := `
validator:
  bin: test-validator cluster: "testnet
  invalid:yaml: content
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test LoadFromConfigFile
	cfg := &SolanaValidatorFailover{}
	err = cfg.LoadFromConfigFile(configPath)
	assert.Error(t, err)
}

func TestLoadFromConfigFile_WithComplexConfig(t *testing.T) {
	// Create a comprehensive config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "complex-config.yaml")

	configContent := `
validator:
  bin: agave-validator
  cluster: mainnet-beta
  rpc_address: https://api.mainnet-beta.solana.com
  ledger_dir: "/mnt/ledger"
  failover:
    set_identity_passive_cmd_template: "custom-set-dientity.sh passive --keyfile {{ .Identities.Passive.KeyFile }} --ledger {{ .LedgerDir }}"
    set_identity_active_cmd_template: "custom-set-dientity.sh active --keyfile {{ .Identities.Active.KeyFile }} --ledger {{ .LedgerDir }}"
    server:
      port: 12345
      heartbeat_interval: 30s
      stream_timeout: 15m
    min_time_to_leader_slot: 20s
    monitor:
      credit_samples:
        count: 15
        interval: 30s
    peers:
      peer1:
        address: peer1.private.net:9898
      peer2:
        address: peer2.private.net:9898
      peer3:
        address: peer3.private.net:9898
  tower:
    file_name_template: "custom-tower-{{ .Identities.Active.PubKey }}-{{ .Cluster }}.bin"
  identities:
    active: /keys/active.json
    passive: /keys/passive.json
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test LoadFromConfigFile
	cfg := &SolanaValidatorFailover{}
	err = cfg.LoadFromConfigFile(configPath)
	require.NoError(t, err)

	// Verify all values are loaded correctly
	assert.Equal(t, "agave-validator", cfg.Validator.Bin)
	assert.Equal(t, "mainnet-beta", cfg.Validator.Cluster)
	assert.Equal(t, "https://api.mainnet-beta.solana.com", cfg.Validator.RPCAddress)
	assert.Equal(t, "/mnt/ledger", cfg.Validator.LedgerDir)

	// Verify failover configuration
	assert.Equal(t,
		"custom-set-dientity.sh passive --keyfile {{ .Identities.Passive.KeyFile }} --ledger {{ .LedgerDir }}",
		cfg.Validator.Failover.SetIdentityPassiveCmdTemplate,
	)
	assert.Equal(t,
		"custom-set-dientity.sh active --keyfile {{ .Identities.Active.KeyFile }} --ledger {{ .LedgerDir }}",
		cfg.Validator.Failover.SetIdentityActiveCmdTemplate,
	)
	assert.Equal(t, 12345, cfg.Validator.Failover.Server.Port)
	assert.Equal(t, "30s", cfg.Validator.Failover.Server.HeartbeatInterval)
	assert.Equal(t, "15m", cfg.Validator.Failover.Server.StreamTimeout)
	assert.Equal(t, "20s", cfg.Validator.Failover.MinimumTimeToLeaderSlot)
	assert.Equal(t, 15, cfg.Validator.Failover.Monitor.CreditSamples.Count)
	assert.Equal(t, "30s", cfg.Validator.Failover.Monitor.CreditSamples.Interval)

	// Verify tower configuration
	assert.Equal(t,
		"custom-tower-{{ .Identities.Active.PubKey }}-{{ .Cluster }}.bin",
		cfg.Validator.Tower.FileNameTemplate,
	)

	// Verify identities
	assert.Equal(t, "/keys/active.json", cfg.Validator.Identities.Active)
	assert.Equal(t, "/keys/passive.json", cfg.Validator.Identities.Passive)

	// Verify peers
	assert.Len(t, cfg.Validator.Failover.Peers, 3)
	assert.Equal(t, "peer1.private.net:9898", cfg.Validator.Failover.Peers["peer1"].Address)
	assert.Equal(t, "peer2.private.net:9898", cfg.Validator.Failover.Peers["peer2"].Address)
	assert.Equal(t, "peer3.private.net:9898", cfg.Validator.Failover.Peers["peer3"].Address)
}

func TestLoadFromConfigFile_WithHomeDirectoryPath(t *testing.T) {
	// Test with a path that uses ~ (home directory)
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	tempDir := filepath.Join(homeDir, "test-config-dir")
	err = os.MkdirAll(tempDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `
validator:
  bin: home-validator
  cluster: home-testnet
`

	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Test with ~ path
	tildePath := filepath.Join("~", "test-config-dir", "config.yaml")
	cfg := &SolanaValidatorFailover{}
	err = cfg.LoadFromConfigFile(tildePath)
	require.NoError(t, err)

	assert.Equal(t, "home-validator", cfg.Validator.Bin)
	assert.Equal(t, "home-testnet", cfg.Validator.Cluster)
}
