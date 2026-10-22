package failover

import (
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/quic-go/quic-go"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
	pkgconstants "github.com/sol-strategies/solana-validator-failover/pkg/constants"
)

// Stream is the message sent from the active node to the passive node (server) to initiate the failover process
type Stream struct {
	message Message
	Stream  *quic.Stream
	decoder *gob.Decoder
	encoder *gob.Encoder
}

// NewFailoverStream creates a new FailoverStream from a QUIC stream
func NewFailoverStream(stream *quic.Stream) *Stream {
	decoder := gob.NewDecoder(stream)
	encoder := gob.NewEncoder(stream)

	return &Stream{
		Stream:  stream,
		decoder: decoder,
		encoder: encoder,
	}
}

// Encode encodes the FailoverStream into the stream
func (s *Stream) Encode() error {
	err := s.encoder.Encode(s.message)
	if err != nil {
		log.Error("failed to encode failover message", "err", err)
		return err
	}
	return nil
}

// Decode decodes the FailoverStream from the stream
func (s *Stream) Decode() error {
	err := s.decoder.Decode(&s.message)
	if err != nil {
		log.Error("failed to decode failover message", "err", err)
		return err
	}
	return nil
}

// GetCanProceed returns whether the failover can proceed
func (s *Stream) GetCanProceed() bool {
	return s.message.CanProceed
}

// SetCanProceed sets whether the failover can proceed
func (s *Stream) SetCanProceed(canProceed bool) {
	s.message.CanProceed = canProceed
}

// GetErrorMessage returns the error message
func (s *Stream) GetErrorMessage() string {
	return s.message.ErrorMessage
}

// SetErrorMessage sets the error message
func (s *Stream) SetErrorMessage(errorMessage string) {
	s.message.ErrorMessage = errorMessage
}

// SetErrorMessagef sets the error message with a formatted string
func (s *Stream) SetErrorMessagef(format string, a ...any) {
	s.message.ErrorMessage = fmt.Sprintf(format, a...)
}

// LogErrorWithSetMessagef logs an error with a formatted string and sets the error message
func (s *Stream) LogErrorWithSetMessagef(format string, a ...any) {
	log.Errorf(format, a...)
	s.SetErrorMessagef(format, a...)
}

// SetPassiveNodeInfo sets the passive node info
func (s *Stream) SetPassiveNodeInfo(passiveNodeInfo *NodeInfo) {
	s.message.PassiveNodeInfo = *passiveNodeInfo
}

// GetPassiveNodeInfo returns the passive node info
func (s *Stream) GetPassiveNodeInfo() *NodeInfo {
	return &s.message.PassiveNodeInfo
}

// SetActiveNodeInfo sets the active node info
func (s *Stream) SetActiveNodeInfo(activeNodeInfo *NodeInfo) {
	s.message.ActiveNodeInfo = *activeNodeInfo
}

// GetActiveNodeInfo returns the active node info
func (s *Stream) GetActiveNodeInfo() *NodeInfo {
	return &s.message.ActiveNodeInfo
}

// SetIsDryRunFailover sets the is dry run failover
func (s *Stream) SetIsDryRunFailover(isDryRunFailover bool) {
	s.message.IsDryRunFailover = isDryRunFailover
}

// GetIsDryRunFailover returns the is dry run failover
func (s Stream) GetIsDryRunFailover() bool {
	return s.message.IsDryRunFailover
}

func (s *Stream) SetHandoffStrategy(strategy string) {
	s.message.HandoffStrategy = strategy
	s.refreshHistoryTransfer()
}

func (s *Stream) refreshHistoryTransfer() {
	s.message.VoteHistoryWillBeTransferred = s.message.HandoffStrategy == HandoffStrategyVoteHistory
}

func (s Stream) GetHandoffStrategy() string { return s.message.HandoffStrategy }

func (s Stream) GetVoteHistoryWillBeTransferred() bool { return s.message.VoteHistoryWillBeTransferred }

func (s *Stream) SetHandoffAborted(aborted bool) { s.message.HandoffAborted = aborted }

func (s Stream) GetHandoffAborted() bool { return s.message.HandoffAborted }

func (s *Stream) SetRollbackCommands(active, passive string) {
	s.message.ActiveRollbackCommand = active
	s.message.PassiveRollbackCommand = passive
}

func (s Stream) GetActiveRollbackCommand() string { return s.message.ActiveRollbackCommand }

func (s Stream) GetPassiveRollbackCommand() string { return s.message.PassiveRollbackCommand }

// SetIsSuccessfullyCompleted sets the is successfully completed
func (s *Stream) SetIsSuccessfullyCompleted(isSuccessfullyCompleted bool) {
	s.message.IsSuccessfullyCompleted = isSuccessfullyCompleted
}

