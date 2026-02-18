package failover

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/charmbracelet/huh/spinner"
	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
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

// ClientConfig is the configuration for the failover client, client is always the active node
type ClientConfig struct {
	ServerName                     string
	ServerAddress                  string
	ActiveNodeInfo                 *NodeInfo
	MinTimeToLeaderSlot            time.Duration
	WaitMinTimeToLeaderSlotEnabled bool
	Hooks                          hooks.FailoverHooks
	LocalRPCClient                 *rpc.Client
	SolanaRPCClient                solana.ClientInterface
	RPCURL                         string
	SkipTowerSync                  bool
}

// Client is the failover client - an active node connects to a passive node server to handover as active
type Client struct {
	Conn                           *quic.Conn
	ctx                            context.Context
	cancel                         context.CancelFunc
	logger                         zerolog.Logger
	activeNodeInfo                 *NodeInfo
	failoverStream                 *Stream
	hooks                          hooks.FailoverHooks
	minTimeToLeaderSlot            time.Duration
	waitMinTimeToLeaderSlotEnabled bool
	localRPCClient                 *rpc.Client
	solanaRPCClient                solana.ClientInterface
	rpcURL                         string
	serverName                     string
	serverAddress                  string
	skipTowerSync                  bool
}

// NewClientFromConfig creates a new QUIC client from a configuration
func NewClientFromConfig(config ClientConfig) (client *Client, err error) {
	ctx, cancel := context.WithCancel(context.Background())

	client = &Client{
		logger:                         log.With().Logger(),
		ctx:                            ctx,
		cancel:                         cancel,
		activeNodeInfo:                 config.ActiveNodeInfo,
		hooks:                          config.Hooks,
		minTimeToLeaderSlot:            config.MinTimeToLeaderSlot,
		waitMinTimeToLeaderSlotEnabled: config.WaitMinTimeToLeaderSlotEnabled,
		localRPCClient:                 config.LocalRPCClient,
		solanaRPCClient:                config.SolanaRPCClient,
		rpcURL:                         config.RPCURL,
		serverName:                     config.ServerName,
		serverAddress:                  config.ServerAddress,
		skipTowerSync:                  config.SkipTowerSync,
	}

	err = client.connectToServer()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	client.logger.Debug().Msgf("Connected to %s", style.RenderPassiveString(config.ServerName, false))

	return client, nil
}

