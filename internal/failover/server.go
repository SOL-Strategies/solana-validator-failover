package failover

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/log"
	"github.com/quic-go/quic-go"
	"github.com/sol-strategies/solana-validator-failover/internal/constants"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/solana"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
	pkgconstants "github.com/sol-strategies/solana-validator-failover/pkg/constants"
)

// MonitorConfig holds the configuration for a failover monitor
type MonitorConfig struct {
	CreditSamples CreditSamplesConfig
}

// CreditSamplesConfig holds the configuration for a failover monitor credit samples
type CreditSamplesConfig struct {
	Count            int
	Interval         string
	IntervalDuration time.Duration
}

// ServerConfig is the configuration for the failover server
type ServerConfig struct {
	Port                int
	HeartbeatInterval   string
	StreamTimeout       string
	PassiveNodeInfo     *NodeInfo
	SolanaRPCClient     solana.ClientInterface
	RPCURL              string
	IsDryRunFailover    bool
	Hooks               hooks.FailoverHooks
	MonitorConfig       MonitorConfig
	SkipHistoryTransfer bool
	AutoConfirm         bool
	Rollback            hooks.RollbackConfig
	// TLSConfig is an optional mTLS config. When non-nil, the server requires
	// connecting clients to present a certificate signed by the configured CA.
	// When nil, an ephemeral self-signed certificate is used (no client auth).
	TLSConfig           *tls.Config
	HandoffTimeout      time.Duration
	HandoffPollInterval time.Duration
}

// Server is the failover server - run by the passive node
type Server struct {
	port                int
	listenAddr          string
	tlsConfig           *tls.Config
	transport           *quic.Transport
	listener            *quic.Listener
	heartbeatInterval   time.Duration
	streamTimeout       time.Duration
	ctx                 context.Context
	cancel              context.CancelFunc
	logger              *log.Logger
	passiveNodeInfo     *NodeInfo
	solanaRPCClient     solana.ClientInterface
	rpcURL              string
	failoverStream      *Stream
	isDryRunFailover    bool
	activeConn          *quic.Conn
	hooks               hooks.FailoverHooks
	monitorConfig       MonitorConfig
	skipHistoryTransfer bool
	autoConfirm         bool
	rollback            hooks.RollbackConfig
	mtlsEnabled         bool
	handoffTimeout      time.Duration
	handoffPollInterval time.Duration
}

// NewServerFromConfig creates a new failover server from a configuration
func NewServerFromConfig(config ServerConfig) (*Server, error) {
	var serverTLSConfig *tls.Config
	mtlsEnabled := config.TLSConfig != nil
	if mtlsEnabled {
		cloned := config.TLSConfig.Clone()
		cloned.NextProtos = []string{ProtocolName}
		serverTLSConfig = cloned
	} else {
		tlsCert, err := utils.GenerateTLSCertificate()
		if err != nil {
			return nil, err
		}
		serverTLSConfig = &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{ProtocolName},
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		port:                config.Port,
		tlsConfig:           serverTLSConfig,
		mtlsEnabled:         mtlsEnabled,
		logger:              log.Default(),
		ctx:                 ctx,
		cancel:              cancel,
		passiveNodeInfo:     config.PassiveNodeInfo,
		solanaRPCClient:     config.SolanaRPCClient,
		rpcURL:              config.RPCURL,
		isDryRunFailover:    config.IsDryRunFailover,
		hooks:               config.Hooks,
		monitorConfig:       config.MonitorConfig,
		skipHistoryTransfer: config.SkipHistoryTransfer,
		handoffTimeout:      config.HandoffTimeout,
		handoffPollInterval: config.HandoffPollInterval,
		autoConfirm:         config.AutoConfirm,
		rollback:            config.Rollback,
	}

	if s.port == 0 {
		s.port = DefaultPort
	}
	s.listenAddr = fmt.Sprintf(":%d", s.port)

	if config.HeartbeatInterval == "" {
		config.HeartbeatInterval = DefaultHeartbeatIntervalDurationStr
	}

	if config.StreamTimeout == "" {
		config.StreamTimeout = DefaultStreamTimeoutDurationStr
	}

	var err error
	s.heartbeatInterval, err = time.ParseDuration(config.HeartbeatInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to parse heartbeat interval: %v", err)
	}

	s.streamTimeout, err = time.ParseDuration(config.StreamTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stream timeout: %v", err)
	}

	return s, nil
}

// Start starts the failover server
func (s *Server) Start() error {
	wrapped, err := newBasicPacketConn(fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %v", err)
	}
	s.transport = &quic.Transport{Conn: wrapped}

	listener, err := s.transport.Listen(
		s.tlsConfig,
		&quic.Config{
			KeepAlivePeriod: s.heartbeatInterval,
			MaxIdleTimeout:  s.streamTimeout,
		},
	)
	if err != nil {
		wrapped.Close()
		return fmt.Errorf("failed to create listener: %v", err)
	}
	s.listener = listener

	s.logger.Info(style.RenderPinkString(fmt.Sprintf("listening on port %d - run this program on the ", s.port)) +
		style.RenderActiveString("active", false) +
		style.RenderPinkString(" validator to continue"))

	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			conn, err := s.listener.Accept(context.Background())
			if err != nil {
				if err.Error() == "quic: server closed" {
					return nil
				}
				s.logger.Error("failed to accept connection", "err", err)
				continue
			}

			go s.handleConnection(conn)
		}
	}
}