// GetIsSuccessfullyCompleted returns the is successfully completed
func (s Stream) GetIsSuccessfullyCompleted() bool {
	return s.message.IsSuccessfullyCompleted
}

// SetRollbackRequired signals to the client that it must rollback its identity change.
func (s *Stream) SetRollbackRequired(required bool) {
	s.message.RollbackRequired = required
}

// GetRollbackRequired returns true if the server has signalled that a rollback is required.
func (s Stream) GetRollbackRequired() bool {
	return s.message.RollbackRequired
}

// SetActiveRollbackEnabled records whether the active node (client) has rollback configured.
// Called by the client before sending its initial message so the server can detect mismatches.
func (s *Stream) SetActiveRollbackEnabled(enabled bool) {
	s.message.ActiveRollbackEnabled = enabled
}

// GetActiveRollbackEnabled returns whether the active node (client) has rollback configured.
func (s Stream) GetActiveRollbackEnabled() bool {
	return s.message.ActiveRollbackEnabled
}

// SetFailoverStartSlot sets the failover start slot
func (s *Stream) SetFailoverStartSlot(failoverStartSlot uint64) {
	s.message.FailoverStartSlot = failoverStartSlot
}

// GetFailoverStartSlot returns the failover start slot
func (s Stream) GetFailoverStartSlot() uint64 {
	return s.message.FailoverStartSlot
}

// SetFailoverEndSlot sets the failover end slot
func (s *Stream) SetFailoverEndSlot(failoverEndSlot uint64) {
	s.message.FailoverEndSlot = failoverEndSlot
}

// GetFailoverEndSlot returns the failover end slot
func (s Stream) GetFailoverEndSlot() uint64 {
	return s.message.FailoverEndSlot
}

// buildHookTemplateDataForActiveNode builds HookTemplateData for the active node (client) from Stream data
func (s *Stream) buildHookTemplateDataForActiveNode(isPreFailover bool, rpcURL string) hooks.HookTemplateData {
	data := hooks.HookTemplateData{
		IsDryRunFailover: s.message.IsDryRunFailover,
	}

	if isPreFailover {
		data.ThisNodeRole = "active"
		data.PeerNodeRole = "passive"
	} else {
		data.ThisNodeRole = "passive"
		data.PeerNodeRole = "active"
	}

	// This node (active)
	data.ThisNodeName = s.message.ActiveNodeInfo.Hostname
	data.ThisNodePublicIP = s.message.ActiveNodeInfo.PublicIP
	data.ThisNodeActiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Active.PubKey()
	data.ThisNodeActiveIdentityKeyFile = s.message.ActiveNodeInfo.Identities.Active.KeyFile
	data.ThisNodePassiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Passive.PubKey()
	data.ThisNodePassiveIdentityKeyFile = s.message.ActiveNodeInfo.Identities.Passive.KeyFile
	data.ThisNodeClientVersion = s.message.ActiveNodeInfo.ClientVersion
	data.ThisNodeClientVersionLocalRPC = s.message.ActiveNodeInfo.ClientVersionRPC
	data.ThisNodeRPCAddress = rpcURL
	data.ThisNodeClientFamily = s.message.ActiveNodeInfo.ClientFamily
	data.ThisNodeIsNativeFiredancer = s.message.ActiveNodeInfo.IsNativeFiredancer

	// Peer node (passive)
	data.PeerNodeName = s.message.PassiveNodeInfo.Hostname
	data.PeerNodePublicIP = s.message.PassiveNodeInfo.PublicIP
	data.PeerNodeActiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Active.PubKey()
	data.PeerNodePassiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Passive.PubKey()
	data.PeerNodeClientVersion = s.message.PassiveNodeInfo.ClientVersion
	data.PeerNodeClientVersionLocalRPC = s.message.PassiveNodeInfo.ClientVersionRPC
	data.PeerNodeClientFamily = s.message.PassiveNodeInfo.ClientFamily
	data.PeerNodeIsNativeFiredancer = s.message.PassiveNodeInfo.IsNativeFiredancer
	data.FromNodeClientFamily = s.message.ActiveNodeInfo.ClientFamily
	data.ToNodeClientFamily = s.message.PassiveNodeInfo.ClientFamily
	data.FromNodeIsNativeFiredancer = s.message.ActiveNodeInfo.IsNativeFiredancer
	data.ToNodeIsNativeFiredancer = s.message.PassiveNodeInfo.IsNativeFiredancer
	data.FromNodeIsAgaveDerived = !s.message.ActiveNodeInfo.IsNativeFiredancer
	data.ToNodeIsAgaveDerived = !s.message.PassiveNodeInfo.IsNativeFiredancer
	data.HandoffStrategy = s.message.HandoffStrategy
	data.VoteHistoryWillBeTransferred = s.message.VoteHistoryWillBeTransferred
	data.VoteHistoryFile = s.message.ActiveNodeInfo.VoteHistoryFile
	data.VoteHistoryImportFile = s.message.ActiveNodeInfo.VoteHistoryImportFile

	return data
}

