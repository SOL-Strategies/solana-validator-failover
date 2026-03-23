package validator

import (
	"context"
	gotls "crypto/tls"
	"fmt"
	"html/template"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/internal/constants"
	"github.com/sol-strategies/solana-validator-failover/internal/failover"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/sol-strategies/solana-validator-failover/internal/solana"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
	pkgconstants "github.com/sol-strategies/solana-validator-failover/pkg/constants"
)

// FailoverParams are the parameters for running a failover
type FailoverParams struct {
	NotADrill             bool
	NoWaitForHealthy      bool
	NoMinTimeToLeaderSlot bool
	MinTimeToLeaderSlot   time.Duration
	SkipTowerSync         bool
	AutoConfirm           bool   // -y/--yes: skip all interactive confirmations
	ToPeer                string // --to-peer: auto-select peer by name or IP (active node only)
	RollbackEnabled       bool   // --rollback-enabled/-r: force-enable rollback regardless of config
}

// Peers is a map of peers
type Peers map[string]Peer

// Peer is a peer in the failover configuration
type Peer struct {
	Name    string
	Address string
}

// BinMetadata is the metadata for a validator client
type BinMetadata struct {
	Client  string
	Version string
}

// Validator is a validator that uses the new QUIC protocol
type Validator struct {
	Bin                            string
	BinMetadata                    BinMetadata
	FailoverServerConfig           ServerConfig
	MonitorConfig                  MonitorConfig
	GossipNode                     *solana.Node
	Hooks                          hooks.FailoverHooks
	Hostname                       string
	Identities                     *identities.Identities
	LedgerDir                      string
	MinimumTimeToLeaderSlot        time.Duration
	Peers                          Peers
	PublicIP                       string
	RPCAddress                     string
	SetIdentityActiveCommand       string
	SetIdentityPassiveCommand      string
	TowerFile                      string
	TowerFileAutoDeleteWhenPassive bool
	Rollback                       hooks.RollbackConfig

	logger          *log.Logger
	solanaRPCClient solana.ClientInterface
	serverTLSConfig *gotls.Config // non-nil when mTLS is enabled; used by the passive QUIC server
	clientTLSConfig *gotls.Config // non-nil when mTLS is enabled; used by the active QUIC client
}

// NewSolanaRPCClient creates a new Solana RPC client
func (v *Validator) NewSolanaRPCClient(params solana.NewClientParams) solana.ClientInterface {
	return solana.NewRPCClient(params)
}

// NewFromConfig creates a new validator from a config
func NewFromConfig(cfg *Config) (*Validator, error) {
	validator := &Validator{
		logger: log.WithPrefix("validator"),
	}
	err := validator.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return validator, nil
}

// NewFromConfig initializes the validator from a config
func (v *Validator) NewFromConfig(cfg *Config) error {

	log.Debug("================================================")
	v.logger.Debug("configuring...")
	defer log.Debug("================================================")
	defer v.logger.Debug("configuration done")

	// configure solana rpc clients all in one
	err := v.configureRPCClient(cfg.RPCAddress, cfg.Cluster, cfg.ClusterRPCURL, cfg.AverageSlotDuration)
	if err != nil {
		return err
	}

	// ensure supplied validator binary exists
	err = v.configureBin(cfg.Bin)
	if err != nil {
		return err
	}

	// ledger dir must be valid and exist
	err = v.configureLedgerDir(cfg.LedgerDir)
	if err != nil {
		return err
	}

	// configure identities
	err = v.configureIdentities(cfg.Identities)
	if err != nil {
		return err
	}

	// tower file configure
	err = v.configureTowerFile(cfg.Tower)
	if err != nil {
		return err
	}

	// set identity commands configure
	err = v.configureSetIdenttiyCommands(cfg.Failover)
	if err != nil {
		return err
	}

	// configure hooks
	err = v.configureHooks(cfg.Failover)
	if err != nil {
		return err
	}

	// configure rollback
	err = v.configureRollback(cfg.Failover)
	if err != nil {
		return err
	}

	// must have at least one peer, each peer must have a valid string <host>:<port>
	err = v.configurePeers(cfg.Failover.Peers)
	if err != nil {
		return err
	}

	// get public ip
	err = v.configurePublicIP(cfg.PublicIP)
	if err != nil {
		return err
	}

	// get minimum time to leader slot parse and set
	err = v.configureMinimumTimeToLeaderSlot(cfg.Failover.MinimumTimeToLeaderSlot)
	if err != nil {
		return err
	}

	// get hostname
	err = v.configureHostname(cfg.Name)
	if err != nil {
		return err
	}

	// get gossip node
	err = v.configureGossipNode()
	if err != nil {
		return err
	}

	// get server
	err = v.configureServer(cfg.Failover.Server, cfg.Failover.Monitor)
	if err != nil {
		return err
	}

	// configure mTLS (no-op when tls.enabled is false)
	err = v.configureTLS(cfg.Failover.TLS)
	if err != nil {
		return err
	}

	return nil
}

