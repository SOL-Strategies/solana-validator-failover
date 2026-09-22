package failover

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/log"
	solanago "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/quic-go/quic-go"
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
	Rollback                       hooks.RollbackConfig
	AutoConfirm                    bool
	FallbackWaitSlots              uint64
	// TLSConfig is an optional mTLS config. When non-nil, the client presents its
	// certificate to the server and verifies the server's certificate against the CA.
	// When nil, server certificate verification is skipped (InsecureSkipVerify).
	TLSConfig           *tls.Config
	HandoffTimeout      time.Duration
	HandoffPollInterval time.Duration
}

type identityTransitionFailedError struct {
	message string
}

func (e *identityTransitionFailedError) Error() string {
	return e.message
}

// Client is the failover client - an active node connects to a passive node server to handover as active
type Client struct {
	Conn                           *quic.Conn
	ctx                            context.Context
	cancel                         context.CancelFunc
	logger                         *log.Logger
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
	rollback                       hooks.RollbackConfig
	tlsConfig                      *tls.Config // non-nil when mTLS is enabled
	handoffTimeout                 time.Duration
	handoffPollInterval            time.Duration
	autoConfirm                    bool
	fallbackWaitSlots              uint64
	failure                        error
}

// NewClientFromConfig creates a new QUIC client from a configuration
func NewClientFromConfig(config ClientConfig) (client *Client, err error) {
	ctx, cancel := context.WithCancel(context.Background())

	var clientTLSConfig *tls.Config
	if config.TLSConfig != nil {
		cloned := config.TLSConfig.Clone()
		cloned.NextProtos = []string{ProtocolName}
		clientTLSConfig = cloned
	}

	client = &Client{
		logger:                         log.Default(),
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
		rollback:                       config.Rollback,
		tlsConfig:                      clientTLSConfig,
		handoffTimeout:                 config.HandoffTimeout,
		handoffPollInterval:            config.HandoffPollInterval,
		autoConfirm:                    config.AutoConfirm,
		fallbackWaitSlots:              config.FallbackWaitSlots,
	}

	err = client.connectToServer()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	client.logger.Debugf("connected to %s", style.RenderPassiveString(config.ServerName, false))

	return client, nil
}

func handoffTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 2 * time.Minute
	}
	return timeout
}

// waitForLocalIdentity polls through the handoff deadline because identity
// changes can take a short time to become visible through local RPC.
func waitForLocalIdentity(ctx context.Context, rpcClient solana.ClientInterface, expected string, poll time.Duration) error {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	var lastErr error
	for {
		identity, err := rpcClient.GetLocalIdentity(ctx)
		if err == nil && identity == expected {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("reported identity %s, expected %s", identity, expected)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
}

func confirmSlotFallback(autoConfirm bool, waitSlots uint64) error {
	if waitSlots == 0 {
		waitSlots = nativeFallbackWaitSlots
	}
	if autoConfirm {
		return nil
	}
	confirmed := false
	err := huh.NewForm(huh.NewGroup(huh.NewConfirm().
		Title(fmt.Sprintf("identityTransitionStatus failed; wait %d finalized slots before activating native Firedancer?", waitSlots)).
		Value(&confirmed))).Run()
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("%d-slot fallback declined", waitSlots)
	}
	return nil
}

func negotiatedFallbackWaitSlots(stream *Stream, configured uint64) uint64 {
	if waitSlots := stream.GetFallbackWaitSlots(); waitSlots > 0 {
		return waitSlots
	}
	if configured > 0 {
		return configured
	}
	return nativeFallbackWaitSlots
}

func waitForIdentityTransition(ctx context.Context, client solana.IdentityTransitionClient, sequence uint64, expectedIdentity, expectedVoteAccount string, poll time.Duration) (uint64, error) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	var lastErr error
	for {
		status, err := client.GetIdentityTransitionStatus(ctx)
		if err == nil {
			if status.Sequence >= sequence && status.State == "failed" {
				message := "identity transition failed"
				if status.Error != nil && *status.Error != "" {
					message += ": " + *status.Error
				}
				return 0, &identityTransitionFailedError{message: message}
			}
			if status.Sequence >= sequence && status.State == "complete" && status.CurrentIdentity == expectedIdentity &&
				(status.ToIdentity == "" || status.ToIdentity == expectedIdentity) &&
				(expectedVoteAccount == "" || status.VoteAccount == "" || status.VoteAccount == expectedVoteAccount) {
				return status.LastVoteSlot, nil
			}
			lastErr = fmt.Errorf("identity transition status is not complete for %s (sequence=%d state=%s)", expectedIdentity, status.Sequence, status.State)
		} else {
			lastErr = err
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, lastErr
		case <-timer.C:
		}
	}
}

