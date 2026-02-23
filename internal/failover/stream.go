package failover

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"maps"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/dustin/go-humanize"
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
	// Build template data for hooks
	activePreTemplateData := s.buildHookTemplateDataForActiveNode(true, activeRPCURL)
	activePostTemplateData := s.buildHookTemplateDataForActiveNode(false, activeRPCURL)
	passivePreTemplateData := s.buildHookTemplateDataForPassiveNode(true, passiveRPCURL)
	passivePostTemplateData := s.buildHookTemplateDataForPassiveNode(false, passiveRPCURL)

	// Add custom function to split commands and render hooks
	funcMap := template.FuncMap{
		"splitCommand": func(cmd string) string {
			// Split the command by spaces
			parts := strings.Fields(cmd)
			if len(parts) == 0 {
				return ""
			}
			// Join with newlines and proper indentation
			return parts[0] + " \\\n      " + strings.Join(parts[1:], " \\\n      ")
		},
		"RenderSetIdentityCommand": func(cmd string, isDryRun bool) string {
			prefixSartString := hooks.PrefixStyle.Render("[set-identity")
			dryRunString := ""
			if isDryRun {
				dryRunString = style.RenderLightBlueString(" (dry run)")
			}
			prefixEndString := hooks.PrefixStyle.Render("]:")

			return fmt.Sprintf("%s%s%s %s", prefixSartString, dryRunString, prefixEndString, hooks.PrefixStyle.Render(cmd))
		},
		"RenderTowerFileSyncCommand": func(cmd string) string {
			prefixSartString := hooks.PrefixStyle.Render("[destination]:")
			return fmt.Sprintf("%s %s", prefixSartString, hooks.PrefixStyle.Render(cmd))
		},
		"RenderHooks": func(hooksList hooks.Hooks, nodeName string, hookType string) string {
			if len(hooksList) == 0 {
				return ""
			}
			var result strings.Builder
			var templateData hooks.HookTemplateData

			// Select appropriate template data based on node and hook type
			if nodeName == s.message.ActiveNodeInfo.Hostname {
				if hookType == "pre" {
					templateData = activePreTemplateData
				} else {
					templateData = activePostTemplateData
				}
			} else {
				if hookType == "pre" {
					templateData = passivePreTemplateData
				} else {
					templateData = passivePostTemplateData
				}
			}

			for i, hook := range hooksList {
				hookCmd, err := hooks.RenderHookCommand(hook, templateData)
				if err != nil {
					// If rendering fails, fall back to original command
					hookCmd = hook.Command
					if len(hook.Args) > 0 {
						hookCmd += " " + strings.Join(hook.Args, " ")
					}
				}

				hookIndex := i + 1
				prefixStartString := fmt.Sprintf("[%d/%d %s", hookIndex, len(hooksList), hook.Name)
				mustSucceedString := ""
				if hook.MustSucceed {
					mustSucceedString = " (must succeed)"
				}
				prefixEndString := "]:"
				styledHookString := fmt.Sprintf("%s%s%s %s",
					hooks.PrefixStyle.Render(prefixStartString),
					style.RenderLightWarningString(mustSucceedString),
					hooks.PrefixStyle.Render(prefixEndString),
					hooks.PrefixStyle.Render(hookCmd))

				result.WriteString(fmt.Sprintf(" ↓   %s", styledHookString))
				if i < len(hooksList)-1 {
					result.WriteString(" ↓\n")
				}
			}
			return result.String()
		},
	}

	// Merge with existing style functions
	maps.Copy(funcMap, style.TemplateFuncMap())

	tpl := template.New("confirmFailoverTpl").Funcs(funcMap)
	tpl, err = tpl.Parse(`{{ Purple "solana-validator-failover v" }}{{ Purple .AppVersion }}

{{ Purple "Current state:" }}

{{ .SummaryTable }}

{{ Purple "Failover plan:" }}

{{/* Clear warning when not a drill i.e not a dry run */}}
{{- if .IsDryRun -}}
{{ Blue "This is a dry run - re-run with '--not-a-drill' for a real failover," }}
{{- else -}}
{{ Warning "⚠️  This is a real failover - identities will be changed on both nodes." }}
{{- end }}
{{ if .Hooks.Pre.WhenActive }}
🪝  {{ .ActiveNodeInfo.Hostname }} → {{ Active "pre-passive" false }} hooks
{{ RenderHooks .Hooks.Pre.WhenActive .ActiveNodeInfo.Hostname "pre" }}
 ↓
{{- end }}
🔴 {{ Active .ActiveNodeInfo.Hostname false }} → {{ Passive "PASSIVE" false }} {{ Passive .ActiveNodeInfo.Identities.Passive.PubKey false }}
 ↓   {{ RenderSetIdentityCommand .ActiveNodeInfo.SetIdentityCommand .IsDryRun }} 
{{- if not .SkipTowerSync }}
 ↓
🔁 {{ LightGrey "tower-file" }} {{ LightGrey .ActiveNodeInfo.Hostname }} → {{ LightGrey .PassiveNodeInfo.Hostname }} 
 ↓   {{ RenderTowerFileSyncCommand .ActiveNodeInfo.TowerFile }}
{{- end }}
{{- if .Hooks.Pre.WhenPassive }}
 ↓
🪝  {{ .PassiveNodeInfo.Hostname }} → {{ Passive "pre-active" false }} hooks
{{ RenderHooks .Hooks.Pre.WhenPassive .PassiveNodeInfo.Hostname "pre" }}
 ↓
{{- else }}
{{- if not .SkipTowerSync }}
 ↓
{{- end }}
{{- end }}
🟢 {{ Passive .PassiveNodeInfo.Hostname false }} → {{ Active "ACTIVE" false }} {{ Active .PassiveNodeInfo.Identities.Active.PubKey false }} 
 ↓   {{ RenderSetIdentityCommand .PassiveNodeInfo.SetIdentityCommand .IsDryRun }} 
{{- if .Hooks.Post.WhenActive }}
 ↓
🪝  {{ .PassiveNodeInfo.Hostname }} → {{ Active "post-active" false }} hooks
{{ RenderHooks .Hooks.Post.WhenActive .PassiveNodeInfo.Hostname "post" }}
{{- end }}
{{- if .Hooks.Post.WhenPassive }}
 ↓
🪝  {{ .ActiveNodeInfo.Hostname }} → {{ Passive "post-passive" false }} hooks
{{ RenderHooks .Hooks.Post.WhenPassive .ActiveNodeInfo.Hostname "post" }}
{{- end }}
 ↓
💰 {{ LightGrey "Profit" }}
`)

	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{
		"IsDryRun":        s.message.IsDryRunFailover,
		"SkipTowerSync":   s.message.SkipTowerSync,
		"PassiveNodeInfo": s.message.PassiveNodeInfo,
		"ActiveNodeInfo":  s.message.ActiveNodeInfo,
		"SummaryTable":    s.message.currentStateTableString(),
		"AppVersion":      pkgconstants.AppVersion,
		"Hooks":           failoverHooks,
	}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// print confirm message
	fmt.Println(style.RenderMessageString(buf.String()))

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