// IsActive returns true if the validator is active
func (v *Validator) IsActive() bool {
	return v.GossipNode.PubKey() == v.Identities.Active.PubKey()
}

// IsPassive returns true if the validator is passive
func (v *Validator) IsPassive() bool {
	return v.GossipNode.PubKey() == v.Identities.Passive.PubKey()
}

// Failover runs the failover process
func (v *Validator) Failover(params FailoverParams) (err error) {
	log.Debug("running failover")
	defer log.Debug("run failover done")

	log.Debugf("failover with params: %+v", params)

	// wait until healthy unless told otherwise
	if params.NoWaitForHealthy {
		log.Debug("--no-wait-for-healthy flag is set, skipping wait for healthy")
	} else {
		err = v.waitUntilHealthy()
		if err != nil {
			return fmt.Errorf("failed to wait until healthy: %w", err)
		}
	}

	params.MinTimeToLeaderSlot = v.MinimumTimeToLeaderSlot

	if params.RollbackEnabled && !v.Rollback.Enabled {
		log.Debug("--rollback-enabled flag set: overriding rollback.enabled to true")
		v.Rollback.Enabled = true
	}

	if v.IsActive() {
		if params.AutoConfirm && params.ToPeer != "" {
			log.Warn("non-interactive mode: --yes and --to-peer are both set, all prompts will be skipped")
		}
		if params.AutoConfirm {
			// --yes has no confirmations to skip on the active path, but it's not an error
			log.Debug("--yes flag set (active node: no confirmations in this path)")
		}
		return v.makePassive(params)
	}

	// passive node path
	if params.ToPeer != "" {
		log.Warn("--to-peer flag is only applicable when run on an active node - ignoring", "to_peer", params.ToPeer)
	}
	return v.makeActive(params)
}

// configureRPCClient configures the solana rpc client
func (v *Validator) configureRPCClient(localRPCURL, solanaClusterName, clusterRPCURL, averageSlotDuration string) error {
	if solanaClusterName == "" {
		return fmt.Errorf("cluster is required")
	}

	if !utils.IsValidURLWithPort(localRPCURL) {
		return fmt.Errorf(
			"invalid rpc address: %s, must be a valid url with a port",
			localRPCURL,
		)
	}

	// determine the cluster RPC URL: use built-in URL for known clusters,
	// otherwise require cluster_rpc_url from config
	var solanaClusterRPCURL string
	if utils.IsKnownCluster(solanaClusterName) {
		solanaClusterRPCURL = constants.SolanaClusters[solanaClusterName].RPC
	} else {
		if clusterRPCURL == "" {
			return fmt.Errorf(
				"cluster_rpc_url is required for custom cluster %q (known clusters: %s)",
				solanaClusterName,
				strings.Join(constants.SolanaClusterNames, ", "),
			)
		}
		solanaClusterRPCURL = clusterRPCURL
	}

	avgSlotDuration, err := time.ParseDuration(averageSlotDuration)
	if err != nil {
		return fmt.Errorf("invalid average_slot_duration %q: %w", averageSlotDuration, err)
	}

	v.logger.Debug("rpc client configured",
		"cluster", solanaClusterName,
		"local_rpc_url", localRPCURL,
		"cluster_rpc_url", solanaClusterRPCURL,
	)

	v.RPCAddress = localRPCURL
	v.solanaRPCClient = v.NewSolanaRPCClient(solana.NewClientParams{
		LocalRPCURL:         localRPCURL,
		ClusterRPCURL:       solanaClusterRPCURL,
		AverageSlotDuration: avgSlotDuration,
	})

	return nil
}