// Start starts the QUIC client
func (c *Client) Start() {
	c.logger.Debug().Msg("Starting QUIC client")

	// open a bidirectional stream to the server
	stream, err := c.Conn.OpenStreamSync(c.ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("Failed to open stream")
		return
	}

	c.logger.Debug().Msg("Opened stream to server")

	// send FailoverInitiateRequest
	c.failoverStream = NewFailoverStream(stream)

	// Send message type first
	if _, err := c.failoverStream.Stream.Write([]byte{MessageTypeFailoverInitiateRequest}); err != nil {
		c.logger.Error().Err(err).Msg("Failed to send message type")
		return
	}

	// send message with your own info
	c.failoverStream.SetActiveNodeInfo(c.activeNodeInfo)
	err = c.failoverStream.Encode()
	if err != nil {
		return
	}

	c.logger.Debug().Msg("Sent message type")

	// wait for failover signal from server before proceeding
	sp := spinner.New().Title(fmt.Sprintf("Connected to %s, waiting for failover signal...", style.RenderPassiveString(c.serverName, false)))
	sp.ActionWithErr(func(ctx context.Context) error {
		return c.failoverStream.Decode()
	})
	err = sp.Run()
	if err != nil {
		c.logger.Fatal().Err(err).Msg("failed to wait for failover signal")
		return
	}

	// ensure server is running the same version of this program
	serverVersion := c.failoverStream.GetPassiveNodeInfo().SolanaValidatorFailoverVersion
	clientVersion := pkgconstants.AppVersion
	if serverVersion != clientVersion {
		c.logger.Fatal().Msgf("server is running a different version of this program: %s (them) != %s (us)", serverVersion, clientVersion)
		return
	}

	// see if the server says can proceed, else show error message and exit
	if !c.failoverStream.GetCanProceed() {
		c.logger.Fatal().Msg(c.failoverStream.GetErrorMessage())
		return
	}

	// Get skipTowerSync from the server's message (server is the authority on this)
	skipTowerSync := c.failoverStream.GetSkipTowerSync()

	// wait until the next leader slot is at least the minimum time to leader slot
	err = c.waitMinTimeToLeaderSlot()
	if err != nil {
		c.logger.Fatal().Err(err).Msg("failed to wait for next leader slot")
		return
	}

	// run pre hooks when active
	err = c.hooks.RunPreWhenActive(c.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
		isPreFailover:    true,
	}))
	if err != nil {
		c.logger.Fatal().Err(err).Msg("failed to run pre hooks when active")
		return
	}

	c.logger.Info().Msg("🟢 Failover started")

	// wait until the next slot starts so we switch right at the beginning of the next slot
	// this ensures we're early in the slot when we start the switch
	slot, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		c.logger.Fatal().Err(err).Msgf("failed to wait for next slot to start")
		return
	}

	// set the failover start slot to the current slot (we're now early in this slot)
	c.failoverStream.SetFailoverStartSlot(slot)

	// set identity to passive
	dryRunPrefix := " "
	if c.failoverStream.GetIsDryRunFailover() {
		dryRunPrefix = " (dry run) "
	}
	c.logger.Info().
		Str("command", c.failoverStream.GetActiveNodeInfo().SetIdentityCommand).
		Msgf("👉%sSetting identity to %s - %s",
			dryRunPrefix,
			style.RenderPassiveString(strings.ToUpper(constants.NodeRolePassive), false),
			style.RenderPassiveString(c.failoverStream.GetActiveNodeInfo().Identities.Passive.PubKey(), false),
		)

	c.failoverStream.SetActiveNodeSetIdentityStartTime()

	err = utils.RunCommand(utils.RunCommandParams{
		CommandSlice: strings.Split(c.failoverStream.GetActiveNodeInfo().SetIdentityCommand, " "),
		DryRun:       c.failoverStream.GetIsDryRunFailover(),
		LogDebug:     c.logger.Debug().Enabled(),
	})
	if err != nil {
		c.logger.Error().Err(err).Msgf("failed to set identity to passive")
		return
	}
	c.failoverStream.SetActiveNodeSetIdentityEndTime()

	if skipTowerSync {
		c.logger.Info().Msgf("⏭️  Skipping tower file sync")
		// Don't send anything - server won't wait for tower file when skipTowerSync is true
	} else {
		c.logger.Info().Msgf("👉 Sending tower file to %s", style.RenderPassiveString(c.failoverStream.GetPassiveNodeInfo().Hostname, false))

		// Read the tower file into TowerFileBytes
		c.failoverStream.SetActiveNodeSyncTowerFileStartTime()
		err = c.failoverStream.GetActiveNodeInfo().SetTowerFileBytes()
		if err != nil {
			c.logger.Error().Err(err).Msgf("failed to set tower file bytes for %s", c.failoverStream.GetActiveNodeInfo().TowerFile)
			return
		}
		c.failoverStream.SetActiveNodeSyncTowerFileEndTime()

		// Send the updated node info with tower file bytes
		if err := c.failoverStream.Encode(); err != nil {
			c.logger.Error().Err(err).Msgf("failed to send tower file bytes for %s", c.failoverStream.GetActiveNodeInfo().TowerFile)
			return
		}
	}

	// wait for confirmation from server that failover is complete
	err = c.failoverStream.Decode()
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to decode failover stream")
		return
	}

	// send a message to the server to confirm we're proceeding
	if !c.failoverStream.GetIsSuccessfullyCompleted() {
		c.logger.Error().Msgf("server failed to complete failover: %s", c.failoverStream.GetErrorMessage())
		return
	}

	c.logger.Info().Msg("🟤 Failover complete")

	// run post hooks now this is passive and active node says all is peachy
	c.hooks.RunPostWhenPassive(c.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
		isPostFailover:   true,
	}))
}

