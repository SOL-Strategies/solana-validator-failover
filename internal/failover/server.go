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
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	Port              int
	HeartbeatInterval string
	StreamTimeout     string
	PassiveNodeInfo   *NodeInfo
	SolanaRPCClient   solana.ClientInterface
	RPCURL            string
	IsDryRunFailover  bool
	Hooks             hooks.FailoverHooks
	MonitorConfig     MonitorConfig
	SkipTowerSync     bool
	AutoConfirm       bool
}

// Server is the failover server - run by the passive node
type Server struct {
	port              int
	listenAddr        string
	tlsConfig         *tls.Config
	transport         *quic.Transport
	listener          *quic.Listener
	heartbeatInterval time.Duration
	streamTimeout     time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	logger            zerolog.Logger
	passiveNodeInfo   *NodeInfo
	solanaRPCClient   solana.ClientInterface
	rpcURL            string
	failoverStream    *Stream
	isDryRunFailover  bool
	activeConn        *quic.Conn
	hooks             hooks.FailoverHooks
	monitorConfig     MonitorConfig
	skipTowerSync     bool
	autoConfirm       bool
}

// NewServerFromConfig creates a new failover server from a configuration
func NewServerFromConfig(config ServerConfig) (*Server, error) {
	// TODO: accept and parse local cert if supplied
	tlsCert, err := utils.GenerateTLSCertificate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		port: config.Port,
		tlsConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos: []string{
				ProtocolName,
			},
		},
		logger:           log.With().Logger(),
		ctx:              ctx,
		cancel:           cancel,
		passiveNodeInfo:  config.PassiveNodeInfo,
		solanaRPCClient:  config.SolanaRPCClient,
		rpcURL:           config.RPCURL,
		isDryRunFailover: config.IsDryRunFailover,
		hooks:            config.Hooks,
		monitorConfig:    config.MonitorConfig,
		skipTowerSync:    config.SkipTowerSync,
		autoConfirm:      config.AutoConfirm,
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

	s.logger.Info().Msgf("Listening on port %d - run this program on the ACTIVE validator to continue", s.port)

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
				s.logger.Error().Err(err).Msg("Failed to accept connection")
				continue
			}

			go s.handleConnection(conn)
		}
	}
}

// handleConnection handles a new failover connection
func (s *Server) handleConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "connection closed")

	s.logger.Debug().Str("remote_addr", conn.RemoteAddr().String()).Msg("Accepted new connection")
	s.activeConn = conn

	// Accept streams
	for {
		stream, err := conn.AcceptStream(s.ctx)
		if err != nil {
			s.logger.Debug().Str("remote_addr", conn.RemoteAddr().String()).Err(err).Msg("Failed to accept stream")
			return
		}

		s.logger.Debug().Str("remote_addr", conn.RemoteAddr().String()).Msg("Accepted new stream")
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
			s.logger.Debug().Msg("Stream closed by peer")
			return
		}
		s.logger.Debug().Msgf("Failed to read message type: %v", err)
		return
	}

	switch msgType[0] {
	case MessageTypeFailoverInitiateRequest: // failover
		s.logger.Debug().Msgf("Received failover initiate request")
		s.handleFailoverStream(stream)
	default:
		s.logger.Error().Msgf("Unknown message type: %d - ignoring stream", msgType[0])
	}
}