// configureBin ensures the validator binary exists and sets it
func (v *Validator) configureBin(bin string) error {
	err := utils.EnsureBins(bin)
	if err != nil {
		return err
	}
	v.Bin = bin
	v.logger.Debug("validator binary set", "bin", v.Bin)
	return nil
}

// configureLedgerDir ensures the ledger directory exists
func (v *Validator) configureLedgerDir(ledgerDir string) error {
	ledgerDir, err := utils.ResolveAndValidateDir(ledgerDir)
	if err != nil {
		return err
	}
	v.LedgerDir = ledgerDir
	v.logger.Debug("ledger dir set", "ledger_dir", v.LedgerDir)
	return nil
}

// configureIdentities ensures the identities are valid and sets them
func (v *Validator) configureIdentities(identitiesConfig identities.Config) (err error) {
	v.Identities, err = identities.NewFromConfig(&identitiesConfig)
	if err != nil {
		return err
	}

	v.logger.Debug("identities set",
		"active_pubkey", v.Identities.Active.PubKey(),
		"active_keyfile", v.Identities.Active.KeyFile,
		"passive_pubkey", v.Identities.Passive.PubKey(),
		"passive_keyfile", v.Identities.Passive.KeyFile,
	)

	return nil
}

// configureTowerFile ensures the tower file is valid and sets it
func (v *Validator) configureTowerFile(cfg TowerConfig) error {
	v.TowerFileAutoDeleteWhenPassive = cfg.AutoEmptyWhenPassive
	v.logger.Debug("tower file auto delete when passive set",
		"tower_file_auto_delete_when_passive", v.TowerFileAutoDeleteWhenPassive,
	)

	// tower dir must exist
	towerDir, err := utils.ResolveAndValidateDir(cfg.Dir)
	if err != nil {
		return err
	}

	// tower file name template must be valid
	towerFileNameTemplate, err := template.New("tower").Parse(cfg.FileNameTemplate)
	if err != nil {
		return fmt.Errorf(
			"failed to parse file name template %s: %w",
			cfg.FileNameTemplate,
			err,
		)
	}
	v.logger.Debug("tower file name template set", "template", cfg.FileNameTemplate)

	// tower file name template must compile
	var towerFileNameBuf strings.Builder
	if err := towerFileNameTemplate.Execute(&towerFileNameBuf, v); err != nil {
		return fmt.Errorf(
			"failed to execute file name template %s: %w",
			cfg.FileNameTemplate,
			err,
		)
	}

	v.TowerFile = filepath.Join(towerDir, towerFileNameBuf.String())
	v.logger.Debug("tower file set", "tower_file", v.TowerFile)

	return nil
}

// configureSetIdenttiyCommands ensures the set identity commands are valid and sets them
func (v *Validator) configureSetIdenttiyCommands(cfg FailoverConfig) (err error) {
	var (
		setIdentityActiveCmdBuf  strings.Builder
		setIdentityPassiveCmdBuf strings.Builder
	)

	// parse active command template
	setIdentityActiveCmdTemplate, err := template.New("set_identity_active_cmd").
		Parse(cfg.SetIdentityActiveCmdTemplate)
	if err != nil {
		return fmt.Errorf(
			"failed to parse set identity active cmd template %s: %w",
			cfg.SetIdentityActiveCmdTemplate,
			err,
		)
	}
	v.logger.Debug("set identity active command template set", "template", cfg.SetIdentityActiveCmdTemplate)

	// set identity active command must compile
	if err := setIdentityActiveCmdTemplate.Execute(&setIdentityActiveCmdBuf, v); err != nil {
		return fmt.Errorf(
			"failed to execute set identity active cmd template %s: %w",
			cfg.SetIdentityActiveCmdTemplate,
			err,
		)
	}

	// set identity active command
	v.SetIdentityActiveCommand = setIdentityActiveCmdBuf.String()
	v.logger.Debug("set identity active command set", "command", v.SetIdentityActiveCommand)

	// parse passive command template
	setIdentityPassiveCmdTemplate, err := template.New("set_identity_passive_cmd").
		Parse(cfg.SetIdentityPassiveCmdTemplate)
	if err != nil {
		return fmt.Errorf(
			"failed to parse set identity passive cmd template %s: %w",
			cfg.SetIdentityPassiveCmdTemplate,
			err,
		)
	}
	v.logger.Debug("set identity passive command template set", "template", cfg.SetIdentityPassiveCmdTemplate)

	// set identity passive command must compile
	if err := setIdentityPassiveCmdTemplate.Execute(&setIdentityPassiveCmdBuf, v); err != nil {
		return fmt.Errorf(
			"failed to execute set identity passive cmd template %s: %w",
			cfg.SetIdentityPassiveCmdTemplate,
			err,
		)
	}
	v.SetIdentityPassiveCommand = setIdentityPassiveCmdBuf.String()
	v.logger.Debug("set identity passive command set", "command", v.SetIdentityPassiveCommand)

	// if the commands are the same, warn - could be intentional or a mistake
	if v.SetIdentityActiveCommand == v.SetIdentityPassiveCommand {
		log.Warn("set identity active and passive commands are the same - this could be intentional or a mistake")
	}

	return nil
}