// waitUntilStartOfNextSlot waits until the start of the next slot
// this is important to try to start a failover early in the slot to avoid missing it
// It polls getSlot() to detect when the slot changes and returns the new slot number,
// which naturally gets us well within the first half of the new slot
// should get us in within the first 50-100ms of the next slot
func (c *Client) waitUntilStartOfNextSlot() (newSlot uint64, err error) {
	c.logger.Debug().Msg("Waiting until start of next slot")

	// Get the current slot number
	currentSlot, err := c.solanaRPCClient.GetCurrentSlot()
	if err != nil {
		return 0, fmt.Errorf("failed to get current slot: %w", err)
	}

	// Poll getSlot() to detect when the slot changes
	// This is fast and accurate - getSlot() is a lightweight RPC call
	pollInterval := 50 * time.Millisecond // poll every 50ms
	for {
		slot, err := c.solanaRPCClient.GetCurrentSlot()
		if err != nil {
			// If RPC fails, retry after a short delay
			c.logger.Debug().Err(err).Msg("Failed to get slot, retrying")
			time.Sleep(pollInterval)
			continue
		}

		// Slot has changed, we're now in the next slot
		if slot > currentSlot {
			c.logger.Debug().
				Uint64("old_slot", currentSlot).
				Uint64("new_slot", slot).
				Msg("Slot transition detected, proceeding")
			return slot, nil
		}

		// Still in the same slot, continue polling
		time.Sleep(pollInterval)
	}
}

// waitMinTimeToLeaderSlot waits until the next leader slot is at least the minimum time to leader slot
func (c *Client) waitMinTimeToLeaderSlot() (err error) {
	if !c.waitMinTimeToLeaderSlotEnabled {
		c.logger.Debug().Msg("Waiting for min time to leader slot is disabled, skipping")
		return nil
	}

	c.logger.Debug().Msgf("Ensuring next leader slot is at least %s in the future", c.minTimeToLeaderSlot.String())
	sp := spinner.New().TitleStyle(style.SpinnerTitleStyle).Title("Checking next leader slot...")
	maxRetries := 10
	var calculatedTimeToNextLeaderSlot time.Duration
	sp.ActionWithErr(func(ctx context.Context) error {
		sleepDuration := 2 * time.Second
		pubkey, err := solanago.PublicKeyFromBase58(c.activeNodeInfo.Identities.Active.PubKey())
		if err != nil {
			return fmt.Errorf("failed to parse active identity pubkey: %w", err)
		}
		remainingRetries := maxRetries
		stringMinTimeToLeaderSlot := c.minTimeToLeaderSlot.Round(time.Second).String()

		for {
			isOnLeaderSchedule, timeToNextLeaderSlot, err := c.solanaRPCClient.GetTimeToNextLeaderSlotForPubkey(pubkey)
			if err != nil {
				if remainingRetries == 0 {
					return fmt.Errorf("failed to get time to next leader slot: %w", err)
				}
				log.Debug().Err(err).Msgf("failed to get time to next leader slot")
				sp.Title(style.RenderErrorStringf(
					"Failed to get time to next leader slot, retrying in %s (%d retries left): %s",
					sleepDuration.String(),
					remainingRetries,
					err.Error(),
				))
				remainingRetries--
				time.Sleep(sleepDuration)
				continue
			}

			if !isOnLeaderSchedule {
				sp.Title(style.RenderActiveString("This validator is not on the leader schedule, skipping wait for next leader slot to pass", false))
				return nil
			}

			stringTimeToNextLeaderSlot := timeToNextLeaderSlot.Round(time.Second).String()

			if timeToNextLeaderSlot < c.minTimeToLeaderSlot {
				// show duration as human readable time until leader slot
				sp.Title(style.RenderActiveString(
					fmt.Sprintf("Next leader slot in %s, waiting for it before proceeding...",
						stringTimeToNextLeaderSlot),
					false,
				))
				time.Sleep(sleepDuration)
				continue
			}

			calculatedTimeToNextLeaderSlot = timeToNextLeaderSlot
			sp.Title(style.RenderActiveString(
				fmt.Sprintf("Next leader slot in %s > %s, proceeding...",
					stringTimeToNextLeaderSlot,
					stringMinTimeToLeaderSlot,
				),
				false,
			))
			time.Sleep(sleepDuration)
			return nil
		}
	})

	err = sp.Run()
	if err != nil {
		return fmt.Errorf("failed to wait for next leader slot: %w", err)
	}

	if calculatedTimeToNextLeaderSlot > 0 {
		c.logger.Info().Msgf("Time to next leader slot %s", calculatedTimeToNextLeaderSlot.Round(time.Second).String())
	} else {
		c.logger.Info().Msg("No upcoming leader slots found")
	}

	return nil
}

