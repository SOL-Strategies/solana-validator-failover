package validator

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

	logger          zerolog.Logger
	solanaRPCClient solana.ClientInterface
}

// NewSolanaRPCClient creates a new Solana RPC client
func (v *Validator) NewSolanaRPCClient(params solana.NewClientParams) solana.ClientInterface {
	return solana.NewRPCClient(params)
}

// NewFromConfig creates a new validator from a config
func NewFromConfig(cfg *Config) (*Validator, error) {
	validator := &Validator{
		logger: log.With().Str("component", "validator").Logger(),
	}
	err := validator.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return validator, nil
}

// NewFromConfig initializes the validator from a config
func (v *Validator) NewFromConfig(cfg *Config) error {

	log.Debug().Msg("================================================")
	v.logger.Debug().Msg("configuring...")
	defer log.Debug().Msg("================================================")
	defer v.logger.Debug().Msg("configuration done")

	// configure solana rpc clients all in one
	err := v.configureRPCClient(cfg.RPCAddress, cfg.Cluster, cfg.ClusterRPCURL, cfg.AverageSlotTime)
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
	err = v.configureHostname(cfg.Hostname)
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
	log.Debug().Msg("running failover")
	defer log.Debug().Msg("run failover done")

	log.Debug().Msgf("failover with params: %+v", params)

	// wait until healthy unless told otherwise
	if params.NoWaitForHealthy {
		log.Debug().Msg("--no-wait-for-healthy flag is set, skipping wait for healthy")
	} else {
		err = v.waitUntilHealthy()
		if err != nil {
			return fmt.Errorf("failed to wait until healthy: %w", err)
		}
	}

	params.MinTimeToLeaderSlot = v.MinimumTimeToLeaderSlot

	if v.IsActive() {
		return v.makePassive(params)
	}

	return v.makeActive(params)
}