// handleConnection handles a new failover connection
func (s *Server) handleConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "connection closed")

	s.logger.Debug("accepted new connection", "remote_addr", conn.RemoteAddr().String())

	if s.mtlsEnabled {
		tlsState := conn.ConnectionState().TLS
		if len(tlsState.PeerCertificates) > 0 {
			peer := tlsState.PeerCertificates[0]
			s.logger.Info("mTLS: client certificate verified",
				"remote_addr", conn.RemoteAddr().String(),
				"subject", peer.Subject.String(),
				"issuer", peer.Issuer.String(),
				"expires", peer.NotAfter,
			)
		}
	}

	s.activeConn = conn

	// Accept streams
	for {
		stream, err := conn.AcceptStream(s.ctx)
		if err != nil {
			s.logger.Debug("failed to accept stream", "remote_addr", conn.RemoteAddr().String(), "err", err)
			return
		}

		s.logger.Debug("accepted new stream", "remote_addr", conn.RemoteAddr().String())
		go s.handleStream(stream)
	}
}

// handleStream handles a new failover stream
func (s *Server) handleStream(stream *quic.Stream) {
	defer stream.Close()

	// Read the message type
	msgType := make([]byte, 1)
	if _, err := io.ReadFull(stream, msgType); err != nil {
		if err == io.EOF {
			s.logger.Debug("stream closed by peer")
			return
		}
		s.logger.Debugf("failed to read message type: %v", err)
		return
	}

	// Check wire protocol version before any gob work.
	// This gives a clear error when nodes are running incompatible versions,
	// rather than the cryptic gob type-mismatch that would otherwise surface.
	if err := readAndCheckWireVersion(stream); err != nil {
		s.logger.Error("rejecting failover stream with incompatible wire protocol", "err", err)
		return
	}

	switch msgType[0] {
	case MessageTypeFailoverInitiateRequest: // failover
		s.logger.Debug("received failover initiate request")
		s.handleFailoverStream(stream)
	default:
		s.logger.Errorf("unknown message type: %d - ignoring stream", msgType[0])
	}
}