func waitForFinalizedVoteAccount(ctx context.Context, client solana.ClientInterface, voteAccount, expectedNode string, targetSlot uint64, poll time.Duration, progress func(lastVote uint64, err error)) error {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	var lastVote uint64
	var lastErr error
	for {
		account, err := client.GetVoteAccountState(ctx, voteAccount, false, rpc.CommitmentFinalized)
		if err == nil && account != nil {
			lastErr = nil
			lastVote = account.LastVote
			if expectedNode != "" && account.NodePubkey.String() != expectedNode {
				err = fmt.Errorf("vote account %s belongs to node %s, expected source node %s", voteAccount, account.NodePubkey, expectedNode)
			} else if lastVote >= targetSlot {
				return nil
			}
		}
		if err != nil {
			lastErr = err
		}
		if progress != nil {
			progress(lastVote, lastErr)
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return fmt.Errorf("vote account %s did not reach finalized slot %d (last_vote=%d): %w", voteAccount, targetSlot, lastVote, lastErr)
			}
			return fmt.Errorf("vote account %s did not reach finalized slot %d (last_vote=%d): %w", voteAccount, targetSlot, lastVote, ctx.Err())
		case <-timer.C:
		}
	}
}

func maxSlotGap(target, observed uint64) uint64 {
	if target > observed {
		return target - observed
	}
	return 0
}

func (c *Client) setFailure(message string, failure error) {
	if failure == nil {
		c.failure = fmt.Errorf("%s", message)
		return
	}
	c.failure = fmt.Errorf("%s: %w", message, failure)
}

// Failure returns the terminal failover error, if Start encountered one.
func (c *Client) Failure() error { return c.failure }

