package config

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/log"
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

	// DefaultAverageSlotDuration is the default average slot duration
	DefaultAverageSlotDuration = "400ms"

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
	DefaultSetIdentityPassiveCmdTemplate = "{{ if or .ThisNodeIsNativeFiredancer (eq .ThisNodeClientFamily \"frankendancer\") }}{{ .Bin }} set-identity{{ if .ClientConfigPath }} --config {{ .ClientConfigPath }}{{ end }} {{ .Identities.Passive.KeyFile }}{{ else }}{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Passive.KeyFile }}{{ end }}"

	// DefaultSetIdentityActiveCmdTemplate is the default set identity active command template for the validator
	DefaultSetIdentityActiveCmdTemplate = "{{ if or .ThisNodeIsNativeFiredancer (eq .ThisNodeClientFamily \"frankendancer\") }}{{ .Bin }} set-identity{{ if .ClientConfigPath }} --config {{ .ClientConfigPath }}{{ end }} {{ .Identities.Active.KeyFile }}{{ else }}{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Active.KeyFile }}{{ if .TowerFileAvailableAtDestination }} --require-tower{{ end }}{{ end }}"
)

var (
	// DefaultConfigPath is the default path to the config file
	DefaultConfigPath = filepath.Join("~", constants.AppName, constants.AppName+".yaml")
)

// UpdateConfig holds update-check settings
type UpdateConfig struct {
	CheckOnStartup bool `mapstructure:"check_on_startup"`
}

// SolanaValidatorFailover is the configuration for the program
type SolanaValidatorFailover struct {
	Log       LogConfig        `mapstructure:"log"`
	Validator validator.Config `mapstructure:"validator"`
	Update    UpdateConfig     `mapstructure:"update"`
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
	logger := log.WithPrefix("config")
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
	v.SetDefault("log.level", DefaultLogLevel)
	v.SetDefault("log.format", DefaultLogFormat)
	v.SetDefault("validator.bin", DefaultBin)
	v.SetDefault("validator.client.family", "auto")
	v.SetDefault("validator.client.consensus", "auto")
	v.SetDefault("validator.client.metrics_address", "http://127.0.0.1:7999")
	v.SetDefault("validator.failover.handoff.commitment", "finalized")
	v.SetDefault("validator.failover.handoff.timeout", "2m")
	v.SetDefault("validator.failover.handoff.poll_interval", "500ms")
	v.SetDefault("validator.average_slot_duration", DefaultAverageSlotDuration)
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
	v.SetDefault("update.check_on_startup", true)

	// Read config file
	logger.Debug("loading", "config_file", loadConfigPath)
	err = v.ReadInConfig()
	if err != nil {
		return
	}

	// Unmarshal into the full config structure
	if err := v.Unmarshal(s); err != nil {
		return err
	}

	return s.Log.Validate()
}