func (s *Server) handleFailoverStream(stream *quic.Stream) {
	var dryRunHistoryPath string
	defer func() {
		if dryRunHistoryPath == "" {
			return
		}
		if err := os.Remove(dryRunHistoryPath); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to remove temporary dry-run vote history", "path", dryRunHistoryPath, "err", err)
		}
	}()

	// read the message and parse it into a Stream struct
	s.failoverStream = NewFailoverStream(stream)
	if s.failoverStream.Decode() != nil {
		return
	}

	// Write our wire protocol version in the server→client direction before
	// any gob encode so the client can verify compatibility symmetrically.
	if err := writeWireVersion(stream); err != nil {
		s.logger.Error("failed to write wire version to client", "err", err)
		return
	}

	// set the is dry run failover flag
	s.failoverStream.SetIsDryRunFailover(s.isDryRunFailover)

	// set the skip history transfer flag
	s.failoverStream.SetSkipHistoryTransfer(s.skipHistoryTransfer)

	// set this node's info so subsequent responses can be sent to the client with it
	s.failoverStream.SetPassiveNodeInfo(s.passiveNodeInfo)

	activeInfo := s.failoverStream.GetActiveNodeInfo()
	passiveInfo := s.failoverStream.GetPassiveNodeInfo()
	strategy := HandoffStrategyVoteHistory
	s.failoverStream.SetHandoffStrategy(strategy)
	if activeInfo.Identities.Active.PubKey() != passiveInfo.Identities.Active.PubKey() {
		s.failoverStream.SetErrorMessage("source and destination active identities differ")
		_ = s.failoverStream.Encode()
		return
	}
	for _, info := range []*NodeInfo{activeInfo, passiveInfo} {
		if info.ClientFamily != "agave" && info.ClientFamily != "jito-solana" && info.ClientFamily != "firedancer" {
			s.failoverStream.SetErrorMessage("unsupported validator client family")
			_ = s.failoverStream.Encode()
			return
		}
		if info.IsNativeFiredancer != (info.ClientFamily == "firedancer") {
			s.failoverStream.SetErrorMessage("inconsistent native Firedancer metadata")
			_ = s.failoverStream.Encode()
			return
		}
		if err := ValidateNativeVersion(info); err != nil {
			s.failoverStream.SetErrorMessage(err.Error())
			_ = s.failoverStream.Encode()
			return
		}
	}
	// Commands remain operator supplied. Render them only after both sides are
	// known, so templates can select flags for the direction and pairing.
	transferHistory := s.failoverStream.GetVoteHistoryWillBeTransferred()
	activeCommand, err := RenderIdentityCommand(activeInfo.SetIdentityCommandTemplate, NewCommandTemplateData(*activeInfo, *passiveInfo, *activeInfo, *passiveInfo, strategy, transferHistory, s.isDryRunFailover))
	if err != nil {
		s.failoverStream.SetErrorMessagef("failed to render active identity command: %v", err)
		_ = s.failoverStream.Encode()
		return
	}
	passiveCommand, err := RenderIdentityCommand(passiveInfo.SetIdentityCommandTemplate, NewCommandTemplateData(*passiveInfo, *activeInfo, *activeInfo, *passiveInfo, strategy, transferHistory, s.isDryRunFailover))
	if err != nil {
		s.failoverStream.SetErrorMessagef("failed to render passive identity command: %v", err)
		_ = s.failoverStream.Encode()
		return
	}
	if activeCommand != "" {
		activeInfo.SetIdentityCommand = activeCommand
	}
	for _, command := range []string{activeCommand, passiveCommand} {
		if _, err := utils.CommandArgs(command); err != nil {
			s.failoverStream.SetErrorMessagef("invalid identity command: %v", err)
			_ = s.failoverStream.Encode()
			return
		}
	}
	if transferHistory && passiveInfo.IsNativeFiredancer {
		if err := ValidateHistoryCommand(passiveCommand, passiveInfo.VoteHistoryImportFile); err != nil {
			s.failoverStream.SetErrorMessage(err.Error())
			_ = s.failoverStream.Encode()
			return
		}
	}
	if passiveCommand != "" {
		passiveInfo.SetIdentityCommand = passiveCommand
	}
	activeRollback, passiveRollback, err := renderNegotiatedRollbackCommands(*activeInfo, *passiveInfo, s.rollback.ToPassive.ResolvedCmd, strategy, transferHistory, s.isDryRunFailover)
	if err != nil {
		s.failoverStream.SetErrorMessagef("failed to render rollback command: %v", err)
		_ = s.failoverStream.Encode()
		return
	}
	s.failoverStream.SetRollbackCommands(activeRollback, passiveRollback)
	// The plan is rendered on the passive node, so replace its local source
	// rollback value with the exact command negotiated for the active client.
	// An empty value is intentional: in that case the active client retains its
	// own local fallback rather than receiving a server-side command.
	s.rollback.ToActive.ResolvedCmd = activeRollback
	s.rollback.ToPassive.ResolvedCmd = passiveRollback

	// ensure client and this server are using the same version of solana-validator-failover
	clientVersion := s.failoverStream.GetActiveNodeInfo().SolanaValidatorFailoverVersion
	serverVersion := pkgconstants.AppVersion

	s.logger.Debug("checking for client and server version mismatch",
		"server_version", serverVersion,
		"client_version", clientVersion,
	)

	if clientVersion != serverVersion {
		s.failoverStream.LogErrorWithSetMessagef("Server (%s) and client (%s) version mismatch", serverVersion, clientVersion)
		if err := s.failoverStream.Encode(); err != nil {
			s.logger.Error("failed to send error message to client", "err", err)
		}
		s.logger.Fatal("server and client running different versions of this program - aborting")
		return
	}
	activeMode := s.failoverStream.GetActiveNodeInfo().Consensus
	passiveMode := s.passiveNodeInfo.Consensus
	if activeMode != ConsensusAlpenglow {
		s.failoverStream.SetErrorMessagef("active node reported unsupported consensus mode %q", activeMode)
		_ = s.failoverStream.Encode()
		return
	}
	if passiveInfo.IsNativeFiredancer {
		version, versionErr := s.solanaRPCClient.GetLocalNodeVersion()
		if versionErr != nil {
			s.failoverStream.SetErrorMessagef("cannot read native Firedancer runtime version: %v", versionErr)
			_ = s.failoverStream.Encode()
			return
		}
		passiveInfo.ClientVersionRPC = version
		if err := ValidateNativeVersion(passiveInfo); err != nil {
			s.failoverStream.SetErrorMessage(err.Error())
			_ = s.failoverStream.Encode()
			return
		}
	}
	passiveState, err := DetectConsensus(s.rpcURL)
	if err != nil {
		s.failoverStream.SetErrorMessagef("cannot detect passive node consensus: %v", err)
		_ = s.failoverStream.Encode()
		return
	}
	if passiveState.Mode == "migrating" {
		s.failoverStream.SetErrorMessage("passive node is migrating to Alpenglow; failover is disabled during migration")
		_ = s.failoverStream.Encode()
		return
	}
	if passiveState.Mode != passiveMode {
		s.failoverStream.SetErrorMessagef("passive consensus declaration %q disagrees with local RPC consensus %q", passiveMode, passiveState.Mode)
		_ = s.failoverStream.Encode()
		return
	}
	if activeMode != passiveState.Mode || (activeMode == ConsensusAlpenglow && s.failoverStream.GetActiveNodeInfo().ConsensusGenesisSlot != passiveState.GenesisSlot) {
		s.failoverStream.SetErrorMessagef("consensus mismatch: active=%s genesis=%d passive=%s genesis=%d", activeMode, s.failoverStream.GetActiveNodeInfo().ConsensusGenesisSlot, passiveState.Mode, passiveState.GenesisSlot)
		_ = s.failoverStream.Encode()
		return
	}
	passiveInfo.Consensus = passiveState.Mode
	passiveInfo.ConsensusGenesisSlot = passiveState.GenesisSlot
	// Keep the negotiated command and fresh runtime version when updating
	// local metadata; startup commands may include a different transfer choice.
	*s.passiveNodeInfo = *passiveInfo

	// Query gossip for the client by both its public IP and configured active identity.
	activeNodeInfo := s.failoverStream.GetActiveNodeInfo()
	expectedActivePubkey := activeNodeInfo.Identities.Active.PubKey()
	s.logger.Debug("querying gossip for active node",
		"public_ip", activeNodeInfo.PublicIP,
		"pubkey", expectedActivePubkey,
	)
	gossipActiveNode, err := s.solanaRPCClient.NodeFromIPWithExpectedPubkey(activeNodeInfo.PublicIP, expectedActivePubkey)
	if err != nil {
		s.failoverStream.LogErrorWithSetMessagef("Failed to validate active node: %v", err)
		if s.failoverStream.Encode() != nil {
			return
		}
		return
	}

	// Ensure the failover request comes from the configured active node.
	if err := validateActiveGossipIdentity(gossipActiveNode.IP(), gossipActiveNode.PubKey(), activeNodeInfo.PublicIP, expectedActivePubkey); err != nil {
		s.failoverStream.LogErrorWithSetMessagef("Failed to validate active node: %v", err)
		if s.failoverStream.Encode() != nil {
			return
		}
		return
	}

	// confirm the failover with the user
	// Get RPC URLs from the messages passed between client and server
	activeRPCURL := s.failoverStream.GetActiveNodeInfo().RPCAddress
	passiveRPCURL := s.failoverStream.GetPassiveNodeInfo().RPCAddress

	// Abort if rollback is enabled on one side but not the other.
	// Partial rollback is worse than no rollback — one node reverts while the other stays passive.
	clientRollback := s.failoverStream.GetActiveRollbackEnabled()
	serverRollback := s.rollback.Enabled
	if serverRollback != clientRollback {
		var msg string
		if serverRollback {
			msg = "rollback mismatch: this node has rollback.enabled=true but the active node does not"
		} else {
			msg = "rollback mismatch: the active node has rollback.enabled=true but this node does not"
		}
		msg += " — both nodes must have rollback identically configured; fix the config and retry"
		s.failoverStream.SetErrorMessage(msg)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error("failed to send error message to client", "err", encodeErr)
		}
		s.logger.Fatal(msg)
		return
	}

	s.logger.Infof("%s connected from %s - failover plan:", s.failoverStream.GetActiveNodeInfo().Hostname, s.activeConn.RemoteAddr())

	if err := s.failoverStream.ConfirmFailover(s.hooks, s.rollback, activeRPCURL, passiveRPCURL, s.autoConfirm); err != nil {
		s.logger.Error("failover cancelled", "err", err)

		// Send error message to client before exiting
		s.failoverStream.SetErrorMessagef("server cancelled failover: %v", err)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error("failed to send error message to client", "err", encodeErr)
		}

		// close the server listener and cancel the context to stop accepting new connections
		if s.listener != nil {
			if err := s.listener.Close(); err != nil {
				s.logger.Error("failed to close listener", "err", err)
			}
		}
		if s.transport != nil {
			if err := s.transport.Close(); err != nil {
				s.logger.Error("failed to close transport", "err", err)
			}
		}
		s.cancel()
		os.Exit(1)
	}

	// take initial sample of vote credits and rank for the active key - use it to compare later
	s.logger.Debug("pulling pre-failover vote credits sample...")
	err = s.failoverStream.PullActiveIdentityVoteCreditsSample(s.solanaRPCClient)
	if err != nil {
		s.logger.Warn("failed to pull pre-failover vote credits sample", "err", err)
	}
	if transferHistory {
		if passiveInfo.IsNativeFiredancer && activeNodeInfo.VoteHistoryFileSizeBytes > NativeHistoryMaxSize {
			s.failoverStream.SetErrorMessage("source vote history exceeds Firedancer's 32688-byte limit")
			_ = s.failoverStream.Encode()
			return
		}
		path := passiveInfo.VoteHistoryFile
		if passiveInfo.IsNativeFiredancer {
			path = passiveInfo.VoteHistoryImportFile
		}
		if passiveInfo.IsNativeFiredancer {
			if err := validateHistoryDirectories(path, passiveInfo.VoteHistoryFile); err != nil {
				s.failoverStream.SetErrorMessagef("invalid Firedancer history directories: %v", err)
				_ = s.failoverStream.Encode()
				return
			}
		}
		if err := prepareHistoryDestination(path, passiveInfo.IsNativeFiredancer, s.isDryRunFailover, s.logger); err != nil {
			s.failoverStream.SetErrorMessagef("cannot prepare vote history destination: %v", err)
			_ = s.failoverStream.Encode()
			return
		}
		if err := preflightStateDestination(path, 1); err != nil {
			s.failoverStream.SetErrorMessagef("cannot receive vote history: %v", err)
			_ = s.failoverStream.Encode()
			return
		}
		if s.isDryRunFailover {
			dryRunHistoryPath, err = createDryRunHistoryDestination(path)
			if err != nil {
				s.failoverStream.SetErrorMessagef("cannot prepare temporary dry-run vote history destination: %v", err)
				_ = s.failoverStream.Encode()
				return
			}
		}
	}

	// The local and peer consensus states were verified during the handshake.
	// Continue with that result without querying RPC again before identity changes.
	s.failoverStream.SetCanProceed(true)
	if s.failoverStream.Encode() != nil {
		return
	}

	if err := s.failoverStream.Decode(); err != nil {
		s.logger.Error("failed to receive source demotion acknowledgement", "err", err)
		return
	}
	// Incoming frames carry timing metadata, not authority to change the
	// destination command, history paths, or negotiated transfer choice.
	s.failoverStream.SetPassiveNodeInfo(s.passiveNodeInfo)
	s.failoverStream.SetIsDryRunFailover(s.isDryRunFailover)
	s.failoverStream.SetSkipHistoryTransfer(s.skipHistoryTransfer)
	s.failoverStream.SetHandoffStrategy(HandoffStrategyVoteHistory)
	if s.failoverStream.GetHandoffAborted() {
		s.failoverStream.SetRollbackRequired(true)
		_ = s.failoverStream.Encode()
		return
	}
	if transferHistory {
		if s.isDryRunFailover {
			s.logger.Infof("dry run: receiving current vote-history snapshot from %s into a temporary file", activeNodeInfo.Hostname)
		} else {
			s.logger.Infof("waiting for vote history from %s", activeNodeInfo.Hostname)
		}
		path := passiveInfo.VoteHistoryFile
		if passiveInfo.IsNativeFiredancer {
			path = passiveInfo.VoteHistoryImportFile
		}
		if s.isDryRunFailover {
			path = dryRunHistoryPath
		}
		limit := int64(0)
		if passiveInfo.IsNativeFiredancer {
			limit = NativeHistoryMaxSize
		}
		if err := s.failoverStream.receiveStateFileLimited(path, limit); err != nil {
			s.failoverStream.SetErrorMessagef("vote-history transfer failed before activation: %v", err)
			s.failoverStream.SetRollbackRequired(true)
			_ = s.failoverStream.Encode()
			return
		}
		if err := s.failoverStream.Decode(); err != nil {
			s.logger.Error("failed to receive transfer metadata", "err", err)
			return
		}
		s.failoverStream.SetPassiveNodeInfo(s.passiveNodeInfo)
		s.failoverStream.SetIsDryRunFailover(s.isDryRunFailover)
		s.failoverStream.SetSkipHistoryTransfer(s.skipHistoryTransfer)
		s.failoverStream.SetHandoffStrategy(HandoffStrategyVoteHistory)
		s.failoverStream.SetPassiveNodeSyncVoteHistoryEndTime()
	} else {
		s.logger.Info("history transfer skipped")
	}
	if s.failoverStream.GetHandoffAborted() {
		s.failoverStream.SetRollbackRequired(true)
		_ = s.failoverStream.Encode()
		return
	}

	// run pre hooks when passive
	err = s.hooks.RunPreWhenPassive(s.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: s.isDryRunFailover,
		isPreFailover:    true,
	}))
	if err != nil {
		s.failoverStream.SetRollbackRequired(true)
		s.failoverStream.SetErrorMessagef("server failed to run its pre-failover hooks: %v", err)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error("failed to send error message to client", "err", encodeErr)
		}
		s.logger.Error("failed to run pre hooks when passive", "err", err)
		return
	}

	// set identity to active
	dryRunPrefix := ""
	if s.isDryRunFailover {
		dryRunPrefix = style.RenderLightGreyString("(dry run)") + " "
	}
	s.logger.Info(dryRunPrefix +
		style.RenderPinkString("changing to ") +
		style.RenderActiveString(constants.NodeRoleActive, false) +
		style.RenderPinkString(" identity"))

	s.failoverStream.SetPassiveNodeSetIdentityStartTime()

	err = utils.RunIdentityCommand(s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand, s.isDryRunFailover, s.logger.GetLevel() <= log.DebugLevel)
	if err != nil {
		s.logger.Error(fmt.Sprintf("failed to set identity to active with command: %s", s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand), "err", err)
		// Activation may have signed votes even if the command failed. Reusing
		// the source history after that attempt cannot prove a safe rollback.
		if s.rollback.Enabled {
			if rollbackErr := RunRollbackToPassive(s.rollback, s.getHookEnvMap(hookEnvMapParams{isDryRunFailover: s.isDryRunFailover, isPostFailover: true}), s.isDryRunFailover, s.logger); rollbackErr != nil {
				s.logger.Error("destination demotion failed", "err", rollbackErr)
			}
		}
		s.failoverStream.SetErrorMessagef("destination activation failed; source reactivation requires manual recovery: %v", err)
		_ = s.failoverStream.Encode()
		return
	}

	if !s.isDryRunFailover {
		identityCtx, identityCancel := context.WithTimeout(s.ctx, handoffTimeout(s.handoffTimeout))
		identityErr := waitForLocalIdentity(identityCtx, s.solanaRPCClient, s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(), s.handoffPollInterval)
		identityCancel()
		if identityErr != nil {
			s.failoverStream.SetErrorMessagef("destination identity command completed without activating the expected identity: %v", identityErr)
			_ = s.failoverStream.Encode()
			s.logger.Error("handoff quarantined after destination activation attempt", "err", identityErr)
			return
		}
	}

	s.failoverStream.SetPassiveNodeSetIdentityEndTime()

	// get the current slot and record it - sometimes rpc will be a slot behind, if so, assume same-slot
	failoverEndSlot, err := s.solanaRPCClient.GetCurrentSlot()
	if err != nil {
		s.logger.Warn("failed to get current slot", "err", err)
		err = nil
	} else if failoverEndSlot < s.failoverStream.GetFailoverStartSlot() {
		s.failoverStream.SetFailoverEndSlot(s.failoverStream.GetFailoverStartSlot())
	} else {
		s.failoverStream.SetFailoverEndSlot(failoverEndSlot)
	}

	// set is successfully completed to true
	s.failoverStream.SetIsSuccessfullyCompleted(true)
	if s.failoverStream.Encode() != nil {
		return
	}

	// run post hooks when active
	s.hooks.RunPostWhenActive(s.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: s.isDryRunFailover,
		isPostFailover:   true,
	}))

	if !s.isDryRunFailover {
		s.confirmGossipNodesPostFailover()
	}

	// build and render the post-failover summary immediately (before credit monitoring)
	summaryData := s.failoverStream.BuildSummaryData()
	rendered, renderErr := RenderFailoverSummary(summaryData)
	if renderErr != nil {
		s.logger.Error("failed to render failover summary", "err", renderErr)
	} else {
		s.logger.Info("post-failover state:")
		fmt.Println(style.RenderMessageString(strings.TrimLeft(rendered, "\n")))
	}

	// monitor the credits by pulling configured samples
	s.logger.Info("monitoring vote credits post-failover...")
	err = s.failoverStream.PullActiveIdentityVoteCreditsSamples(s.solanaRPCClient, s.monitorConfig.CreditSamples.Count, s.monitorConfig.CreditSamples.IntervalDuration)
	if err != nil {
		s.logger.Error("failed to pull active identity vote credits samples", "err", err)
	}

	rankDifference, firstRank, lastRank, rankErr := s.failoverStream.GetVoteCreditRankDifference()
	if rankErr == nil {
		var rankMsg string
		switch {
		case rankDifference > 0:
			rankMsg = style.RenderActiveString(fmt.Sprintf("improved by +%d", rankDifference), false)
		case rankDifference < 0:
			rankMsg = style.RenderPassiveString(fmt.Sprintf("worsened by %d", rankDifference), false)
		default:
			rankMsg = style.RenderLightGreyString("unchanged")
		}
		s.logger.Infof("vote credits: rank %s (%d → %d)", rankMsg, firstRank, lastRank)
	}

	// close the stream and connection cleanly
	if err := stream.Close(); err != nil {
		s.logger.Error("failed to close stream", "err", err)
	}
	if err := s.activeConn.CloseWithError(quic.ApplicationErrorCode(0), "failover complete"); err != nil {
		s.logger.Debugf("closing connection after successful failover: %v", err)
	}

	// close the server listener and cancel the context to stop accepting new connections
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.logger.Error("failed to close listener", "err", err)
		}
	}
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			s.logger.Error("failed to close transport", "err", err)
		}
	}
	s.cancel()
}