// Start starts the QUIC client and returns a terminal handoff error when the
// protocol does not complete successfully.
func (c *Client) Start() (startErr error) {
	c.logger.Debug("starting QUIC client")
	var wentPassive bool
	completed := false
	defer func() {
		if completed {
			return
		}
		// Start now returns terminal failures to its caller instead of exiting
		// the process. Tear down the protocol session on every such path so the
		// server is not left blocked on the old stream and an in-process retry
		// cannot overlap it.
		if c.failoverStream != nil && c.failoverStream.Stream != nil {
			if err := c.failoverStream.Stream.Close(); err != nil {
				c.logger.Debug("failed to close failover stream after client failure", "err", err)
			}
		}
		if c.Conn != nil {
			if err := c.Conn.CloseWithError(quic.ApplicationErrorCode(1), "failover client stopped"); err != nil {
				c.logger.Debug("failed to close QUIC connection after client failure", "err", err)
			}
		}
		c.cancel()
		if c.failure == nil {
			message := "failover client stopped before completion"
			if c.failoverStream != nil && c.failoverStream.GetErrorMessage() != "" {
				message = c.failoverStream.GetErrorMessage()
			}
			c.setFailure(message, nil)
		}
		startErr = c.failure
	}()
	recoverBeforeDestinationActivation := func(message string, failure error) {
		c.logger.Error(message, "err", failure)
		c.setFailure(message, failure)
		if c.rollback.Enabled {
			c.logger.Warn("handoff failed before destination activation; reverting this node to active")
			if rollbackErr := RunRollbackToActive(c.rollback, c.getHookEnvMap(hookEnvMapParams{
				isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
				isPostFailover:   true,
			}), c.failoverStream.GetIsDryRunFailover(), c.logger); rollbackErr != nil {
				c.logger.Error("rollback to active failed — manual intervention required", "err", rollbackErr)
			}
			return
		}
		c.logger.Error("rollback disabled — this node is passive and destination activation has not begun; manual intervention required")
		if c.rollback.ToActive.ResolvedCmd != "" {
			c.logger.Errorf("to recover this node to active: %s", c.rollback.ToActive.ResolvedCmd)
		}
	}
	abortDestinationHandoff := func(message string, failure error) {
		reason := message
		if failure != nil {
			reason = fmt.Sprintf("%s: %v", message, failure)
		}
		c.failoverStream.SetHandoffAborted(true)
		c.failoverStream.SetErrorMessage(reason)
		if err := c.failoverStream.Encode(); err != nil {
			c.logger.Error("failed to send handoff abort to destination", "err", err)
		}
	}

	// open a bidirectional stream to the server
	stream, err := c.Conn.OpenStreamSync(c.ctx)
	if err != nil {
		c.logger.Error("failed to open stream", "err", err)
		c.setFailure("failed to open failover stream", err)
		return
	}

	c.logger.Debug("opened stream to server")

	// send FailoverInitiateRequest
	c.failoverStream = NewFailoverStream(stream)

	// Send message type first
	if _, err := c.failoverStream.Stream.Write([]byte{MessageTypeFailoverInitiateRequest}); err != nil {
		c.logger.Error("failed to send message type", "err", err)
		c.setFailure("failed to send failover request", err)
		return
	}

	// Send wire protocol version before any gob encoding so the server can
	// verify compatibility before attempting to decode the gob payload.
	if err := writeWireVersion(stream); err != nil {
		c.logger.Error("failed to send wire protocol version", "err", err)
		c.setFailure("failed to send wire protocol version", err)
		return
	}

	// Native Firedancer handoffs reconcile on-chain vote state, so resolve an
	// omitted vote account before sending initial node information. Legacy
	// tower-file handoffs do not use VoteAccount and must not acquire a new
	// cluster-RPC prerequisite.
	if c.activeNodeInfo.IsNativeFiredancer && c.activeNodeInfo.VoteAccount == "" {
		lookupCtx, lookupCancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
		resolver, supportsContext := c.solanaRPCClient.(solana.ContextVoteAccountClient)
		if !supportsContext {
			lookupCancel()
			err := fmt.Errorf("RPC client does not support context-aware vote-account lookup")
			c.logger.Error("failed to resolve source vote account before sending failover information", "err", err)
			c.setFailure("failed to resolve source vote account", err)
			return
		}
		account, _, lookupErr := resolver.GetCreditRankedVoteAccountFromPubkeyContext(lookupCtx, c.activeNodeInfo.Identities.Active.PubKey())
		lookupCancel()
		if lookupErr != nil {
			c.logger.Error("failed to resolve source vote account before sending failover information", "err", lookupErr)
			c.setFailure("failed to resolve source vote account", lookupErr)
			return
		}
		c.activeNodeInfo.VoteAccount = account.VotePubkey.String()
	}

	// send message with your own info
	c.failoverStream.SetActiveNodeInfo(c.activeNodeInfo)
	c.failoverStream.SetActiveRollbackEnabled(c.rollback.Enabled)
	err = c.failoverStream.Encode()
	if err != nil {
		c.setFailure("failed to send active node information", err)
		return
	}

	c.logger.Debug("sent message type")

	// wait for failover signal from server before proceeding
	sp := spinner.New().Title(style.RenderPinkString("connected to ") + style.RenderPassiveString(c.serverName, false) + style.RenderPinkString(", waiting for failover signal..."))
	sp.ActionWithErr(func(ctx context.Context) error {
		// Read the server's wire protocol version before any gob decoding.
		// A mismatch here means the passive node is running an incompatible version.
		if err := readAndCheckWireVersion(stream); err != nil {
			return err
		}
		return c.failoverStream.Decode()
	})
	err = sp.Run()
	if err != nil {
		c.logger.Error("failed to wait for failover signal", "err", err)
		c.setFailure("failed to wait for failover signal", err)
		return
	}

	// Probe only when the negotiated direction can require the Agave/Jito
	// transition RPC. Legacy tower-file handoffs never acquire this dependency.
	if c.failoverStream.GetProbeIdentityTransitionRPC() {
		info := c.failoverStream.GetActiveNodeInfo()
		available := false
		if probeClient, ok := c.solanaRPCClient.(solana.IdentityTransitionClient); ok {
			probeCtx, probeCancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			var probeErr error
			available, probeErr = probeClient.ProbeIdentityTransitionStatus(probeCtx)
			probeCancel()
			if probeErr != nil {
				c.logger.Warn("identity transition RPC probe failed; configured slot fallback may be required", "err", probeErr)
			}
		}
		info.IdentityTransitionRPCAvailable = available
		c.failoverStream.SetActiveNodeInfo(info)
		c.failoverStream.SetIdentityTransitionRPCAvailable(available)
		if err := c.failoverStream.Encode(); err != nil {
			c.setFailure("failed to send identity transition RPC capability", err)
			return
		}
		if err := c.failoverStream.Decode(); err != nil {
			c.setFailure("failed to receive negotiated handoff strategy", err)
			return
		}
	}

	// ensure server is running the same version of this program
	serverVersion := c.failoverStream.GetPassiveNodeInfo().SolanaValidatorFailoverVersion
	clientVersion := pkgconstants.AppVersion
	if serverVersion != clientVersion {
		versionErr := fmt.Errorf("server is running a different version of this program: %s (them) != %s (us)", serverVersion, clientVersion)
		c.logger.Error(versionErr.Error())
		c.setFailure("failover version mismatch", versionErr)
		return
	}

	// see if the server says can proceed, else show error message and exit
	if !c.failoverStream.GetCanProceed() {
		rejection := c.failoverStream.GetErrorMessage()
		if rejection == "" {
			rejection = "server rejected failover"
		}
		c.logger.Error(rejection)
		c.setFailure("server rejected failover", fmt.Errorf("%s", rejection))
		return
	}
	if command := c.failoverStream.GetActiveRollbackCommand(); command != "" {
		c.rollback.ToActive.ResolvedCmd = command
	}

	// Get skipTowerSync from the server's message (server is the authority on this)
	skipTowerSync := c.failoverStream.GetSkipTowerSync()

	// Probe native Firedancer metrics before demoting the source. The final
	// watermark is read again after the identity change, but an unavailable
	// endpoint must not strand this validator in passive state first.
	var preDemotionNativeVoteSlot uint64
	var preDemotionTransitionSequence uint64
	var preDemotionTransitionVoteSlot uint64
	var transitionClient solana.IdentityTransitionClient
	if !skipTowerSync && c.failoverStream.GetHandoffStrategy() == HandoffStrategyOnchain {
		sourceInfo := c.failoverStream.GetActiveNodeInfo()
		if sourceInfo.IsNativeFiredancer {
			metricsCtx, metricsCancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			var metricsErr error
			preDemotionNativeVoteSlot, metricsErr = ReadNativeTowerVoteSlot(metricsCtx, sourceInfo.MetricsAddress)
			metricsCancel()
			if metricsErr != nil {
				c.setFailure("failed to validate native Firedancer metrics before switching to passive", metricsErr)
				c.logger.Error("failed to validate native Firedancer metrics before switching to passive", "err", metricsErr)
				return
			}
		}
		if sourceInfo := c.failoverStream.GetActiveNodeInfo(); !sourceInfo.IsNativeFiredancer &&
			c.failoverStream.GetHandoffStrategy() == HandoffStrategyOnchain && c.failoverStream.GetIdentityTransitionRPCAvailable() {
			var ok bool
			transitionClient, ok = c.solanaRPCClient.(solana.IdentityTransitionClient)
			if !ok {
				c.setFailure("identity transition RPC was negotiated but is unavailable locally", nil)
				return
			}
			statusCtx, statusCancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			status, statusErr := transitionClient.GetIdentityTransitionStatus(statusCtx)
			statusCancel()
			if statusErr != nil {
				patchURL := sourceInfo.IdentityTransitionRPCPatchURL
				if patchURL == "" {
					patchURL = IdentityTransitionPatchURL(sourceInfo.ClientVersionRPC, pkgconstants.AppVersion)
				}
				c.logger.Warn("identityTransitionStatus is unavailable; the hosted validator patch is available at", "url", patchURL)
				waitSlots := negotiatedFallbackWaitSlots(c.failoverStream, c.fallbackWaitSlots)
				if fallbackErr := confirmSlotFallback(c.autoConfirm, waitSlots); fallbackErr != nil {
					c.setFailure("failed to read identity transition status before demotion", statusErr)
					return
				}
				c.failoverStream.SetSlotFallbackRequired(true)
				c.failoverStream.SetFallbackWaitSlots(waitSlots)
				transitionClient = nil
			} else {
				preDemotionTransitionSequence = status.Sequence
				preDemotionTransitionVoteSlot = status.LastVoteSlot
			}
		}
	}

	// wait until the next leader slot is at least the minimum time to leader slot
	err = c.waitMinTimeToLeaderSlot()
	if err != nil {
		c.logger.Error("failed to wait for next leader slot", "err", err)
		c.setFailure("failed to wait for next leader slot", err)
		return
	}

	// run pre hooks when active
	err = c.hooks.RunPreWhenActive(c.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
		isPreFailover:    true,
	}))
	if err != nil {
		c.logger.Error("failed to run pre hooks when active", "err", err)
		c.setFailure("failed to run pre hooks when active", err)
		return
	}

	c.logger.Info("failover started")

	// wait until the next slot starts so we switch right at the beginning of the next slot
	// this ensures we're early in the slot when we start the switch
	slot, err := c.waitUntilStartOfNextSlot()
	if err != nil {
		c.logger.Error("failed to wait for next slot to start", "err", err)
		c.setFailure("failed to wait for next slot to start", err)
		return
	}

	// set the failover start slot to the current slot (we're now early in this slot)
	c.failoverStream.SetFailoverStartSlot(slot)

	// set identity to passive
	dryRunPrefix := ""
	if c.failoverStream.GetIsDryRunFailover() {
		dryRunPrefix = style.RenderLightGreyString("(dry run)") + " "
	}
	c.logger.Info(dryRunPrefix +
		style.RenderPinkString("changing to ") +
		style.RenderPassiveString(constants.NodeRolePassive, false) +
		style.RenderPinkString(" identity"))

	c.failoverStream.SetActiveNodeSetIdentityStartTime()

	err = utils.RunCommand(utils.RunCommandParams{
		CommandSlice: strings.Split(c.failoverStream.GetActiveNodeInfo().SetIdentityCommand, " "),
		DryRun:       c.failoverStream.GetIsDryRunFailover(),
		LogDebug:     c.logger.GetLevel() <= log.DebugLevel,
	})
	if err != nil {
		c.logger.Error("failed to set identity to passive", "err", err)
		c.setFailure("failed to set identity to passive", err)
		return
	}
	c.failoverStream.SetActiveNodeSetIdentityEndTime()
	wentPassive = true // this node is now passive; used below for rollback/warning decisions
	if c.failoverStream.GetHandoffStrategy() == HandoffStrategyOnchain && !skipTowerSync {
		c.failoverStream.SetHandoffEvidenceStartTime()
	}

	if skipTowerSync {
		c.logger.Info("skipping tower file sync")
		// Don't send anything - server won't wait for tower file when skipTowerSync is true
	} else if c.failoverStream.GetHandoffStrategy() == HandoffStrategyOnchain {
		if !c.failoverStream.GetIsDryRunFailover() {
			identityCtx, identityCancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			identityErr := waitForLocalIdentity(identityCtx, c.solanaRPCClient, c.activeNodeInfo.Identities.Passive.PubKey(), c.handoffPollInterval)
			identityCancel()
			if identityErr != nil {
				recoverBeforeDestinationActivation("failed to verify passive identity after set-identity", identityErr)
				return
			}
		}
		sourceInfo := c.failoverStream.GetActiveNodeInfo()
		var tip uint64
		var tipErr error
		if sourceInfo.IsNativeFiredancer {
			watermarkStarted := time.Now()
			lastWatermarkProgress := watermarkStarted
			c.logger.Info("confirming native Firedancer vote watermark after demotion", "captured_slot", preDemotionNativeVoteSlot, "poll_interval", c.handoffPollInterval)
			ctx, cancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			if c.failoverStream.GetIsDryRunFailover() {
				tip, tipErr = waitForNativeTowerVoteAtLeast(ctx, sourceInfo.MetricsAddress, c.handoffPollInterval, preDemotionNativeVoteSlot)
			} else {
				_, tipErr = waitForNativeTowerVoteAtLeastWithProgress(ctx, sourceInfo.MetricsAddress, c.handoffPollInterval, preDemotionNativeVoteSlot, func(slot uint64, err error) {
					if time.Since(lastWatermarkProgress) < 5*time.Second {
						return
					}
					fields := []any{"captured_slot", preDemotionNativeVoteSlot, "latest_slot", slot, "elapsed", time.Since(watermarkStarted).Round(time.Second)}
					if err != nil {
						fields = append(fields, "err", err)
					}
					c.logger.Info("still waiting for native Firedancer vote watermark after demotion", fields...)
					lastWatermarkProgress = time.Now()
				})
				// The captured pre-demotion watermark is the safety target. The
				// metric may continue advancing while Firedancer drains/replays
				// state after set-identity, so a later metric value is not a
				// reliable frozen-vote boundary.
				tip = preDemotionNativeVoteSlot
			}
			cancel()
			if tipErr == nil {
				c.logger.Info("native Firedancer vote watermark confirmed after demotion", "frozen_slot", tip, "captured_slot", preDemotionNativeVoteSlot, "elapsed", time.Since(watermarkStarted).Round(time.Millisecond))
			}
		} else if c.failoverStream.GetSlotFallbackRequired() {
			// The server will enforce the conservative post-demotion slot barrier.
			tip = 0
		} else {
			if tipErr == nil {
				if transitionClient != nil && c.failoverStream.GetIsDryRunFailover() {
					// The identity command is intentionally not executed in a dry run,
					// so no new transition sequence can appear. Use the sampled value
					// for the plan without waiting for an impossible state change.
					tip = preDemotionTransitionVoteSlot
				} else if transitionClient != nil {
					ctx, cancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
					tip, tipErr = waitForIdentityTransition(ctx, transitionClient, preDemotionTransitionSequence, sourceInfo.Identities.Passive.PubKey(), sourceInfo.VoteAccount, c.handoffPollInterval)
					cancel()
					if tipErr != nil {
						var transitionFailedErr *identityTransitionFailedError
						if errors.As(tipErr, &transitionFailedErr) {
							c.logger.Error("identity transition failed; refusing slot fallback", "err", tipErr)
						} else {
							patchURL := sourceInfo.IdentityTransitionRPCPatchURL
							if patchURL == "" {
								patchURL = IdentityTransitionPatchURL(sourceInfo.ClientVersionRPC, pkgconstants.AppVersion)
							}
							c.logger.Warn("identityTransitionStatus did not complete; install the hosted validator patch for future fast handoffs", "url", patchURL)
							waitSlots := negotiatedFallbackWaitSlots(c.failoverStream, c.fallbackWaitSlots)
							if fallbackErr := confirmSlotFallback(c.autoConfirm, waitSlots); fallbackErr == nil {
								c.failoverStream.SetSlotFallbackRequired(true)
								c.failoverStream.SetFallbackWaitSlots(waitSlots)
								tipErr = nil
								tip = 0
							}
						}
					}
				} else {
					tip = 0
				}
			}
		}
		if tipErr != nil {
			if sourceInfo.IsNativeFiredancer && !c.failoverStream.GetIsDryRunFailover() {
				// The source must establish the finalized on-chain barrier before
				// the destination receives evidence and begins reconciliation.
				abortDestinationHandoff("native Firedancer watermark was not observed at finalized commitment", tipErr)
			}
			recoverBeforeDestinationActivation("failed to determine frozen tower vote", tipErr)
			return
		}
		if sourceInfo.IsNativeFiredancer && !c.failoverStream.GetIsDryRunFailover() && tip > 0 {
			reconciliationStarted := time.Now()
			c.logger.Info("waiting for native Firedancer tower watermark to reach finalized on-chain vote", "vote_account", sourceInfo.VoteAccount, "frozen_slot", tip, "commitment", rpc.CommitmentFinalized, "timeout", handoffTimeout(c.handoffTimeout), "poll_interval", c.handoffPollInterval)
			ctx, cancel := context.WithTimeout(c.ctx, handoffTimeout(c.handoffTimeout))
			lastProgress := time.Now()
			finalizedErr := waitForFinalizedVoteAccount(ctx, c.solanaRPCClient, sourceInfo.VoteAccount, sourceInfo.Identities.Active.PubKey(), tip, c.handoffPollInterval, func(lastVote uint64, err error) {
				if time.Since(lastProgress) < 5*time.Second {
					return
				}
				c.logger.Info("still waiting for native Firedancer tower watermark to finalize on-chain", "frozen_slot", tip, "network_last_vote", lastVote, "slot_gap", maxSlotGap(tip, lastVote), "elapsed", time.Since(reconciliationStarted).Round(time.Second), "err", err)
				lastProgress = time.Now()
			})
			cancel()
			if finalizedErr != nil {
				abortDestinationHandoff("native Firedancer tower watermark did not reach finalized commitment", finalizedErr)
				recoverBeforeDestinationActivation("failed to confirm native Firedancer tower watermark on-chain", finalizedErr)
				return
			}
			c.logger.Info("native Firedancer tower watermark reached finalized on-chain vote", "frozen_slot", tip, "elapsed", time.Since(reconciliationStarted).Round(time.Millisecond))
		}
		c.failoverStream.SetFrozenTowerSlot(tip)
		c.failoverStream.SetHandoffEvidenceEndTime()
		if err := c.failoverStream.Encode(); err != nil {
			// A write error is ambiguous: the peer may have received and acted on
			// the complete frame even though this side observed a transport error.
			// Never reactivate the source automatically in this state.
			c.logger.Error("failed to send on-chain handoff evidence", "err", err)
			c.setFailure("failed to send on-chain handoff evidence", err)
			c.logger.Error("CRITICAL: evidence delivery status is unknown after this node switched to passive; destination may be reconciling or active — check gossip and intervene manually")
			if c.rollback.ToActive.ResolvedCmd != "" {
				c.logger.Errorf("if this node needs to revert to active after confirming the destination is passive: %s", c.rollback.ToActive.ResolvedCmd)
			}
			return
		}
		if err := c.failoverStream.Decode(); err != nil {
			c.logger.Error("failed waiting for on-chain reconciliation", "err", err)
			c.setFailure("failed waiting for on-chain reconciliation", err)
			if wentPassive {
				c.logger.Error("CRITICAL: connection lost or protocol failed after this node switched to passive; destination state is unknown — check gossip and intervene manually")
			}
			return
		}
		if !c.failoverStream.GetReconciliationComplete() {
			reason := c.failoverStream.GetErrorMessage()
			if reason == "" {
				reason = "peer rejected on-chain reconciliation without a reason"
			}
			c.logger.Error("on-chain reconciliation rejected", "err", reason)
			c.setFailure("on-chain reconciliation rejected", fmt.Errorf("%s", reason))
			if wentPassive {
				if c.rollback.Enabled {
					c.logger.Warn("reconciliation was rejected before destination activation; reverting this node to active")
					if rbErr := RunRollbackToActive(c.rollback, c.getHookEnvMap(hookEnvMapParams{
						isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
						isPostFailover:   true,
					}), c.failoverStream.GetIsDryRunFailover(), c.logger); rbErr != nil {
						c.logger.Error("rollback to active failed — manual intervention required", "err", rbErr)
					}
				} else {
					c.logger.Error("CRITICAL: this node remains passive and destination activation did not begin — manual intervention required")
					if c.rollback.ToActive.ResolvedCmd != "" {
						c.logger.Errorf("to recover this node: %s", c.rollback.ToActive.ResolvedCmd)
					}
				}
			}
			return
		}
	} else {
		c.logger.Infof("sending tower file to %s", style.RenderPassiveString(c.failoverStream.GetPassiveNodeInfo().Hostname, false))

		// Read the tower file into TowerFileBytes
		c.failoverStream.SetActiveNodeSyncTowerFileStartTime()
		err = c.failoverStream.GetActiveNodeInfo().SetTowerFileBytes()
		if err != nil {
			c.logger.Error(fmt.Sprintf("failed to set tower file bytes for %s", c.failoverStream.GetActiveNodeInfo().TowerFile), "err", err)
			c.setFailure("failed to read tower file", err)
			return
		}
		c.failoverStream.SetActiveNodeSyncTowerFileEndTime()

		// Send the updated node info with tower file bytes
		if err := c.failoverStream.Encode(); err != nil {
			c.logger.Error(fmt.Sprintf("failed to send tower file bytes for %s", c.failoverStream.GetActiveNodeInfo().TowerFile), "err", err)
			c.setFailure("failed to send tower file bytes", err)
			if wentPassive {
				c.logger.Error(
					"CRITICAL: tower sync failed after this node switched to passive — " +
						"the passive node has not changed identity; check gossip and intervene manually if needed",
				)
				if c.rollback.ToActive.ResolvedCmd != "" {
					c.logger.Errorf("if this node needs to revert to active: %s", c.rollback.ToActive.ResolvedCmd)
				}
			}
			return
		}
	}

	// wait for confirmation from server that failover is complete
	err = c.failoverStream.Decode()
	if err != nil {
		c.logger.Error("failed to decode failover stream", "err", err)
		c.setFailure("failed to decode failover completion", err)
		if wentPassive {
			// The connection dropped after this node switched to passive.
			// We cannot know whether the server successfully set its identity — do NOT
			// auto-rollback (risk of two active validators). Check gossip manually.
			c.logger.Error(
				"CRITICAL: connection lost after this node switched to passive — " +
					"check gossip to determine cluster state and intervene manually if needed",
			)
			if c.rollback.ToActive.ResolvedCmd != "" {
				c.logger.Errorf("if this node needs to revert to active: %s", c.rollback.ToActive.ResolvedCmd)
			}
		}
		return
	}

	// Check for explicit rollback signal from server
	if c.failoverStream.GetRollbackRequired() {
		c.logger.Error("server signalled rollback required — failover failed on the passive node")
		c.setFailure("server signalled rollback required", fmt.Errorf("failover failed on the destination node"))
		if c.rollback.Enabled && wentPassive {
			c.logger.Warn("rollback enabled: reverting this node to active")
			if rbErr := RunRollbackToActive(c.rollback, c.getHookEnvMap(hookEnvMapParams{
				isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
				isPostFailover:   true,
			}), c.failoverStream.GetIsDryRunFailover(), c.logger); rbErr != nil {
				c.logger.Error("rollback to active failed — manual intervention required", "err", rbErr)
				if c.rollback.ToActive.ResolvedCmd != "" {
					c.logger.Errorf("to recover this node: %s", c.rollback.ToActive.ResolvedCmd)
				}
			}
		} else {
			c.logger.Error("rollback disabled — this node is currently passive; manual intervention required")
			if c.rollback.ToActive.ResolvedCmd != "" {
				c.logger.Errorf("to revert this node to active: %s", c.rollback.ToActive.ResolvedCmd)
			}
		}
		return
	}

	if !c.failoverStream.GetIsSuccessfullyCompleted() {
		c.logger.Errorf("server failed to complete failover: %s", c.failoverStream.GetErrorMessage())
		c.setFailure("server failed to complete failover", fmt.Errorf("%s", c.failoverStream.GetErrorMessage()))
		return
	}

	c.logger.Info("failover complete")
	completed = true

	// run post hooks now this is passive and active node says all is peachy
	c.hooks.RunPostWhenPassive(c.getHookEnvMap(hookEnvMapParams{
		isDryRunFailover: c.failoverStream.GetIsDryRunFailover(),
		isPostFailover:   true,
	}))

	return nil
}