// configureHooks ensures the hooks are valid and sets them
func (v *Validator) configureHooks(cfg FailoverConfig) (err error) {
	v.Hooks = cfg.Hooks
	v.logger.Debug("hooks set",
		"pre_when_active", len(v.Hooks.Pre.WhenActive),
		"pre_when_passive", len(v.Hooks.Pre.WhenPassive),
		"post_when_active", len(v.Hooks.Post.WhenActive),
		"post_when_passive", len(v.Hooks.Post.WhenPassive),
	)
	for _, h := range v.Hooks.Pre.WhenActive {
		v.logger.Debug("pre hook (when active)", "name", h.Name, "command", h.Command, "args", h.Args, "must_succeed", h.MustSucceed)
	}
	for _, h := range v.Hooks.Pre.WhenPassive {
		v.logger.Debug("pre hook (when passive)", "name", h.Name, "command", h.Command, "args", h.Args, "must_succeed", h.MustSucceed)
	}
	for _, h := range v.Hooks.Post.WhenActive {
		v.logger.Debug("post hook (when active)", "name", h.Name, "command", h.Command, "args", h.Args, "must_succeed", h.MustSucceed)
	}
	for _, h := range v.Hooks.Post.WhenPassive {
		v.logger.Debug("post hook (when passive)", "name", h.Name, "command", h.Command, "args", h.Args, "must_succeed", h.MustSucceed)
	}
	return nil
}

// configureRollback resolves rollback command templates and stores the fully-expanded
// rollback commands alongside any configured rollback hooks.
// If a cmd_template is empty, it falls back to the corresponding set-identity command.
func (v *Validator) configureRollback(cfg FailoverConfig) error {
	v.Rollback.Enabled = cfg.Rollback.Enabled
	v.Rollback.ToActive.Hooks = cfg.Rollback.ToActive.Hooks
	v.Rollback.ToPassive.Hooks = cfg.Rollback.ToPassive.Hooks

	// resolve to-active rollback command
	if cfg.Rollback.ToActive.CmdTemplate == "" {
		v.Rollback.ToActive.ResolvedCmd = v.SetIdentityActiveCommand
	} else {
		tpl, err := template.New("rollback_to_active_cmd").Parse(cfg.Rollback.ToActive.CmdTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse rollback.to_active.cmd_template: %w", err)
		}
		var buf strings.Builder
		if err := tpl.Execute(&buf, v); err != nil {
			return fmt.Errorf("failed to execute rollback.to_active.cmd_template: %w", err)
		}
		v.Rollback.ToActive.ResolvedCmd = buf.String()
	}

	// resolve to-passive rollback command
	if cfg.Rollback.ToPassive.CmdTemplate == "" {
		v.Rollback.ToPassive.ResolvedCmd = v.SetIdentityPassiveCommand
	} else {
		tpl, err := template.New("rollback_to_passive_cmd").Parse(cfg.Rollback.ToPassive.CmdTemplate)
		if err != nil {
			return fmt.Errorf("failed to parse rollback.to_passive.cmd_template: %w", err)
		}
		var buf strings.Builder
		if err := tpl.Execute(&buf, v); err != nil {
			return fmt.Errorf("failed to execute rollback.to_passive.cmd_template: %w", err)
		}
		v.Rollback.ToPassive.ResolvedCmd = buf.String()
	}

	v.logger.Debug("rollback configured",
		"enabled", v.Rollback.Enabled,
		"to_active_cmd", v.Rollback.ToActive.ResolvedCmd,
		"to_passive_cmd", v.Rollback.ToPassive.ResolvedCmd,
	)

	return nil
}