func renderNegotiatedRollbackCommands(activeInfo, passiveInfo NodeInfo, passiveFallback, strategy string, transferHistory, dryRun bool) (string, string, error) {
	activeRollback := ""
	passiveRollback := passiveFallback
	// Rollback to active uses the original source history, while destination
	// activation imports the history received over the failover stream.
	activeData := NewCommandTemplateData(activeInfo, passiveInfo, activeInfo, passiveInfo, strategy, transferHistory, dryRun)
	activeData.VoteHistoryImportFile = activeInfo.VoteHistoryFile
	passiveData := NewCommandTemplateData(passiveInfo, activeInfo, activeInfo, passiveInfo, strategy, transferHistory, dryRun)
	if activeInfo.RollbackToActiveCommandTemplate != "" {
		var err error
		activeRollback, err = RenderIdentityCommand(activeInfo.RollbackToActiveCommandTemplate, activeData)
		if err != nil {
			return "", "", err
		}
	} else if activeInfo.SetIdentityActiveCommandTemplate != "" {
		var err error
		activeRollback, err = RenderIdentityCommand(activeInfo.SetIdentityActiveCommandTemplate, activeData)
		if err != nil {
			return "", "", err
		}
	}
	if passiveInfo.RollbackToPassiveCommandTemplate != "" {
		var err error
		passiveRollback, err = RenderIdentityCommand(passiveInfo.RollbackToPassiveCommandTemplate, passiveData)
		if err != nil {
			return "", "", err
		}
	} else if passiveInfo.SetIdentityPassiveCommandTemplate != "" {
		var err error
		passiveRollback, err = RenderIdentityCommand(passiveInfo.SetIdentityPassiveCommandTemplate, passiveData)
		if err != nil {
			return "", "", err
		}
	}
	if transferHistory && activeInfo.IsNativeFiredancer {
		if err := ValidateHistoryCommand(activeRollback, activeInfo.VoteHistoryFile); err != nil {
			return "", "", fmt.Errorf("source rollback: %w", err)
		}
	}
	return activeRollback, passiveRollback, nil
}