// waitUntilStartOfNextSlot waits until the start of the next slot
// this is important to try to start a failover early in the slot to avoid missing it
// It polls getSlot() to detect when the slot changes and returns the new slot number,
// which naturally gets us well within the first few milliseconds of the new slot
// should get us in within the first 10ms of the next slot on average
func (c *Client) waitUntilStartOfNextSlot() (newSlot uint64, err error) {
	c.logger.Debug("waiting until start of next slot")

	// Get the current slot number
	currentSlot, err := c.solanaRPCClient.GetCurrentSlot()
	if err != nil {
		return 0, fmt.Errorf("failed to get current slot: %w", err)
	}

	// Poll getSlot() to detect when the slot changes.
	// getSlot() is a lightweight local RPC call so 10ms polling is cheap and gives
	// ~5ms average detection lag vs ~25ms at the previous 50ms interval.
	// On RPC error use a longer back-off to avoid hammering a struggling local node.
	const (
		pollInterval       = 10 * time.Millisecond
		errorRetryInterval = 50 * time.Millisecond
	)
	for {
		slot, err := c.solanaRPCClient.GetCurrentSlot()
		if err != nil {
			c.logger.Debug("failed to get slot, retrying", "err", err)
			time.Sleep(errorRetryInterval)
			continue
		}

		// Slot has changed, we're now in the next slot
		if slot > currentSlot {
			c.logger.Debug("slot transition detected, proceeding", "old_slot", currentSlot, "new_slot", slot)
			return slot, nil
		}

		// Still in the same slot, continue polling
		time.Sleep(pollInterval)
	}
}