// configurePeers ensures the peers are valid and sets them
func (v *Validator) configurePeers(cfg PeersConfig) (err error) {
	if len(cfg) == 0 {
		return fmt.Errorf("must have at least one peer")
	}

	v.Peers = make(Peers)
	for name, peer := range cfg {
		if !utils.IsValidURLWithPort(peer.Address) {
			return fmt.Errorf(
				"invalid peer address %s for peer %s - must be a valid url with a port",
				peer.Address,
				name,
			)
		}
		v.Peers[name] = Peer{
			Name:    name,
			Address: peer.Address,
		}
		log.Debug("registered peer", "name", name, "address", peer.Address)
	}

	return nil
}

// GetPublicIP returns the public IP address - can be overridden in tests
func (v *Validator) GetPublicIP() (string, error) {
	return utils.GetPublicIP()
}

// configurePublicIP ensures the public ip is valid and sets it
func (v *Validator) configurePublicIP(publicIP string) (err error) {
	if publicIP != "" {
		v.PublicIP = publicIP
		v.logger.Debug(
			"public ip set in config - not recommended and actually a dirty hack for testing, likely to break and/or be removed in the future",
			"public_ip", v.PublicIP,
		)
		return nil
	}

	v.PublicIP, err = v.GetPublicIP()
	if err != nil {
		return err
	}

	v.logger.Debug("public ip set", "public_ip", v.PublicIP)

	return nil
}

// configureMinimumTimeToLeaderSlot ensures the minimum time to leader slot is valid and sets it
func (v *Validator) configureMinimumTimeToLeaderSlot(timeToLeaderSlotDurationString string) (err error) {
	minimumTimeToLeaderSlotDuration, err := time.ParseDuration(timeToLeaderSlotDurationString)
	if err != nil {
		return fmt.Errorf(
			"failed to parse minimum time to leader slot %s: %w",
			timeToLeaderSlotDurationString,
			err,
		)
	}
	v.MinimumTimeToLeaderSlot = minimumTimeToLeaderSlotDuration
	v.logger.Debug("minimum time to leader slot set", "minimum_time_to_leader_slot", v.MinimumTimeToLeaderSlot.String())
	return nil
}

// GetHostname returns the hostname - can be overridden in tests
func (v *Validator) GetHostname() (string, error) {
	return os.Hostname()
}

// configureHostname sets the node's display name from validator.name if provided,
// otherwise falls back to the OS hostname.
func (v *Validator) configureHostname(name string) (err error) {
	if name != "" {
		v.Hostname = name
		v.logger.Debug("name set from config", "name", v.Hostname)
		return nil
	}

	v.Hostname, err = v.GetHostname()
	if err != nil {
		return err
	}
	v.logger.Debug("hostname set from OS", "hostname", v.Hostname)
	return nil
}

// configureServer ensures the server is valid and sets it
func (v *Validator) configureServer(cfg ServerConfig, monitorCfg MonitorConfig) (err error) {
	v.FailoverServerConfig = cfg

	// validate monitor configuration early
	if monitorCfg.CreditSamples.Count < 1 {
		return fmt.Errorf("credit samples count must be >= 1, got %d", monitorCfg.CreditSamples.Count)
	}

	if monitorCfg.CreditSamples.Interval == "" {
		return fmt.Errorf("credit samples interval cannot be empty")
	}

	// validate that the interval can be parsed and store the parsed duration
	duration, err := time.ParseDuration(monitorCfg.CreditSamples.Interval)
	if err != nil {
		return fmt.Errorf("invalid credit samples interval %q: %v", monitorCfg.CreditSamples.Interval, err)
	}

	// store the monitor config with parsed duration
	monitorCfg.CreditSamples.IntervalDuration = duration
	v.MonitorConfig = monitorCfg

	v.logger.Debug("server and monitor config set",
		"port", v.FailoverServerConfig.Port,
		"credit_samples_count", v.MonitorConfig.CreditSamples.Count,
		"credit_samples_interval", v.MonitorConfig.CreditSamples.Interval,
	)
	return nil
}