// buildHookTemplateDataForPassiveNode builds HookTemplateData for the passive node (server) from Stream data
func (s *Stream) buildHookTemplateDataForPassiveNode(isPreFailover bool, rpcURL string) hooks.HookTemplateData {
	data := hooks.HookTemplateData{
		IsDryRunFailover: s.message.IsDryRunFailover,
	}

	if isPreFailover {
		data.ThisNodeRole = "passive"
		data.PeerNodeRole = "active"
	} else {
		data.ThisNodeRole = "active"
		data.PeerNodeRole = "passive"
	}

	// This node (passive)
	data.ThisNodeName = s.message.PassiveNodeInfo.Hostname
	data.ThisNodePublicIP = s.message.PassiveNodeInfo.PublicIP
	data.ThisNodeActiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Active.PubKey()
	data.ThisNodeActiveIdentityKeyFile = s.message.PassiveNodeInfo.Identities.Active.KeyFile
	data.ThisNodePassiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Passive.PubKey()
	data.ThisNodePassiveIdentityKeyFile = s.message.PassiveNodeInfo.Identities.Passive.KeyFile
	data.ThisNodeClientVersion = s.message.PassiveNodeInfo.ClientVersion
	data.ThisNodeClientVersionLocalRPC = s.message.PassiveNodeInfo.ClientVersionRPC
	data.ThisNodeRPCAddress = rpcURL
	data.ThisNodeClientFamily = s.message.PassiveNodeInfo.ClientFamily
	data.ThisNodeIsNativeFiredancer = s.message.PassiveNodeInfo.IsNativeFiredancer

	// Peer node (active)
	data.PeerNodeName = s.message.ActiveNodeInfo.Hostname
	data.PeerNodePublicIP = s.message.ActiveNodeInfo.PublicIP
	data.PeerNodeActiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Active.PubKey()
	data.PeerNodePassiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Passive.PubKey()
	data.PeerNodeClientVersion = s.message.ActiveNodeInfo.ClientVersion
	data.PeerNodeClientVersionLocalRPC = s.message.ActiveNodeInfo.ClientVersionRPC
	data.PeerNodeClientFamily = s.message.ActiveNodeInfo.ClientFamily
	data.PeerNodeIsNativeFiredancer = s.message.ActiveNodeInfo.IsNativeFiredancer
	data.FromNodeClientFamily = s.message.ActiveNodeInfo.ClientFamily
	data.ToNodeClientFamily = s.message.PassiveNodeInfo.ClientFamily
	data.FromNodeIsNativeFiredancer = s.message.ActiveNodeInfo.IsNativeFiredancer
	data.ToNodeIsNativeFiredancer = s.message.PassiveNodeInfo.IsNativeFiredancer
	data.FromNodeIsAgaveDerived = !s.message.ActiveNodeInfo.IsNativeFiredancer
	data.ToNodeIsAgaveDerived = !s.message.PassiveNodeInfo.IsNativeFiredancer
	data.HandoffStrategy = s.message.HandoffStrategy
	data.VoteHistoryWillBeTransferred = s.message.VoteHistoryWillBeTransferred
	data.VoteHistoryFile = s.message.PassiveNodeInfo.VoteHistoryFile
	data.VoteHistoryImportFile = s.message.PassiveNodeInfo.VoteHistoryImportFile

	return data
}

