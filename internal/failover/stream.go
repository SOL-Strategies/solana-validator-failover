package failover

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/solana"
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
		message: Message{
			CreditSamples: make(CreditSamples),
		},
	}
}

// Encode encodes the FailoverStream into the stream
func (s *Stream) Encode() error {
	err := s.encoder.Encode(s.message)
	if err != nil {
		log.Err(err).Msg("failed to encode failover message")
		return err
	}
	return nil
}

// Decode decodes the FailoverStream from the stream
func (s *Stream) Decode() error {
	err := s.decoder.Decode(&s.message)
	if err != nil {
		log.Err(err).Msg("failed to decode failover message")
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
	log.Error().Msgf(format, a...)
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

// SetSkipTowerSync sets the skip tower sync flag
func (s *Stream) SetSkipTowerSync(skipTowerSync bool) {
	s.message.SkipTowerSync = skipTowerSync
}

// GetSkipTowerSync returns the skip tower sync flag
func (s Stream) GetSkipTowerSync() bool {
	return s.message.SkipTowerSync
}

// SetIsSuccessfullyCompleted sets the is successfully completed
func (s *Stream) SetIsSuccessfullyCompleted(isSuccessfullyCompleted bool) {
	s.message.IsSuccessfullyCompleted = isSuccessfullyCompleted
}

// GetIsSuccessfullyCompleted returns the is successfully completed
func (s Stream) GetIsSuccessfullyCompleted() bool {
	return s.message.IsSuccessfullyCompleted
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
	data.ThisNodeRPCAddress = rpcURL

	// Peer node (passive)
	data.PeerNodeName = s.message.PassiveNodeInfo.Hostname
	data.PeerNodePublicIP = s.message.PassiveNodeInfo.PublicIP
	data.PeerNodeActiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Active.PubKey()
	data.PeerNodePassiveIdentityPubkey = s.message.PassiveNodeInfo.Identities.Passive.PubKey()
	data.PeerNodeClientVersion = s.message.PassiveNodeInfo.ClientVersion

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
	data.ThisNodeRPCAddress = rpcURL

	// Peer node (active)
	data.PeerNodeName = s.message.ActiveNodeInfo.Hostname
	data.PeerNodePublicIP = s.message.ActiveNodeInfo.PublicIP
	data.PeerNodeActiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Active.PubKey()
	data.PeerNodePassiveIdentityPubkey = s.message.ActiveNodeInfo.Identities.Passive.PubKey()
	data.PeerNodeClientVersion = s.message.ActiveNodeInfo.ClientVersion

	return data
}

// ConfirmFailover is called by the passive node to proceed with the failover
// it shows confirmation message and waits for user to confirm. once confirmed
// it allows the stream to proceed and the active node begins setting identity
// and tower file sync
func (s *Stream) ConfirmFailover(failoverHooks hooks.FailoverHooks, activeRPCURL, passiveRPCURL string, autoConfirm bool) (err error) {
	data := PlanData{
		IsDryRun:            s.message.IsDryRunFailover,
		SkipTowerSync:       s.message.SkipTowerSync,
		ActiveNodeInfo:      s.message.ActiveNodeInfo,
		PassiveNodeInfo:     s.message.PassiveNodeInfo,
		AppVersion:          pkgconstants.AppVersion,
		Hooks:               failoverHooks,
		ActivePreHookData:   s.buildHookTemplateDataForActiveNode(true, activeRPCURL),
		ActivePostHookData:  s.buildHookTemplateDataForActiveNode(false, activeRPCURL),
		PassivePreHookData:  s.buildHookTemplateDataForPassiveNode(true, passiveRPCURL),
		PassivePostHookData: s.buildHookTemplateDataForPassiveNode(false, passiveRPCURL),
	}

	rendered, err := RenderFailoverPlan(data)
	if err != nil {
		return err
	}

	fmt.Print(style.RenderMessageString(strings.Trim(rendered, "\n")))

	if autoConfirm {
		log.Warn().Msg("--yes flag set, automatically proceeding with failover")
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
	return SummaryData{
		IsDryRun:      s.message.IsDryRunFailover,
		SkipTowerSync: s.message.SkipTowerSync,

		OrigActiveNode:  s.message.ActiveNodeInfo,
		OrigPassiveNode: s.message.PassiveNodeInfo,

		OrigActiveSetIdentityDuration:  s.message.ActiveNodeSetIdentityEndTime.Sub(s.message.ActiveNodeSetIdentityStartTime),
		TowerSyncDuration:              s.message.PassiveNodeSyncTowerFileEndTime.Sub(s.message.ActiveNodeSyncTowerFileStartTime),
		TowerFileSizeBytes:             int64(len(s.message.ActiveNodeInfo.TowerFileBytes)),
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

// SetActiveNodeSyncTowerFileStartTime sets the active node sync tower file start time
func (s *Stream) SetActiveNodeSyncTowerFileStartTime() {
	s.message.ActiveNodeSyncTowerFileStartTime = time.Now()
}

// SetActiveNodeSyncTowerFileEndTime sets the active node sync tower file end time
func (s *Stream) SetActiveNodeSyncTowerFileEndTime() {
	s.message.ActiveNodeSyncTowerFileEndTime = time.Now()
}

// SetPassiveNodeSetIdentityStartTime sets the passive node set identity start time
func (s *Stream) SetPassiveNodeSetIdentityStartTime() {
	s.message.PassiveNodeSetIdentityStartTime = time.Now()
}

// SetPassiveNodeSetIdentityEndTime sets the passive node set identity end time
func (s *Stream) SetPassiveNodeSetIdentityEndTime() {
	s.message.PassiveNodeSetIdentityEndTime = time.Now()
}

// SetPassiveNodeSyncTowerFileEndTime sets the passive node sync tower file end time
func (s *Stream) SetPassiveNodeSyncTowerFileEndTime() {
	s.message.PassiveNodeSyncTowerFileEndTime = time.Now()
}

// PullActiveIdentityVoteCreditsSample pulls a sample of the vote credits for the active identity
func (s *Stream) PullActiveIdentityVoteCreditsSample(solanaRPCClient solana.ClientInterface) (err error) {
	identityPubkey := s.message.ActiveNodeInfo.Identities.Active.PubKey()

	// fetch current state of vote account from its pubkey
	voteAccount, creditRank, err := solanaRPCClient.GetCreditRankedVoteAccountFromPubkey(identityPubkey)
	if err != nil {
		return fmt.Errorf("failed to get vote accounts: %w", err)
	}

	// initialize the credit samples for the identity if it doesn't exist
	if _, ok := s.message.CreditSamples[identityPubkey]; !ok {
		s.message.CreditSamples[identityPubkey] = make([]CreditsSample, 0)
	}

	// take sample
	sample := &CreditsSample{
		Timestamp: time.Now(),
		VoteRank:  creditRank,
	}

	// find compute credits
	if len(voteAccount.EpochCredits) > 0 {
		// Calculate credits as the difference between current and previous epoch credits
		lastIndex := len(voteAccount.EpochCredits) - 1
		currentCredits := voteAccount.EpochCredits[lastIndex][1]
		previousCredits := int64(0)
		if lastIndex > 0 {
			previousCredits = voteAccount.EpochCredits[lastIndex-1][1]
		}
		sample.Credits = int(currentCredits - previousCredits)
	}

	// append sample to the identity's credit samples
	s.message.CreditSamples[identityPubkey] = append(
		s.message.CreditSamples[identityPubkey],
		*sample,
	)

	return nil
}

// PullActiveIdentityVoteCreditsSamples pulls a sample of the vote credits for the active identity
func (s *Stream) PullActiveIdentityVoteCreditsSamples(solanaRPCClient solana.ClientInterface, nSamples int, interval time.Duration) (err error) {
	if nSamples == 0 {
		return nil
	}
	if nSamples == 1 {
		return s.PullActiveIdentityVoteCreditsSample(solanaRPCClient)
	}

	// multiple samples may take some time so show a spinner to keep you patient
	var sp *spinner.Spinner
	sp = spinner.New().Title(fmt.Sprintf("Pulling %d vote credit samples %s apart...", nSamples, interval))

	sampleCount := 0
	sp.ActionWithErr(func(ctx context.Context) error {
		for range make([]struct{}, nSamples) {
			sampleCount++
			sp.Title(fmt.Sprintf("Pulling vote credit sample %d of %d...", sampleCount, nSamples))
			err := s.PullActiveIdentityVoteCreditsSample(solanaRPCClient)
			if err != nil {
				sp.Title(fmt.Sprintf("Failed to pull vote credits sample: %s", err))
				continue
			}
			sample := s.message.CreditSamples[s.message.ActiveNodeInfo.Identities.Active.PubKey()][len(s.message.CreditSamples[s.message.ActiveNodeInfo.Identities.Active.PubKey()])-1]
			if len(s.message.CreditSamples[s.message.ActiveNodeInfo.Identities.Active.PubKey()]) > 2 {
				// check and warn if credits are not increasing between the last two samples
				previousSample := s.message.CreditSamples[s.message.ActiveNodeInfo.Identities.Active.PubKey()][len(s.message.CreditSamples[s.message.ActiveNodeInfo.Identities.Active.PubKey()])-2]
				if sample.Credits <= previousSample.Credits {
					sp.Title(style.RenderWarningStringf(
						"Vote credits are not increasing between samples %d and %d - this is not good",
						sampleCount-1,
						sampleCount,
					))
				}
			}
			time.Sleep(interval)
			sp.Title(fmt.Sprintf("Pulled vote credit sample %d of %d - credits: %d, rank: %d...", sampleCount, nSamples, sample.Credits, sample.VoteRank))
		}
		log.Debug().Msgf("Pulled %d vote credit samples", sampleCount)
		return nil
	})
	return sp.Run()
}

// GetVoteCreditRankDifference returns the difference in vote credit rank between the first and last sample
func (s *Stream) GetVoteCreditRankDifference() (difference, first, last int, err error) {
	pubkey := s.message.ActiveNodeInfo.Identities.Active.PubKey()
	samples := s.message.CreditSamples[pubkey]
	if len(samples) < 2 {
		return 0, 0, 0, fmt.Errorf("not enough vote credit samples to calculate difference")
	}
	first = samples[0].VoteRank
	last = samples[len(samples)-1].VoteRank
	difference = last - first
	// invert the difference (lower number is better)
	return -1 * difference, first, last, nil
}