// GetStateTable returns the state table
func (s *Stream) GetStateTable() string {
	return s.message.currentStateTableString()
}

// GetFailoverDurationTableString returns the failover duration table string
func (s *Stream) GetFailoverDurationTableString() string {
	rows := [][]string{
		{
			formatStageColumnRows(
				[]string{
					style.RenderActiveString(s.message.ActiveNodeInfo.Hostname, false),
					style.RenderGreyString("--set-identity-->", false),
					style.RenderPassiveString(s.message.ActiveNodeInfo.Identities.Passive.PubKey(), false),
				},
			)[0],
			s.message.ActiveNodeSetIdentityEndTime.Sub(s.message.ActiveNodeSetIdentityStartTime).String(),
			humanize.Comma(int64(s.GetFailoverStartSlot())),
		},
	}

	// Only add tower file sync row if not skipping
	if !s.message.SkipTowerSync {
		rows = append(rows, []string{
			formatStageColumnRows(
				[]string{
					style.RenderGreyString(s.message.ActiveNodeInfo.Hostname, false),
					style.RenderGreyString("---tower-file--->", false),
					style.RenderGreyString(s.message.PassiveNodeInfo.Hostname, false),
				},
			)[0],
			fmt.Sprintf("%s (%s)",
				s.message.PassiveNodeSyncTowerFileEndTime.Sub(s.message.ActiveNodeSyncTowerFileStartTime).String(),
				humanize.Bytes(uint64(len(s.message.ActiveNodeInfo.TowerFileBytes))),
			),
			" ",
		})
	}

	// Add passive node set identity row
	rows = append(rows, []string{
		formatStageColumnRows(
			[]string{
				style.RenderPassiveString(s.message.PassiveNodeInfo.Hostname, false),
				style.RenderGreyString("--set-identity-->", false),
				style.RenderActiveString(s.message.PassiveNodeInfo.Identities.Active.PubKey(), false),
			},
		)[0],
		s.message.PassiveNodeSetIdentityEndTime.Sub(s.message.PassiveNodeSetIdentityStartTime).String(),
		humanize.Comma(int64(s.GetFailoverEndSlot())),
	})

	// Add total row
	totalRowIndex := len(rows)
	rows = append(rows, []string{
		style.RenderBoldMessage("Total"),
		fmt.Sprintf("%s (wall clock)", style.RenderBoldMessage(s.GetFailoverDuration().String())),
		style.RenderBoldMessage(fmt.Sprintf("%s slots", humanize.Comma(int64(s.GetFailoverSlotsDuration())))),
	})

	return style.RenderTable(
		[]string{"Stage", "Duration", "Slot"},
		rows,
		func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return style.TableHeaderStyle
			}
			// total stage title
			if row == totalRowIndex && col == 0 {
				return style.TableCellStyle.Align(lipgloss.Right)
			}
			return style.TableCellStyle.Align(lipgloss.Left)
		},
	)
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

// formatStageColumnRows formats the stage column rows
// each row is a slice of strings representing 3 columns
// that must be padded to all have the same length
func formatStageColumnRows(rows ...[]string) (formattedRows []string) {
	maxColumnLengths := []int{0, 0, 0}
	formattedRows = make([]string, len(rows))

	// get the longest string length of each column
	for _, row := range rows {
		// for each row get the longest string length of each column
		if len(row[0]) > maxColumnLengths[0] {
			maxColumnLengths[0] = len(row[0])
		}
		if len(row[1]) > maxColumnLengths[1] {
			maxColumnLengths[1] = len(row[1])
		}
		if len(row[2]) > maxColumnLengths[2] {
			maxColumnLengths[2] = len(row[2])
		}
	}

	// pad each column to the longest string length
	for rowIndex, row := range rows {
		col1Value := row[0]
		col2Value := row[1]
		col3Value := row[2]

		col1Style := lipgloss.NewStyle().PaddingRight(maxColumnLengths[0] - len(col1Value))
		col2Style := lipgloss.NewStyle().PaddingRight(maxColumnLengths[1] - len(col2Value))
		col3Style := lipgloss.NewStyle().PaddingRight(maxColumnLengths[2] - len(col3Value))

		formattedRows[rowIndex] = fmt.Sprintf("%s %s %s",
			col1Style.Render(col1Value),
			col2Style.Render(col2Value),
			col3Style.Render(col3Value),
		)
	}

	return formattedRows
}