// ConfirmFailover is called by the passive node to proceed with the failover
// it shows confirmation message and waits for user to confirm. once confirmed
// it allows the stream to proceed and the active node begins setting identity
// and vote-history transfer
func (s *Stream) ConfirmFailover(failoverHooks hooks.FailoverHooks, rollback hooks.RollbackConfig, activeRPCURL, passiveRPCURL string, autoConfirm bool) (err error) {
	data := PlanData{
		IsDryRun:                s.message.IsDryRunFailover,
		HandoffStrategy:         s.message.HandoffStrategy,
		ActiveNodeInfo:          s.message.ActiveNodeInfo,
		PassiveNodeInfo:         s.message.PassiveNodeInfo,
		AppVersion:              pkgconstants.AppVersion,
		VotingMonitoringEnabled: s.message.VotingMonitoringEnabled,
		Hooks:                   failoverHooks,
		Rollback:                rollback,
		ActivePreHookData:       s.buildHookTemplateDataForActiveNode(true, activeRPCURL),
		ActivePostHookData:      s.buildHookTemplateDataForActiveNode(false, activeRPCURL),
		PassivePreHookData:      s.buildHookTemplateDataForPassiveNode(true, passiveRPCURL),
		PassivePostHookData:     s.buildHookTemplateDataForPassiveNode(false, passiveRPCURL),
	}

	rendered, err := RenderFailoverPlan(data)
	if err != nil {
		return err
	}

	fmt.Print(style.RenderMessageString(strings.TrimLeft(rendered, "\n")))
	fmt.Println()

	if autoConfirm {
		log.Warn("--yes flag set, automatically proceeding with failover")
		return nil
	}

	var confirmFailover bool
	// ask to proceed
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Proceed with failover?").
				Value(&confirmFailover),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("server cancelled failover: %w", err)
	}

	if !confirmFailover {
		return fmt.Errorf("server cancelled failover")
	}

	return nil
}

// GetFailoverDuration returns the failover duration
func (s *Stream) GetFailoverDuration() time.Duration {
	return s.message.PassiveNodeSetIdentityEndTime.Sub(s.message.ActiveNodeSetIdentityStartTime)
}

// GetFailoverSlotsDuration returns the failover slots duration
func (s *Stream) GetFailoverSlotsDuration() uint64 {
	return s.GetFailoverEndSlot() - s.GetFailoverStartSlot()
}

// BuildSummaryData builds a SummaryData from the current stream message state.
// Call this after the failover is complete and all timing fields are set.
func (s *Stream) BuildSummaryData() SummaryData {
	stateFileSize := s.message.ActiveNodeInfo.VoteHistoryFileSizeBytes
	return SummaryData{
		IsDryRun:              s.message.IsDryRunFailover,
		Voting:                s.message.Voting,
		SuccessfullyCompleted: s.message.IsSuccessfullyCompleted,

		OrigActiveNode:  s.message.ActiveNodeInfo,
		OrigPassiveNode: s.message.PassiveNodeInfo,

		OrigActiveSetIdentityDuration:  s.message.ActiveNodeSetIdentityEndTime.Sub(s.message.ActiveNodeSetIdentityStartTime),
		HistorySyncDuration:            s.message.PassiveNodeSyncVoteHistoryEndTime.Sub(s.message.ActiveNodeSyncVoteHistoryStartTime),
		VoteHistoryFileSizeBytes:       stateFileSize,
		OrigPassiveSetIdentityDuration: s.message.PassiveNodeSetIdentityEndTime.Sub(s.message.PassiveNodeSetIdentityStartTime),
		TotalDuration:                  s.GetFailoverDuration(),

		FailoverStartSlot: s.message.FailoverStartSlot,
		FailoverEndSlot:   s.message.FailoverEndSlot,
		SlotsDuration:     s.GetFailoverSlotsDuration(),
	}
}

// SetActiveNodeSetIdentityStartTime sets the active node set identity start time
func (s *Stream) SetActiveNodeSetIdentityStartTime() {
	s.message.ActiveNodeSetIdentityStartTime = time.Now()
}

// SetActiveNodeSetIdentityEndTime sets the active node set identity end time
func (s *Stream) SetActiveNodeSetIdentityEndTime() {
	s.message.ActiveNodeSetIdentityEndTime = time.Now()
}

// SetActiveNodeSyncVoteHistoryStartTime sets the active node transfer vote history start time
func (s *Stream) SetActiveNodeSyncVoteHistoryStartTime() {
	s.message.ActiveNodeSyncVoteHistoryStartTime = time.Now()
}

// SetActiveNodeSyncVoteHistoryEndTime sets the active node transfer vote history end time
func (s *Stream) SetActiveNodeSyncVoteHistoryEndTime() {
	s.message.ActiveNodeSyncVoteHistoryEndTime = time.Now()
}

// SetPassiveNodeSetIdentityStartTime sets the passive node set identity start time
func (s *Stream) SetPassiveNodeSetIdentityStartTime() {
	s.message.PassiveNodeSetIdentityStartTime = time.Now()
}

// SetPassiveNodeSetIdentityEndTime sets the passive node set identity end time
func (s *Stream) SetPassiveNodeSetIdentityEndTime() {
	s.message.PassiveNodeSetIdentityEndTime = time.Now()
}

// SetPassiveNodeSyncVoteHistoryEndTime sets the passive node transfer vote history end time
func (s *Stream) SetPassiveNodeSyncVoteHistoryEndTime() {
	s.message.PassiveNodeSyncVoteHistoryEndTime = time.Now()
}