func (s *Server) handleFailoverStream(stream *quic.Stream) {
	// read the message and parse it into a Stream struct
	s.failoverStream = NewFailoverStream(stream)
	if s.failoverStream.Decode() != nil {
		return
	}

	// set the is dry run failover flag
	s.failoverStream.SetIsDryRunFailover(s.isDryRunFailover)

	// set the skip tower sync flag
	s.failoverStream.SetSkipTowerSync(s.skipTowerSync)

	// set this node's info so subsequent responses can be sent to the client with it
	s.failoverStream.SetPassiveNodeInfo(s.passiveNodeInfo)

	// ensure client and this server are using the same version of solana-validator-failover
	clientVersion := s.failoverStream.GetActiveNodeInfo().SolanaValidatorFailoverVersion
	serverVersion := pkgconstants.AppVersion

	s.logger.Debug().
		Str("server_version", serverVersion).
		Str("client_version", clientVersion).
		Msg("checking for client and server version mismatch")

	if clientVersion != serverVersion {
		s.failoverStream.LogErrorWithSetMessagef("Server (%s) and client (%s) version mismatch", serverVersion, clientVersion)
		if err := s.failoverStream.Encode(); err != nil {
			s.logger.Error().Err(err).Msg("failed to send error message to client")
		}
		s.logger.Fatal().Msg("Server and client running different versions of this program - aborting")
		return
	}

	// query gossip for client by its public IP
	s.logger.Debug().Msgf("querying gossip for active node IP %s", s.failoverStream.GetActiveNodeInfo().PublicIP)
	gossipActiveNode, err := s.solanaRPCClient.NodeFromIP(s.failoverStream.GetActiveNodeInfo().PublicIP)
	if err != nil {
		s.failoverStream.LogErrorWithSetMessagef("Failed to validate active node: %v", err)
		if s.failoverStream.Encode() != nil {
			return
		}
		return
	}

	// ensure the failover request comes from the active node
	if gossipActiveNode.IP() != s.failoverStream.GetActiveNodeInfo().PublicIP {
		s.failoverStream.LogErrorWithSetMessagef(
			"Failed to validate active node: active node IP %s does not match expected IP %s",
			gossipActiveNode.IP(),
			s.failoverStream.GetActiveNodeInfo().PublicIP,
		)
		if s.failoverStream.Encode() != nil {
			return
		}
		return
	}

	// confirm the failover with the user
	// Get RPC URLs from the messages passed between client and server
	activeRPCURL := s.failoverStream.GetActiveNodeInfo().RPCAddress
	passiveRPCURL := s.failoverStream.GetPassiveNodeInfo().RPCAddress

	if err := s.failoverStream.ConfirmFailover(s.hooks, activeRPCURL, passiveRPCURL, s.autoConfirm); err != nil {
		s.logger.Error().Err(err).Msg("failover cancelled")

		// Send error message to client before exiting
		s.failoverStream.SetErrorMessagef("server cancelled failover: %v", err)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error().Err(encodeErr).Msg("Failed to send error message to client")
		}

		// close the server listener and cancel the context to stop accepting new connections
		if s.listener != nil {
			if err := s.listener.Close(); err != nil {
				s.logger.Error().Err(err).Msg("failed to close listener")
			}
		}
		if s.transport != nil {
			if err := s.transport.Close(); err != nil {
				s.logger.Error().Err(err).Msg("failed to close transport")
			}
		}
		s.cancel()
		os.Exit(1)
	}

	// take initial sample of vote credits and rank for the active key - use it to compare later
	s.logger.Debug().Msg("Pulling pre-failover vote credits sample...")
	err = s.failoverStream.PullActiveIdentityVoteCreditsSample(s.solanaRPCClient)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to pull active identity vote credits sample")
		s.failoverStream.SetErrorMessagef("server failed to pull active identity vote credits sample: %v", err)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error().Err(encodeErr).Msg("Failed to send error message to client")
		}
		return
	}

	// this is where the actual failover starts

	var towerFile *os.File
	// if skip tower sync is enabled, remove tower file if it exists
	if s.skipTowerSync {
		if utils.FileExists(s.failoverStream.GetPassiveNodeInfo().TowerFile) {
			s.logger.Info().Msgf("removing existing tower file at %s", s.failoverStream.GetPassiveNodeInfo().TowerFile)
			if err := utils.RemoveFile(s.failoverStream.GetPassiveNodeInfo().TowerFile); err != nil {
				s.failoverStream.SetErrorMessagef("failed to remove tower file at %s: %v", s.failoverStream.GetPassiveNodeInfo().TowerFile, err)
				if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
					s.logger.Error().Err(encodeErr).Msg("Failed to send error message to client")
				}
				s.logger.Fatal().Err(err).Msgf("failed to remove tower file at %s", s.failoverStream.GetPassiveNodeInfo().TowerFile)
				return
			}
		}
	} else {
		// Open tower file handle early to speed up failover
		var err error
		towerFile, err = os.OpenFile(
			s.failoverStream.GetPassiveNodeInfo().TowerFile,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
			os.FileMode(0644), // User and group can read/write, others can read
		)
		if err != nil {
			s.logger.Error().Err(err).Msgf("failed to open tower file %s", s.failoverStream.GetPassiveNodeInfo().TowerFile)
			s.failoverStream.SetErrorMessagef("server failed to open its tower file %s: %v", s.failoverStream.GetPassiveNodeInfo().TowerFile, err)
			if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
				s.logger.Error().Err(encodeErr).Msg("Failed to send error message to client")
			}
			return
		}
		defer utils.SafeCloseFile(towerFile)
	}

	// run pre hooks when passive
	err = s.hooks.RunPreWhenPassive(s.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: s.isDryRunFailover,
		isPreFailover:    true,
	}))
	if err != nil {
		s.failoverStream.SetErrorMessagef("server failed to run its pre-failover hooks: %v", err)
		if encodeErr := s.failoverStream.Encode(); encodeErr != nil {
			s.logger.Error().Err(encodeErr).Msg("Failed to send error message to client")
		}
		s.logger.Fatal().Err(err).Msg("failed to run pre hooks when passive")
		return
	}

	// set can proceed to true
	s.failoverStream.SetCanProceed(true)
	if s.failoverStream.Encode() != nil {
		return
	}

	if s.skipTowerSync {
		s.logger.Info().Msgf("🟤 Failover started - skipping tower file sync")
	} else {
		s.logger.Info().Msgf("🟤 Failover started - waiting for tower file from %s", s.failoverStream.GetActiveNodeInfo().Hostname)

		// Wait for the updated node info with tower file bytes
		if err := s.failoverStream.Decode(); err != nil {
			s.logger.Error().Err(err).Msg("failed to decode updated node info")
			return
		}

		// check that the TowerFileBytes sent are the same as the hash of the tower file
		computedTowerFileHash := s.failoverStream.GetActiveNodeInfo().ComputeTowerFileHashFromBytes(s.failoverStream.GetActiveNodeInfo().TowerFileBytes)
		expectedTowerFileHash := s.failoverStream.GetActiveNodeInfo().TowerFileHash

		s.logger.Debug().Msgf("Checking tower file hash - received: %s expected: %s", computedTowerFileHash, expectedTowerFileHash)

		if computedTowerFileHash != expectedTowerFileHash {
			s.logger.Error().Msgf("tower file hash mismatch: (got: %s) != (expected: %s)", computedTowerFileHash, expectedTowerFileHash)
			s.logger.Error().Msg("aborting failover - save it by running:")
			fmt.Printf(
				"  rsync -avz --no-perms --no-i-r --no-progress --no-motd --no-times -e ssh -i <YOUR-SSH-KEY> -o PubkeyAcceptedKeyTypes=+ssh-ed25519 -o HostKeyAlgorithms=+ssh-ed25519 -o BatchMode=yes -o StrictHostKeyChecking=no %s@%s:%s %s \n",
				os.Getenv("USER"),
				s.failoverStream.GetActiveNodeInfo().Hostname,
				s.failoverStream.GetActiveNodeInfo().TowerFile,
				s.failoverStream.GetPassiveNodeInfo().TowerFile,
			)
			s.logger.Error().Msg("then run:")
			fmt.Printf("  %s \n", s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand)
			s.logger.Fatal().Msg("something has turned to 💩")
			return
		}

		// Write bytes and close immediately
		if _, err := towerFile.Write(s.failoverStream.GetActiveNodeInfo().TowerFileBytes); err != nil {
			s.logger.Error().Err(err).Msgf("failed to write tower file to %s", s.failoverStream.GetPassiveNodeInfo().TowerFile)
			return
		}

		// close the file handle - defer utils.SafeCloseFile() above won't conflict
		if err := towerFile.Close(); err != nil {
			s.logger.Error().Err(err).Msgf("failed to close tower file %s", s.failoverStream.GetPassiveNodeInfo().TowerFile)
			return
		}

		s.failoverStream.SetPassiveNodeSyncTowerFileEndTime()
		s.logger.Info().Msg("👉 Received tower file")
	}

	// set identity to active
	dryRunPrefix := " "
	if s.isDryRunFailover {
		dryRunPrefix = " (dry run) "
	}
	s.logger.Info().
		Str("command", s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand).
		Msgf("👉%sSetting identity to %s - %s",
			dryRunPrefix,
			style.RenderActiveString(strings.ToUpper(constants.NodeRoleActive), false),
			style.RenderActiveString(s.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey(), false),
		)

	s.failoverStream.SetPassiveNodeSetIdentityStartTime()

	err = utils.RunCommand(utils.RunCommandParams{
		CommandSlice: strings.Split(s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand, " "),
		DryRun:       s.isDryRunFailover,
		LogDebug:     s.logger.Debug().Enabled(),
	})
	if err != nil {
		s.logger.Fatal().Err(err).Msgf("failed to set identity to active with command: %s", s.failoverStream.GetPassiveNodeInfo().SetIdentityCommand)
	}

	s.failoverStream.SetPassiveNodeSetIdentityEndTime()

	// get the current slot and record it - sometimes rpc will be a slot behind, if so, assume same-slot
	failoverEndSlot, err := s.solanaRPCClient.GetCurrentSlot()
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to get current slot")
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

	// failover is complete, timings will be reported in the main failover stream
	s.logger.Info().Msg("🟢 Failover complete:")
	fmt.Println(s.failoverStream.GetStateTable())

	// run post hooks when active
	s.hooks.RunPostWhenActive(s.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: s.isDryRunFailover,
		isPostFailover:   true,
	}))

	s.logger.Info().Msg("🕐 Failover timing summary:")
	fmt.Println(s.failoverStream.GetFailoverDurationTableString())

	if !s.isDryRunFailover {
		s.confirmGossipNodesPostFailover()
	}

	// monitor the credits by pulling configured samples
	s.logger.Info().Msg("🩺 Monitoring vote credits post-failover...")
	err = s.failoverStream.PullActiveIdentityVoteCreditsSamples(s.solanaRPCClient, s.monitorConfig.CreditSamples.Count, s.monitorConfig.CreditSamples.IntervalDuration)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to pull active identity vote credits samples")
		return
	}

	// report the credit samples difference
	rankDifference, firstRank, lastRank, err := s.failoverStream.GetVoteCreditRankDifference()
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to get vote credit rank difference")
		return
	}
	s.logger.Info().Msgf("🏁 Vote credit rank change: %d (%d -> %d)", rankDifference, firstRank, lastRank)

	// close the stream and connection cleanly
	if err := stream.Close(); err != nil {
		s.logger.Error().Err(err).Msg("failed to close stream")
	}
	if err := s.activeConn.CloseWithError(quic.ApplicationErrorCode(0), "failover complete"); err != nil {
		s.logger.Debug().Msgf("closing connection after successful failover: %v", err)
	}

	// close the server listener and cancel the context to stop accepting new connections
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.logger.Error().Err(err).Msg("failed to close listener")
		}
	}
	if s.transport != nil {
		if err := s.transport.Close(); err != nil {
			s.logger.Error().Err(err).Msg("failed to close transport")
		}
	}
	s.cancel()
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

	sp := spinner.New().Title("confirming gossip nodes switched roles...")
	sp.ActionWithErr(func(ctx context.Context) error {
		maxRetries := 5
		retryCount := 0
		// it can take a few seconds for gossip to update so try to refresh gossip identities a few times before claiming error
		for retryCount < maxRetries {
			retryDelay := time.Duration(1<<(retryCount+1)) * time.Second
			retryCount++
			hasRetriesLeft := retryCount < maxRetries

			// active node is now the old passive node
			solanaActiveNode, err = s.solanaRPCClient.NodeFromIP(s.failoverStream.GetPassiveNodeInfo().PublicIP)
			if err != nil && hasRetriesLeft {
				sp.Title(style.RenderWarningStringf("(attempt %d of %d) failed to refresh active node info from gossip - retrying", retryCount, maxRetries))
				time.Sleep(retryDelay)
				continue
			}
			if err != nil && !hasRetriesLeft {
				sp.Title(style.RenderErrorStringf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries))
				s.logger.Error().Err(err).Msgf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries)
				return fmt.Errorf("(attempt %d of %d) failed to refresh active node info from gossip - giving up", retryCount, maxRetries)
			}

			// passive node is now the old active node
			solanaPassiveNode, err = s.solanaRPCClient.NodeFromIP(s.failoverStream.GetActiveNodeInfo().PublicIP)
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
		s.logger.Error().Err(err).Msg("failed to confirm gossip nodes switched roles - potentially serious shit - investigate immediately")
	}

	if isActiveNodeKeySwitchReflectedInGossip && isPassiveNodeKeySwitchReflectedInGossip {
		s.logger.Info().Msg("Gossip confirms nodes switched roles successfully")
	} else {
		s.logger.Error().Msg("Gossip does not confirm role switch")
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
	envMap["THIS_NODE_RPC_ADDRESS"] = s.rpcURL

	// peer node is active
	envMap["PEER_NODE_NAME"] = s.failoverStream.GetActiveNodeInfo().Hostname
	envMap["PEER_NODE_PUBLIC_IP"] = s.failoverStream.GetActiveNodeInfo().PublicIP
	envMap["PEER_NODE_ACTIVE_IDENTITY_PUBKEY"] = s.failoverStream.GetActiveNodeInfo().Identities.Active.PubKey()
	envMap["PEER_NODE_PASSIVE_IDENTITY_PUBKEY"] = s.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey()
	envMap["PEER_NODE_CLIENT_VERSION"] = s.failoverStream.GetActiveNodeInfo().ClientVersion

	return
}