// configureTLS validates and loads mTLS material when tls.enabled is true.
// When disabled (the default), it is a no-op and the QUIC layer falls back
// to an ephemeral self-signed certificate (encrypted but unauthenticated).
func (v *Validator) configureTLS(cfg TLSConfig) error {
	if !cfg.Enabled {
		v.logger.Debug("mTLS disabled; QUIC connections use an ephemeral self-signed certificate (encrypted but unauthenticated)")
		return nil
	}

	if cfg.CACert == "" {
		return fmt.Errorf("tls.ca_cert is required when tls.enabled is true")
	}
	if cfg.Cert == "" {
		return fmt.Errorf("tls.cert is required when tls.enabled is true")
	}
	if cfg.Key == "" {
		return fmt.Errorf("tls.key is required when tls.enabled is true")
	}

	caCertPath, err := utils.ResolvePath(cfg.CACert)
	if err != nil {
		return fmt.Errorf("tls.ca_cert: failed to resolve path: %w", err)
	}
	certPath, err := utils.ResolvePath(cfg.Cert)
	if err != nil {
		return fmt.Errorf("tls.cert: failed to resolve path: %w", err)
	}
	keyPath, err := utils.ResolvePath(cfg.Key)
	if err != nil {
		return fmt.Errorf("tls.key: failed to resolve path: %w", err)
	}

	serverTLS, err := utils.BuildMTLSServerConfig(caCertPath, certPath, keyPath)
	if err != nil {
		return fmt.Errorf("tls: failed to build server TLS config: %w", err)
	}

	clientTLS, err := utils.BuildMTLSClientConfig(caCertPath, certPath, keyPath)
	if err != nil {
		return fmt.Errorf("tls: failed to build client TLS config: %w", err)
	}

	v.serverTLSConfig = serverTLS
	v.clientTLSConfig = clientTLS

	v.logger.Info("mTLS enabled: certificate and CA loaded successfully",
		"ca_cert", caCertPath,
		"cert", certPath,
	)

	return nil
}

// getLocalNodeVersion returns the solana-core version from the local validator RPC (best-effort).
// Returns an empty string if the call fails, so callers can treat it as "not available".
func (v *Validator) getLocalNodeVersion() string {
	version, err := v.solanaRPCClient.GetLocalNodeVersion()
	if err != nil {
		v.logger.Warn("failed to get local node version from RPC - ClientVersionRPC will be empty", "error", err)
		return ""
	}
	return version
}

// configureGossipNode ensures the gossip node is valid and sets it
func (v *Validator) configureGossipNode() (err error) {
	v.GossipNode, err = v.solanaRPCClient.NodeFromIP(v.PublicIP)
	if err != nil {
		return err
	}
	v.logger.Debug("gossip node set",
		"public_ip", v.GossipNode.IP(),
		"pubkey", v.GossipNode.PubKey(),
	)
	return nil
}