func validateActiveGossipIdentity(actualIP, actualPubkey, expectedIP, expectedPubkey string) error {
	if actualIP != expectedIP {
		return fmt.Errorf("active node IP %s does not match expected IP %s", actualIP, expectedIP)
	}
	if actualPubkey != expectedPubkey {
		return fmt.Errorf(
			"active node pubkey %s at IP %s does not match expected pubkey %s",
			actualPubkey,
			actualIP,
			expectedPubkey,
		)
	}
	return nil
}

// confirmGossipNodesPostFailover confirms that the gossip nodes have switched roles post-failover
func (s *Server) confirmGossipNodesPostFailover() {
	var (
		solanaActiveNode                        *solana.Node
		solanaPassiveNode                       *solana.Node
		err                                     error
		isActiveNodeKeySwitchReflectedInGossip  bool
		isPassiveNodeKeySwitchReflectedInGossip bool
	)

	sp := spinner.New().Title(style.RenderPinkString("confirming gossip nodes switched roles..."))
	sp.ActionWithErr(func(ctx context.Context) error {
		maxRetries := 5
		retryCount := 0
		// it can take a few seconds for gossip to update so try to refresh gossip identities a few times before claiming error
		for retryCount < maxRetries {
			retryDelay := time.Duration(1<<(retryCount+1)) * time.Second
			retryCount++
			hasRetriesLeft := retryCount < maxRetries

			// active node is now the old passive node — prefer the expected pubkey to handle
			// dual CRDS entries that briefly coexist during a gossip identity transition
			solanaActiveNode, err = s.solanaRPCClient.NodeFromIPWithExpectedPubkey(
				s.failoverStream.GetPassiveNodeInfo().PublicIP,
				s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(),
			)
			if err != nil && hasRetriesLeft {
				sp.Title(style.RenderWarningStringf("(attempt %d of %d) failed to refresh active node info from gossip - retrying", retryCount, maxRetries))
				time.Sleep(retryDelay)
				continue
			}
			if err != nil && !hasRetriesLeft {
				sp.Title(style.RenderErrorStringf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries))
				s.logger.Error(fmt.Sprintf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries), "err", err)
				return fmt.Errorf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries)
			}

			// passive node is now the old active node — prefer the expected pubkey for the same reason
			solanaPassiveNode, err = s.solanaRPCClient.NodeFromIPWithExpectedPubkey(
				s.failoverStream.GetActiveNodeInfo().PublicIP,
				s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey(),
			)
			if err != nil && hasRetriesLeft {
				sp.Title(style.RenderWarningStringf("(attempt %d of %d) failed to refresh fetch passive node info - retrying", retryCount, maxRetries))
				time.Sleep(retryDelay)
				continue
			}
			if err != nil && !hasRetriesLeft {
				sp.Title(style.RenderErrorStringf("(attempt %d of %d) failed to refresh fetch passive node info - giving up", retryCount, maxRetries))
				return fmt.Errorf("(attempt %d of %d) failed to refresh fetch passive node info - giving up", retryCount, maxRetries)
			}

			// check the gossip pubkeys switched
			isActiveNodeKeySwitchReflectedInGossip = solanaActiveNode.PubKey() == s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey()
			isPassiveNodeKeySwitchReflectedInGossip = solanaPassiveNode.PubKey() == s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey()

			// if the active node key is not reflected in gossip, query gossip again
			if !isActiveNodeKeySwitchReflectedInGossip && hasRetriesLeft {
				sp.Title(style.RenderWarningStringf("(attempt %d of %d) gossip active node %s pubkey does not match expected pubkey: %s != %s - retrying in %s",
					retryCount,
					maxRetries,
					solanaActiveNode.IP(),
					solanaActiveNode.PubKey(),
					s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(),
					retryDelay,
				))
				time.Sleep(retryDelay)
				continue
			}

			// if the active node key is not reflected in gossip after retries show error and exit
			if !isActiveNodeKeySwitchReflectedInGossip && !hasRetriesLeft {
				sp.Title(style.RenderErrorStringf("gossip active node %s pubkey does not match expected pubkey: %s != %s - after %d retries",
					solanaActiveNode.IP(),
					solanaActiveNode.PubKey(),
					s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(),
					retryCount,
				))
				return fmt.Errorf("gossip active node %s pubkey does not match expected pubkey: %s != %s - after %d retries",
					solanaActiveNode.IP(),
					solanaActiveNode.PubKey(),
					s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(),
					retryCount,
				)
			}

			// if the passive node key is not reflected in gossip, query gossip again
			if !isPassiveNodeKeySwitchReflectedInGossip && hasRetriesLeft {
				sp.Title(style.RenderWarningStringf("(attempt %d of %d) gossip passive node %s pubkey does not match expected pubkey: %s != %s - retrying in %s",
					retryCount,
					maxRetries,
					solanaPassiveNode.IP(),
					solanaPassiveNode.PubKey(),
					s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey(),
					retryDelay,
				))
				time.Sleep(retryDelay)
				continue
			}

			// if the passive node key is not reflected in gossip after retries show error
			if !isPassiveNodeKeySwitchReflectedInGossip && !hasRetriesLeft {
				sp.Title(style.RenderErrorStringf("gossip passive node %s pubkey does not match expected pubkey: %s != %s - after %d retries",
					solanaPassiveNode.IP(),
					solanaPassiveNode.PubKey(),
					s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey(),
					retryCount,
				))
				return fmt.Errorf("gossip passive node %s pubkey does not match expected pubkey: %s != %s - after %d retries",
					solanaPassiveNode.IP(),
					solanaPassiveNode.PubKey(),
					s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey(),
					retryCount,
				)
			}
		}

		return nil
	})

	err = sp.Run()
	if err != nil {
		s.logger.Error("failed to confirm gossip nodes switched roles - potentially serious shit - investigate immediately", "err", err)
	}

	if isActiveNodeKeySwitchReflectedInGossip && isPassiveNodeKeySwitchReflectedInGossip {
		s.logger.Info("gossip confirms nodes switched roles successfully")
	} else {
		s.logger.Error("gossip does not confirm role switch")
	}
}