// configureRPCClient configures the solana rpc client
func (v *Validator) configureRPCClient(localRPCURL, solanaClusterName, clusterRPCURL string, averageSlotTime int) error {
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

	v.logger.Debug().
		Str("cluster", solanaClusterName).
		Str("local_rpc_url", localRPCURL).
		Str("cluster_rpc_url", solanaClusterRPCURL).
		Msg("rpc client configured")

	v.RPCAddress = localRPCURL
	v.solanaRPCClient = v.NewSolanaRPCClient(solana.NewClientParams{
		LocalRPCURL:     localRPCURL,
		ClusterRPCURL:   solanaClusterRPCURL,
		AverageSlotTime: averageSlotTime,
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
	v.logger.Debug().
		Str("bin", v.Bin).
		Msg("validator binary set")
	return nil
}

// configureLedgerDir ensures the ledger directory exists
func (v *Validator) configureLedgerDir(ledgerDir string) error {
	ledgerDir, err := utils.ResolveAndValidateDir(ledgerDir)
	if err != nil {
		return err
	}
	v.LedgerDir = ledgerDir
	v.logger.Debug().
		Str("ledger_dir", v.LedgerDir).
		Msg("ledger dir set")
	return nil
}

// configureIdentities ensures the identities are valid and sets them
func (v *Validator) configureIdentities(identitiesConfig identities.Config) (err error) {
	v.Identities, err = identities.NewFromConfig(&identitiesConfig)
	if err != nil {
		return err
	}

	v.logger.Debug().
		Str("active_pubkey", v.Identities.Active.PubKey()).
		Str("active_keyfile", v.Identities.Active.KeyFile).
		Str("passive_pubkey", v.Identities.Passive.PubKey()).
		Str("passive_keyfile", v.Identities.Passive.KeyFile).
		Msg("identities set")

	return nil
}

// configureTowerFile ensures the tower file is valid and sets it
func (v *Validator) configureTowerFile(cfg TowerConfig) error {
	v.TowerFileAutoDeleteWhenPassive = cfg.AutoEmptyWhenPassive
	v.logger.Debug().
		Bool("tower_file_auto_delete_when_passive", v.TowerFileAutoDeleteWhenPassive).
		Msg("tower file auto delete when passive set")

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
	v.logger.Debug().
		Str("template", cfg.FileNameTemplate).
		Msg("tower file name template set")

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
	v.logger.Debug().
		Str("tower_file", v.TowerFile).
		Msg("tower file set")

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
	v.logger.Debug().
		Str("template", cfg.SetIdentityActiveCmdTemplate).
		Msg("set identity active command template set")

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
	v.logger.Debug().
		Str("command", v.SetIdentityActiveCommand).
		Msg("set identity active command set")

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
	v.logger.Debug().
		Str("template", cfg.SetIdentityPassiveCmdTemplate).
		Msg("set identity passive command template set")

	// set identity passive command must compile
	if err := setIdentityPassiveCmdTemplate.Execute(&setIdentityPassiveCmdBuf, v); err != nil {
		return fmt.Errorf(
			"failed to execute set identity passive cmd template %s: %w",
			cfg.SetIdentityPassiveCmdTemplate,
			err,
		)
	}
	v.SetIdentityPassiveCommand = setIdentityPassiveCmdBuf.String()
	v.logger.Debug().
		Str("command", v.SetIdentityPassiveCommand).
		Msg("set identity passive command set")

	// if the commands are the same, warn - could be intentional or a mistake
	if v.SetIdentityActiveCommand == v.SetIdentityPassiveCommand {
		log.Warn().
			Msg("set identity active and passive commands are the same - this could be intentional or a mistake")
	}

	return nil
}

// configureHooks ensures the hooks are valid and sets them
func (v *Validator) configureHooks(cfg FailoverConfig) (err error) {
	v.Hooks = cfg.Hooks
	v.logger.Debug().
		Int("pre_when_active", len(v.Hooks.Pre.WhenActive)).
		Int("pre_when_passive", len(v.Hooks.Pre.WhenPassive)).
		Int("post_when_active", len(v.Hooks.Post.WhenActive)).
		Int("post_when_passive", len(v.Hooks.Post.WhenPassive)).
		Msg("hooks set")
	for _, h := range v.Hooks.Pre.WhenActive {
		v.logger.Debug().Str("name", h.Name).Str("command", h.Command).Strs("args", h.Args).Bool("must_succeed", h.MustSucceed).Msg("  pre hook (when active)")
	}
	for _, h := range v.Hooks.Pre.WhenPassive {
		v.logger.Debug().Str("name", h.Name).Str("command", h.Command).Strs("args", h.Args).Bool("must_succeed", h.MustSucceed).Msg("  pre hook (when passive)")
	}
	for _, h := range v.Hooks.Post.WhenActive {
		v.logger.Debug().Str("name", h.Name).Str("command", h.Command).Strs("args", h.Args).Bool("must_succeed", h.MustSucceed).Msg("  post hook (when active)")
	}
	for _, h := range v.Hooks.Post.WhenPassive {
		v.logger.Debug().Str("name", h.Name).Str("command", h.Command).Strs("args", h.Args).Bool("must_succeed", h.MustSucceed).Msg("  post hook (when passive)")
	}
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
		log.Debug().
			Str("name", name).
			Str("address", peer.Address).
			Msg("registered peer")
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
		v.logger.Debug().
			Str("public_ip", v.PublicIP).
			Msg("public ip set in config - not recommended and actually a dirty hack for testing, likely to break and/or be removed in the future")
		return nil
	}

	v.PublicIP, err = v.GetPublicIP()
	if err != nil {
		return err
	}

	v.logger.Debug().
		Str("public_ip", v.PublicIP).
		Msg("public ip set")

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
	v.logger.Debug().
		Str("minimum_time_to_leader_slot", v.MinimumTimeToLeaderSlot.String()).
		Msg("minimum time to leader slot set")
	return nil
}

// GetHostname returns the hostname - can be overridden in tests
func (v *Validator) GetHostname() (string, error) {
	return os.Hostname()
}

// configureHostname ensures the hostname is valid and sets it
func (v *Validator) configureHostname(hostname string) (err error) {
	if hostname != "" {
		v.Hostname = hostname
		v.logger.Debug().
			Str("hostname", v.Hostname).
			Msg("hostname set in config")
		return nil
	}

	hostname, err = v.GetHostname()
	if err != nil {
		return err
	}
	v.Hostname = hostname
	v.logger.Debug().
		Str("hostname", v.Hostname).
		Msg("hostname set")
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

	v.logger.Debug().
		Int("port", v.FailoverServerConfig.Port).
		Int("credit_samples_count", v.MonitorConfig.CreditSamples.Count).
		Str("credit_samples_interval", v.MonitorConfig.CreditSamples.Interval).
		Msg("server and monitor config set")
	return nil
}

// configureGossipNode ensures the gossip node is valid and sets it
func (v *Validator) configureGossipNode() (err error) {
	v.GossipNode, err = v.solanaRPCClient.NodeFromIP(v.PublicIP)
	if err != nil {
		return err
	}
	v.logger.Debug().
		Str("public_ip", v.GossipNode.IP()).
		Str("pubkey", v.GossipNode.PubKey()).
		Msg("gossip node set")
	return nil
}

// makeActive makes this validator active
func (v *Validator) makeActive(params FailoverParams) (err error) {
	log.Debug().Msg("making this validator active")

	if v.IsActive() {
		return fmt.Errorf("this validator is already active - nothing to do")
	}

	log.Info().
		Str("public_ip", v.PublicIP).
		Str("pubkey", v.Identities.Passive.PubKey()).
		Msgf("This validator is currently %s", style.RenderPassiveString(strings.ToUpper(constants.NodeRolePassive), false))

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
		log.Debug().
			Str("tower_file", v.TowerFile).
			Msg("deleting tower file because validator.tower.auto_empty_when_passive is true")

		if err = utils.RemoveFile(v.TowerFile); err != nil {
			return err
		}
	}

	// if the tower file exists and auto empty when passive is false, confirm if you want it deleted and exit if not.
	if !v.TowerFileAutoDeleteWhenPassive && utils.FileExists(v.TowerFile) {
		log.Warn().Msgf("Tower file exists %s", v.TowerFile)
		confirm, err := confirm("Delete tower file and proceed?")
		if err != nil {
			return err
		}
		if !confirm {
			return fmt.Errorf("cancelled")
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
			SolanaValidatorFailoverVersion: pkgconstants.AppVersion,
			RPCAddress:                     v.RPCAddress,
		},
		SolanaRPCClient:  v.solanaRPCClient,
		RPCURL:           v.RPCAddress,
		IsDryRunFailover: !params.NotADrill,
		Hooks:            v.Hooks,
		SkipTowerSync:    params.SkipTowerSync,
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

	log.Info().
		Str("public_ip", v.PublicIP).
		Str("pubkey", v.Identities.Active.PubKey()).
		Msgf("This validator is currently %s", style.RenderActiveString(strings.ToUpper(constants.NodeRoleActive), false))

	log.Debug().Msg("failover active to passive")

	// ensure tower file exists and is not empty
	if !utils.FileExists(v.TowerFile) {
		return fmt.Errorf("tower file does not exist: %s", v.TowerFile)
	}

	if utils.FileSize(v.TowerFile) == 0 {
		return fmt.Errorf("tower file is empty: %s", v.TowerFile)
	}

	// select passive peer to connect to from declared peers
	selectedPassivePeer, err := v.selectPassivePeer()
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
			SetIdentityCommand:             v.SetIdentityPassiveCommand,
			ClientVersion:                  v.GossipNode.Version(),
			SolanaValidatorFailoverVersion: pkgconstants.AppVersion,
			RPCAddress:                     v.RPCAddress,
		},
		Hooks: v.Hooks,
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

			sp.Title(
				style.RenderActiveStringf(
					"validator is healthy and synced - elapsed time %s",
					time.Since(startTime).String(),
				),
			)
			return nil
		}
	})

	return sp.Run()
}

// selectPassivePeer allows selection of a peer from the list of peers
func (v *Validator) selectPassivePeer() (selectedPeer Peer, err error) {
	huhPeerOptions := make([]huh.Option[string], 0)
	for name, peer := range v.Peers {
		selectionKey := style.RenderPassiveString(name, false)
		if zerolog.GlobalLevel() == zerolog.DebugLevel {
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

	log.Debug().Msgf("selected peer: %s address: %s", selectedPeerName, v.Peers[selectedPeerName].Address)

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
