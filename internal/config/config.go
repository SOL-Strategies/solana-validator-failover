package config

import (
	"fmt"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
	"github.com/sol-strategies/solana-validator-failover/internal/validator"
	"github.com/sol-strategies/solana-validator-failover/pkg/constants"
	"github.com/spf13/viper"
)

const (
	// DefaultBin is the default validator binary
	DefaultBin = "agave-validator"

	// DefaultCluster is the default cluster for the validator
	DefaultCluster = "testnet"

	// DefaultAverageSlotTime is the default average slot time in milliseconds
	DefaultAverageSlotTime = 400

	// DefaultFailoverServerPort is the default port for the failover server
	DefaultFailoverServerPort = 9898

	// DefaultFailoverServerHeartbeatInterval is the default heartbeat interval for the failover server
	DefaultFailoverServerHeartbeatInterval = "5s"

	// DefaultFailoverServerStreamTimeout is the default stream timeout for the failover server
	DefaultFailoverServerStreamTimeout = "10m"

	// DefaultFailoverMinimumTimeToLeaderSlot is the default minimum time to leader slot for the failover server
	DefaultFailoverMinimumTimeToLeaderSlot = "5m"

	// DefaultFailoverMonitorCreditSamplesCount is the default credit samples count for the failover server
	DefaultFailoverMonitorCreditSamplesCount = 5

	// DefaultFailoverMonitorCreditSamplesInterval is the default credit samples interval for the failover server
	DefaultFailoverMonitorCreditSamplesInterval = "5s"

	// DefaultTowerFileNameTemplate is the default tower file name template for the validator
	DefaultTowerFileNameTemplate = "tower-1_9-{{ .Identities.Active.PubKey }}.bin"

	// DefaultSetIdentityPassiveCmdTemplate is the default set identity passive command template for the validator
	DefaultSetIdentityPassiveCmdTemplate = "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Passive.KeyFile }}"

	// DefaultSetIdentityActiveCmdTemplate is the default set identity active command template for the validator
	DefaultSetIdentityActiveCmdTemplate = "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Active.KeyFile }} --require-tower"
)

var (
	// DefaultConfigPath is the default path to the config file
	DefaultConfigPath = filepath.Join("~", constants.AppName, constants.AppName+".yaml")
)

// SolanaValidatorFailover is the configuration for the program
type SolanaValidatorFailover struct {
	Validator validator.Config `mapstructure:"validator"`
}

// NewFromFile creates a new SolanaValidatorFailover configuration from a config file
func NewFromFile(configPath string) (s *SolanaValidatorFailover, err error) {
	s = &SolanaValidatorFailover{}

	err = s.LoadFromConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	return
}

// LoadFromConfigFile loads the config from a config file
func (s *SolanaValidatorFailover) LoadFromConfigFile(configPath string) (err error) {
	logger := log.With().Str("component", "config").Logger()
	v := viper.New()

	loadConfigPath := DefaultConfigPath

	if configPath != "" {
		loadConfigPath = configPath
	}

	loadConfigPath, err = utils.ResolvePath(loadConfigPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	v.SetConfigFile(loadConfigPath)

	// Set defaults
	v.SetDefault("validator.bin", DefaultBin)
	v.SetDefault("validator.average_slot_time", DefaultAverageSlotTime)
	v.SetDefault("validator.cluster", DefaultCluster)
	v.SetDefault("validator.failover.min_time_to_leader_slot", DefaultFailoverMinimumTimeToLeaderSlot)
	v.SetDefault("validator.failover.monitor.credit_samples.count", DefaultFailoverMonitorCreditSamplesCount)
	v.SetDefault("validator.failover.monitor.credit_samples.interval", DefaultFailoverMonitorCreditSamplesInterval)
	v.SetDefault("validator.failover.server.heartbeat_interval", DefaultFailoverServerHeartbeatInterval)
	v.SetDefault("validator.failover.server.port", DefaultFailoverServerPort)
	v.SetDefault("validator.failover.server.stream_timeout", DefaultFailoverServerStreamTimeout)
	v.SetDefault("validator.failover.set_identity_active_cmd_template", DefaultSetIdentityActiveCmdTemplate)
	v.SetDefault("validator.failover.set_identity_passive_cmd_template", DefaultSetIdentityPassiveCmdTemplate)
	v.SetDefault("validator.tower.file_name_template", DefaultTowerFileNameTemplate)

	// Read config file
	logger.Debug().Str("config_file", loadConfigPath).Msg("loading")
	err = v.ReadInConfig()
	if err != nil {
		return
	}

	// Unmarshal into the full config structure
	return v.Unmarshal(&s)
}