// waitMinTimeToLeaderSlot waits until the next leader slot is at least the minimum time to leader slot
func (c *Client) waitMinTimeToLeaderSlot() (err error) {
	pubkey, err := solanago.PublicKeyFromBase58(c.activeNodeInfo.Identities.Active.PubKey())
	if err != nil {
		return fmt.Errorf("failed to parse active identity pubkey: %w", err)
	}

	if !c.waitMinTimeToLeaderSlotEnabled {
		c.logger.Debug("min time to leader slot check disabled, skipping wait")
		isOnSchedule, timeToNext, queryErr := c.solanaRPCClient.GetTimeToNextLeaderSlotForPubkey(pubkey)
		if queryErr != nil {
			c.logger.Warn("could not query next leader slot", "err", queryErr)
		} else if !isOnSchedule {
			c.logger.Info("not on leader schedule")
		} else {
			c.logger.Infof("next leader slot in %s", timeToNext.Round(time.Second))
		}
		return nil
	}

	c.logger.Debugf("ensuring next leader slot is at least %s in the future", c.minTimeToLeaderSlot.String())
	sp := spinner.New().TitleStyle(style.SpinnerTitleStyle).Title(style.RenderPinkString("checking next leader slot..."))
	maxRetries := 10
	var calculatedTimeToNextLeaderSlot time.Duration
	var isOnLeaderSchedule bool
	sp.ActionWithErr(func(ctx context.Context) error {
		sleepDuration := 2 * time.Second
		remainingRetries := maxRetries
		stringMinTimeToLeaderSlot := c.minTimeToLeaderSlot.Round(time.Second).String()

		for {
			onSchedule, timeToNextLeaderSlot, err := c.solanaRPCClient.GetTimeToNextLeaderSlotForPubkey(pubkey)
			if err != nil {
				if remainingRetries == 0 {
					return fmt.Errorf("failed to get time to next leader slot: %w", err)
				}
				log.Debug("failed to get time to next leader slot", "err", err)
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

			if !onSchedule {
				sp.Title(style.RenderPinkString("not on leader schedule, skipping wait"))
				return nil
			}

			isOnLeaderSchedule = true
			stringTimeToNextLeaderSlot := timeToNextLeaderSlot.Round(time.Second).String()

			if timeToNextLeaderSlot < c.minTimeToLeaderSlot {
				sp.Title(style.RenderPinkString(fmt.Sprintf("next leader slot in %s, waiting for it before proceeding...", stringTimeToNextLeaderSlot)))
				time.Sleep(sleepDuration)
				continue
			}

			calculatedTimeToNextLeaderSlot = timeToNextLeaderSlot
			sp.Title(style.RenderPinkString(fmt.Sprintf("next leader slot in %s > %s, proceeding...", stringTimeToNextLeaderSlot, stringMinTimeToLeaderSlot)))
			return nil
		}
	})

	err = sp.Run()
	if err != nil {
		return fmt.Errorf("failed to wait for next leader slot: %w", err)
	}

	if !isOnLeaderSchedule {
		c.logger.Info("not on leader schedule")
	} else {
		c.logger.Infof("next leader slot in %s", calculatedTimeToNextLeaderSlot.Round(time.Second))
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
	envMap["THIS_NODE_CLIENT_VERSION_LOCAL_RPC"] = c.activeNodeInfo.ClientVersionRPC
	envMap["THIS_NODE_RPC_ADDRESS"] = c.rpcURL

	// peer node
	envMap["PEER_NODE_NAME"] = c.failoverStream.GetPassiveNodeInfo().Hostname
	envMap["PEER_NODE_PUBLIC_IP"] = c.failoverStream.GetPassiveNodeInfo().PublicIP
	envMap["PEER_NODE_ACTIVE_IDENTITY_PUBKEY"] = c.failoverStream.GetPassiveNodeInfo().Identities.Active.PubKey()
	envMap["PEER_NODE_PASSIVE_IDENTITY_PUBKEY"] = c.failoverStream.GetPassiveNodeInfo().Identities.Passive.PubKey()
	envMap["PEER_NODE_CLIENT_VERSION"] = c.failoverStream.GetPassiveNodeInfo().ClientVersion
	envMap["PEER_NODE_CLIENT_VERSION_LOCAL_RPC"] = c.failoverStream.GetPassiveNodeInfo().ClientVersionRPC
	AddHandoffTemplateEnv(envMap, *c.activeNodeInfo, *c.failoverStream.GetPassiveNodeInfo(), *c.activeNodeInfo, *c.failoverStream.GetPassiveNodeInfo(), c.failoverStream.GetHandoffStrategy(), c.failoverStream.GetTowerFileWillBeTransferred())

	return envMap
}

// connectToServer waits until a QUIC server is listening on the given address
// It shows a spinner and attempts the actual QUIC connection, retrying on error until successful
// This allows the client to start independet of the server being ready to accept connections and latches
// onto the server as soon as it is ready
func (c *Client) connectToServer() error {
	sp := spinner.New().Title(style.RenderPinkString("waiting for ") +
		style.RenderPassiveString(c.serverName, false) +
		style.RenderPinkString(" at ") +
		style.RenderGreyString(c.serverAddress, false) +
		style.RenderPinkString("..."))
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
			} else if isALPNMismatch(err) {
				return fmt.Errorf("passive node rejected connection: incompatible wire protocol version — ensure both nodes run the same version of solana-validator-failover: %w", err)
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
				} else if isALPNMismatch(err) {
					return fmt.Errorf("passive node rejected connection: incompatible wire protocol version — ensure both nodes run the same version of solana-validator-failover: %w", err)
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
		c.logger.Debug("failed to resolve server address", "err", err, "address", c.serverAddress)
		return err
	}

	wrapped, err := newBasicPacketConn(":0")
	if err != nil {
		c.logger.Debug("failed to create UDP socket", "err", err)
		return err
	}

	tr := &quic.Transport{Conn: wrapped}

	quicTLSConfig := c.tlsConfig
	if quicTLSConfig == nil {
		quicTLSConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // intentional fallback when mTLS is not configured
			NextProtos:         []string{ProtocolName},
		}
	}

	conn, err := tr.Dial(c.ctx, udpAddr, quicTLSConfig, nil)
	if err != nil {
		tr.Close()
		if isALPNMismatch(err) {
			return err
		}
		c.logger.Debug("QUIC server not ready, retrying...", "err", err, "address", c.serverAddress)
		return err
	}

	if c.tlsConfig != nil {
		tlsState := conn.ConnectionState().TLS
		if len(tlsState.PeerCertificates) > 0 {
			peer := tlsState.PeerCertificates[0]
			c.logger.Info("mTLS: server certificate verified",
				"remote_addr", conn.RemoteAddr().String(),
				"subject", peer.Subject.String(),
				"issuer", peer.Issuer.String(),
				"expires", peer.NotAfter,
			)
		}
	}

	c.Conn = conn
	return nil
}
