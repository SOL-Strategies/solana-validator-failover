package validator

import (
	"time"

	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
)

// Config is the configuration for the validator
type Config struct {
	Bin           string            `mapstructure:"bin"`
	Cluster       string            `mapstructure:"cluster"`
	NetworkRPCURL  string            `mapstructure:"network_rpc_url"`
	AverageSlotTime int              `mapstructure:"average_slot_time"`
	Failover      FailoverConfig    `mapstructure:"failover"`
	Identities    identities.Config `mapstructure:"identities"`
	RPCAddress    string            `mapstructure:"rpc_address"`
	LedgerDir     string            `mapstructure:"ledger_dir"`
	Tower         TowerConfig       `mapstructure:"tower"`
	PublicIP      string            `mapstructure:"public_ip"` // subject for removal once poor-man's testing setup is removed
	Hostname      string            `mapstructure:"hostname"`  // subject for removal once poor-man's testing setup is removed
}

// TowerConfig is the configuration for the towerfile
type TowerConfig struct {
	Dir                  string `mapstructure:"dir"`
	AutoEmptyWhenPassive bool   `mapstructure:"auto_empty_when_passive"`
	FileNameTemplate     string `mapstructure:"file_name_template"`
}

// FailoverConfig is the configuration for a failover
type FailoverConfig struct {
	SetIdentityPassiveCmdTemplate string              `mapstructure:"set_identity_passive_cmd_template"`
	SetIdentityActiveCmdTemplate  string              `mapstructure:"set_identity_active_cmd_template"`
	Hooks                         hooks.FailoverHooks `mapstructure:"hooks"`
	MinimumTimeToLeaderSlot       string              `mapstructure:"min_time_to_leader_slot"`
	Monitor                       MonitorConfig       `mapstructure:"monitor"`
	Peers                         PeersConfig         `mapstructure:"peers"`
	Server                        ServerConfig        `mapstructure:"server"`
	IsDryRun                      bool
}

// PeersConfig is the configuration for the peers
type PeersConfig map[string]struct {
	Address string `mapstructure:"address"`
}

// MonitorConfig holds the configuration for a failover monitor
type MonitorConfig struct {
	CreditSamples CreditSamplesConfig `mapstructure:"credit_samples"`
}

// CreditSamplesConfig holds the configuration for a failover monitor credit samples
type CreditSamplesConfig struct {
	Count            int           `mapstructure:"count"`
	Interval         string        `mapstructure:"interval"`
	IntervalDuration time.Duration // parsed duration, set during configuration
}

// ServerConfig holds the configuration for a failover server
type ServerConfig struct {
	Port              int    `mapstructure:"port"`
	HeartbeatInterval string `mapstructure:"heartbeat_interval"`
	StreamTimeout     string `mapstructure:"stream_timeout"`
}