// makeActive makes this validator active
func (v *Validator) makeActive(params FailoverParams) (err error) {
	log.Debug("making this validator active")

	if v.IsActive() {
		return fmt.Errorf("this validator is already active - nothing to do")
	}

	log.Info(
		style.RenderPinkString("this validator is currently ")+style.RenderPassiveString(constants.NodeRolePassive, false),
		"public_ip", v.PublicIP,
		"pubkey", v.Identities.Passive.PubKey(),
	)

	// check gossip for active peer and ensure its pubkey is the same as what this node would set itself to
	_, err = v.solanaRPCClient.NodeFromPubkey(v.Identities.Active.PubKey())
	if err != nil {
		return fmt.Errorf(
			"active peer not found in gossip with pubkey %s from file %s: %w",
			v.Identities.Active.PubKey(),
			v.Identities.Active.KeyFile,
			err,
		)
	}

	// delete the tower file if it exists and auto empty when passive is true
	if v.TowerFileAutoDeleteWhenPassive && utils.FileExists(v.TowerFile) {
		log.Debug("deleting tower file because validator.tower.auto_empty_when_passive is true",
			"tower_file", v.TowerFile,
		)

		if err = utils.RemoveFile(v.TowerFile); err != nil {
			return err
		}
	}

	// if the tower file exists and auto empty when passive is false, confirm if you want it deleted and exit if not.
	if !v.TowerFileAutoDeleteWhenPassive && utils.FileExists(v.TowerFile) {
		log.Warn("tower file exists", "tower_file", v.TowerFile)
		if params.AutoConfirm {
			log.Warn("--yes flag set, automatically deleting tower file", "tower_file", v.TowerFile)
		} else {
			confirmed, err := confirm("Delete tower file and proceed?")
			if err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("cancelled")
			}
		}
		// delete the tower file
		if err = utils.RemoveFile(v.TowerFile); err != nil {
			return err
		}
	}

	// create a QUIC server that listens for the active node to connect and decide what to do
	failoverServer, err := failover.NewServerFromConfig(failover.ServerConfig{
		Port:              v.FailoverServerConfig.Port,
		HeartbeatInterval: v.FailoverServerConfig.HeartbeatInterval,
		StreamTimeout:     v.FailoverServerConfig.StreamTimeout,
		PassiveNodeInfo: &failover.NodeInfo{
			Hostname:                       v.Hostname,
			PublicIP:                       v.PublicIP,
			Identities:                     v.Identities,
			TowerFile:                      v.TowerFile,
			SetIdentityCommand:             v.SetIdentityActiveCommand,
			ClientVersion:                  v.GossipNode.Version(),
			ClientVersionRPC:               v.getLocalNodeVersion(),
			SolanaValidatorFailoverVersion: pkgconstants.AppVersion,
			RPCAddress:                     v.RPCAddress,
		},
		SolanaRPCClient:  v.solanaRPCClient,
		RPCURL:           v.RPCAddress,
		IsDryRunFailover: !params.NotADrill,
		Hooks:            v.Hooks,
		Rollback:         v.Rollback,
		SkipTowerSync:    params.SkipTowerSync,
		AutoConfirm:      params.AutoConfirm,
		TLSConfig:        v.serverTLSConfig,
		MonitorConfig: failover.MonitorConfig{
			CreditSamples: failover.CreditSamplesConfig{
				Count:            v.MonitorConfig.CreditSamples.Count,
				Interval:         v.MonitorConfig.CreditSamples.Interval,
				IntervalDuration: v.MonitorConfig.CreditSamples.IntervalDuration,
			},
		},
	})
	if err != nil {
		return err
	}

	failoverServer.Start()

	return nil
}