// getEnvMap returns a map of environment variables to pass to the hooks
func (c *Client) getHookEnvMap(params hookEnvMapParams) (envMap map[string]string) {
	envMap = map[string]string{}

	envMap["IS_DRY_RUN_FAILOVER"] = fmt.Sprintf("%t", params.isDryRunFailover)

	// this node is active
	if params.isPreFailover {
		envMap["THIS_NODE_ROLE"] = constants.NodeRoleActive
		envMap["PEER_NODE_ROLE"] = constants.NodeRolePassive
	}

	// only show switch to passive
	if params.isPostFailover {
		envMap["THIS_NODE_ROLE"] = constants.NodeRolePassive
		envMap["PEER_NODE_ROLE"] = constants.NodeRoleActive
	}

	// this node is active
	envMap["THIS_NODE_NAME"] = c.activeNodeInfo.Hostname
	envMap["THIS_NODE_PUBLIC_IP"] = c.activeNodeInfo.PublicIP
	envMap["THIS_NODE_ACTIVE_IDENTITY_PUBKEY"] = c.activeNodeInfo.Identities.Active.PubKey()
	envMap["THIS_NODE_ACTIVE_IDENTITY_KEYPAIR_FILE"] = c.activeNodeInfo.Identities.Active.KeyFile
	envMap["THIS_NODE_PASSIVE_IDENTITY_PUBKEY"] = c.activeNodeInfo.Identities.Passive.PubKey()
	envMap["THIS_NODE_PASSIVE_IDENTITY_KEYPAIR_FILE"] = c.activeNodeInfo.Identities.Passive.KeyFile
	envMap["THIS_NODE_CLIENT_VERSION"] = c.activeNodeInfo.ClientVersion
	envMap["THIS_NODE_RPC_ADDRESS"] = c.rpcURL

	// peer node
	envMap["PEER_NODE_NAME"] = c.failoverStream.GetPassiveNodeInfo().Hostname
	envMap["PEER_NODE_PUBLIC_IP"] = c.failoverStream.GetPassiveNodeInfo().PublicIP
	envMap["PEER_NODE_ACTIVE_IDENTITY_PUBKEY"] = c.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey()
	envMap["PEER_NODE_PASSIVE_IDENTITY_PUBKEY"] = c.failoverStream.GetPassiveNodeInfo().Identities.Passive.PubKey()
	envMap["PEER_NODE_CLIENT_VERSION"] = c.failoverStream.GetPassiveNodeInfo().ClientVersion

	return envMap
}

// connectToServer waits until a QUIC server is listening on the given address
// It shows a spinner and attempts the actual QUIC connection, retrying on error until successful
// This allows the client to start independet of the server being ready to accept connections and latches
// onto the server as soon as it is ready
func (c *Client) connectToServer() error {
	sp := spinner.New().Title(fmt.Sprintf("Waiting for %s at %s...",
		style.RenderPassiveString(c.serverName, false),
		style.RenderGreyString(c.serverAddress, false),
	))
	sp.ActionWithErr(func(spinnerCtx context.Context) error {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		// Check immediately first, but give spinner a moment to render
		select {
		case <-spinnerCtx.Done():
			return spinnerCtx.Err()
		case <-time.After(100 * time.Millisecond):
			// Small delay to let spinner render
			if err := c.tryQUICConnection(); err == nil {
				return nil
			}
		}

		for {
			select {
			case <-spinnerCtx.Done():
				return spinnerCtx.Err()
			case <-ticker.C:
				// Try the actual QUIC connection
				if err := c.tryQUICConnection(); err == nil {
					return nil
				}
				// Server not ready yet, continue waiting
			}
		}
	})
	return sp.Run()
}

// tryQUICConnection attempts the actual QUIC connection that will be used.
// It uses a basicPacketConn wrapper to avoid quic-go's OOB (recvmsg/sendmsg)
// optimizations that fail on virtual network interfaces like Tailscale/WireGuard.
func (c *Client) tryQUICConnection() error {
	udpAddr, err := net.ResolveUDPAddr("udp4", c.serverAddress)
	if err != nil {
		c.logger.Debug().Err(err).Str("address", c.serverAddress).Msg("failed to resolve server address")
		return err
	}

	wrapped, err := newBasicPacketConn(":0")
	if err != nil {
		c.logger.Debug().Err(err).Msg("failed to create UDP socket")
		return err
	}

	tr := &quic.Transport{Conn: wrapped}
	conn, err := tr.Dial(c.ctx, udpAddr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ProtocolName},
	}, nil)
	if err != nil {
		tr.Close()
		c.logger.Debug().Err(err).Str("address", c.serverAddress).Msg("QUIC server not ready, retrying...")
		return err
	}
	// Connection successful, store it
	c.Conn = conn
	return nil
}