// getEnvMap returns a map of environment variables to pass to the hooks
func (s *Server) getHookEnvMap(params hookEnvMapParams) (envMap map[string]string) {
	envMap = map[string]string{}

	envMap["IS_DRY_RUN_FAILOVER"] = fmt.Sprintf("%t", params.isDryRunFailover)

	// this node is passive
	if params.isPreFailover {
		envMap["THIS_NODE_ROLE"] = constants.NodeRolePassive
		envMap["PEER_NODE_ROLE"] = constants.NodeRoleActive
	}

	// only show switch to active
	if params.isPostFailover {
		envMap["THIS_NODE_ROLE"] = constants.NodeRoleActive
		envMap["PEER_NODE_ROLE"] = constants.NodeRolePassive
	}

	// this node is passive
	envMap["THIS_NODE_NAME"] = s.passiveNodeInfo.Hostname
	envMap["THIS_NODE_PUBLIC_IP"] = s.passiveNodeInfo.PublicIP
	envMap["THIS_NODE_ACTIVE_IDENTITY_PUBKEY"] = s.passiveNodeInfo.Identities.Active.PubKey()
	envMap["THIS_NODE_ACTIVE_IDENTITY_KEYPAIR_FILE"] = s.passiveNodeInfo.Identities.Active.KeyFile
	envMap["THIS_NODE_PASSIVE_IDENTITY_PUBKEY"] = s.passiveNodeInfo.Identities.Passive.PubKey()
	envMap["THIS_NODE_PASSIVE_IDENTITY_KEYPAIR_FILE"] = s.passiveNodeInfo.Identities.Passive.KeyFile
	envMap["THIS_NODE_CLIENT_VERSION"] = s.passiveNodeInfo.ClientVersion
	envMap["THIS_NODE_CLIENT_VERSION_LOCAL_RPC"] = s.passiveNodeInfo.ClientVersionRPC
	envMap["THIS_NODE_RPC_ADDRESS"] = s.rpcURL

	// peer node is active
	envMap["PEER_NODE_NAME"] = s.failoverStream.GetActiveNodeInfo().Hostname
	envMap["PEER_NODE_PUBLIC_IP"] = s.failoverStream.GetActiveNodeInfo().PublicIP
	envMap["PEER_NODE_ACTIVE_IDENTITY_PUBKEY"] = s.failoverStream.GetActiveNodeInfo().Identities.Active.PubKey()
	envMap["PEER_NODE_PASSIVE_IDENTITY_PUBKEY"] = s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey()
	envMap["PEER_NODE_CLIENT_VERSION"] = s.failoverStream.GetActiveNodeInfo().ClientVersion
	envMap["PEER_NODE_CLIENT_VERSION_LOCAL_RPC"] = s.failoverStream.GetActiveNodeInfo().ClientVersionRPC
	AddHandoffTemplateEnv(envMap, *s.failoverStream.GetActiveNodeInfo(), *s.passiveNodeInfo, *s.passiveNodeInfo, *s.failoverStream.GetActiveNodeInfo(), s.failoverStream.GetHandoffStrategy(), s.failoverStream.GetVoteHistoryWillBeTransferred())

	return
}