// makePassive makes this validator passive
func (v *Validator) makePassive(params FailoverParams) (err error) {
	if v.IsPassive() {
		return fmt.Errorf("this validator is already passive - nothing to do")
	}

	log.Info(
		style.RenderPinkString("this validator is currently ")+style.RenderActiveString(constants.NodeRoleActive, false),
		"public_ip", v.PublicIP,
		"pubkey", v.Identities.Active.PubKey(),
	)

	log.Debug("failover active to passive")

	// ensure tower file exists and is not empty
	if !utils.FileExists(v.TowerFile) {
		return fmt.Errorf("tower file does not exist: %s", v.TowerFile)
	}

	if utils.FileSize(v.TowerFile) == 0 {
		return fmt.Errorf("tower file is empty: %s", v.TowerFile)
	}

	// select passive peer to connect to from declared peers
	selectedPassivePeer, err := v.selectPassivePeer(params)
	if err != nil {
		return err
	}

	// connect to the passive peer and follow its lead to handover as active
	failoverClient, err := failover.NewClientFromConfig(failover.ClientConfig{
		ServerName:                     selectedPassivePeer.Name,
		ServerAddress:                  selectedPassivePeer.Address,
		MinTimeToLeaderSlot:            params.MinTimeToLeaderSlot,
		WaitMinTimeToLeaderSlotEnabled: !params.NoMinTimeToLeaderSlot,
		SolanaRPCClient:                v.solanaRPCClient,
		RPCURL:                         v.RPCAddress,
		SkipTowerSync:                  params.SkipTowerSync,
		ActiveNodeInfo: &failover.NodeInfo{
			Hostname:                       v.Hostname,
			PublicIP:                       v.PublicIP,
			Identities:                     v.Identities,
			TowerFile:                      v.TowerFile,
			TowerFileSizeBytes:             utils.FileSize(v.TowerFile),
			SetIdentityCommand:             v.SetIdentityPassiveCommand,
			ClientVersion:                  v.GossipNode.Version(),
			ClientVersionRPC:               v.getLocalNodeVersion(),
			SolanaValidatorFailoverVersion: pkgconstants.AppVersion,
			RPCAddress:                     v.RPCAddress,
		},
		Hooks:     v.Hooks,
		Rollback:  v.Rollback,
		TLSConfig: v.clientTLSConfig,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", selectedPassivePeer.Name, err)
	}

	failoverClient.Start()

	return nil
}

// waitUntilHealthy waits until the validator is healthy and synced
func (v *Validator) waitUntilHealthy() (err error) {
	startTime := time.Now()
	sp := spinner.New().
		TitleStyle(style.SpinnerTitleStyle).
		Title("waiting for validator to be healthy and synced...")

	sp.ActionWithErr(func(ctx context.Context) error {
		for {
			if !v.solanaRPCClient.IsLocalNodeHealthy() {
				sp.Title(
					style.RenderWarningString(
						"waiting for validator to report healthy...",
					),
				)
				time.Sleep(2 * time.Second)
				continue
			}

			sp.Title(style.RenderPinkString(fmt.Sprintf("validator is healthy and synced - elapsed time %s", time.Since(startTime).String())))
			return nil
		}
	})

	return sp.Run()
}

// selectPassivePeer allows selection of a peer from the list of peers.
// When params.ToPeer is set, it auto-selects by name or IP without an interactive prompt.
func (v *Validator) selectPassivePeer(params FailoverParams) (selectedPeer Peer, err error) {
	if params.ToPeer != "" {
		// match by name first
		if peer, ok := v.Peers[params.ToPeer]; ok {
			log.Info("--to-peer: auto-selected peer by name", "peer", peer.Name, "address", peer.Address)
			return peer, nil
		}
		// match by IP (Address is "host:port")
		for _, peer := range v.Peers {
			host, _, splitErr := net.SplitHostPort(peer.Address)
			if splitErr != nil {
				continue
			}
			if host == params.ToPeer {
				log.Info("--to-peer: auto-selected peer by IP", "peer", peer.Name, "address", peer.Address)
				return peer, nil
			}
		}
		return selectedPeer, fmt.Errorf("--to-peer: no peer found matching %q (checked names and IP addresses)", params.ToPeer)
	}

	// no --to-peer: fall back to interactive selector
	huhPeerOptions := make([]huh.Option[string], 0)
	for name, peer := range v.Peers {
		selectionKey := style.RenderPassiveString(name, false)
		if log.GetLevel() == log.DebugLevel {
			selectionKey = fmt.Sprintf(
				"%s %s",
				style.RenderPassiveString(name, false),
				style.RenderGreyString(peer.Address, false),
			)
		}
		huhPeerOptions = append(huhPeerOptions, huh.NewOption(selectionKey, name))
	}

	var selectedPeerName string

	err = huh.NewSelect[string]().
		Title("Select a passive peer to failover to:").
		Options(huhPeerOptions...).
		Value(&selectedPeerName).
		Run()

	if err != nil {
		return selectedPeer, fmt.Errorf("failed to select peer: %w", err)
	}

	log.Debugf("selected peer: %s address: %s", selectedPeerName, v.Peers[selectedPeerName].Address)

	return v.Peers[selectedPeerName], nil
}

func confirm(title string) (confirm bool, err error) {
	// ask to proceed
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Value(&confirm),
		),
	)

	err = form.Run()
	if err != nil {
		return false, err
	}

	if !confirm {
		return false, fmt.Errorf("cancelled")
	}

	return true, nil
}
