package failover

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
)

// SummaryData holds all data needed to render the post-failover summary.
type SummaryData struct {
	IsDryRun bool

	// OrigActiveNode is the node that initiated the failover (was active, now passive).
	// OrigPassiveNode is the node that received the failover (was passive, now active).
	OrigActiveNode  NodeInfo
	OrigPassiveNode NodeInfo

	// Timing
	OrigActiveSetIdentityDuration  time.Duration
	HistorySyncDuration            time.Duration
	VoteHistoryFileSizeBytes       int64
	OrigPassiveSetIdentityDuration time.Duration
	TotalDuration                  time.Duration

	// Slots
	FailoverStartSlot uint64
	FailoverEndSlot   uint64
	SlotsDuration     uint64

	Voting                VotingResult
	SuccessfullyCompleted bool
}

// RenderFailoverSummary renders the post-failover summary to a string.
func RenderFailoverSummary(data SummaryData) (string, error) {
	funcMap := template.FuncMap{
		"stateLabel": stateLabel,
		"trimPrefix": strings.TrimPrefix,
		// truncPubkey uses ASCII "..." so byte-length == display-width.
		"truncPubkey": func(s string) string {
			if len(s) <= 16 {
				return s
			}
			return s[:8] + "..." + s[len(s)-4:]
		},
		// Bold renders s in bold (bright white on most terminals).
		"Bold": func(s string) string {
			return lipgloss.NewStyle().Bold(true).Render(s)
		},
		// HRule renders a solid grey horizontal rule.
		"HRule": func() string {
			return style.RenderGreyString(strings.Repeat("─", 80), false)
		},
		// FormatDuration rounds to ms and converts to string.
		"FormatDuration": func(d time.Duration) string {
			return d.Round(time.Millisecond).String()
		},
		// FormatBytes converts bytes to a human-readable size string.
		"FormatBytes": func(n int64) string {
			return humanize.Bytes(uint64(n))
		},
		// FormatSlot formats a slot number with comma separators.
		"FormatSlot": func(n uint64) string {
			return humanize.Comma(int64(n))
		},
		// SlotsSuffix returns ", same slot" when N == 0, otherwise ", over N slots".
		"SlotsSuffix": func(n uint64) string {
			if n == 0 {
				return ", same slot"
			}
			if n == 1 {
				return ", over 1 slot"
			}
			return fmt.Sprintf(", over %s slots", humanize.Comma(int64(n)))
		},
		// PadRight pads s with trailing spaces to the given width (ASCII labels only).
		"PadRight": func(s string, width int) string {
			if len(s) >= width {
				return s
			}
			return s + strings.Repeat(" ", width-len(s))
		},
	}
	maps.Copy(funcMap, style.TemplateFuncMap())

	// labelWidth is the display width of the widest section label so that the
	// timing info on each header line aligns vertically.
	labelWidth := max(len(data.OrigActiveNode.Hostname), len(data.OrigPassiveNode.Hostname))
	labelWidth = max(labelWidth, len("> "+stateLabel(data.OrigActiveNode.Consensus)))
	votingSlotDelta := uint64(0)
	if data.Voting.LastSlot >= data.Voting.FirstSlot {
		votingSlotDelta = data.Voting.LastSlot - data.Voting.FirstSlot
	}

	tpl, err := template.New("failoverSummary").Funcs(funcMap).Parse(`
  {{ Passive .OrigActiveNode.Hostname true }}
        {{ Muted "role     =" }} {{ Passive "passive" false }}
        {{ Muted "identity =" }} {{ Passive (truncPubkey .OrigActiveNode.Identities.Passive.PubKey) false }}
        {{ Muted "ip       =" }} {{ LightGrey .OrigActiveNode.PublicIP }}
        {{ Muted "took     =" }} {{ LightGrey (FormatDuration .OrigActiveSetIdentityDuration) }}
        {{ Muted "at_slot  =" }} {{ LightGrey (FormatSlot .FailoverStartSlot) }}


  {{ LightGrey (stateLabel .OrigActiveNode.Consensus) }}{{ if .IsDryRun }} {{ Muted "(pre-demotion dry-run snapshot)" }}{{ end }}
        {{ Muted "took     =" }} {{ LightGrey (FormatDuration .HistorySyncDuration) }}
        {{ Muted "size     =" }} {{ LightGrey (FormatBytes .VoteHistoryFileSizeBytes) }}

  {{ Active .OrigPassiveNode.Hostname true }}
        {{ Muted "role     =" }} {{ Active "active" false }}
        {{ Muted "identity =" }} {{ Active (truncPubkey .OrigPassiveNode.Identities.Active.PubKey) false }}
        {{ Muted "ip       =" }} {{ LightGrey .OrigPassiveNode.PublicIP }}
        {{ Muted "took     =" }} {{ LightGrey (FormatDuration .OrigPassiveSetIdentityDuration) }}
        {{ Muted "at_slot  =" }} {{ LightGrey (FormatSlot .FailoverEndSlot) }}

  {{ if .IsDryRun }}{{ Blue (printf "✓ Dry run complete in %s%s" (FormatDuration .TotalDuration) (SlotsSuffix .SlotsDuration)) }}{{ else if .SuccessfullyCompleted }}{{ Active (printf "✓ Failover complete in %s%s" (FormatDuration .TotalDuration) (SlotsSuffix .SlotsDuration)) false }}{{ else }}{{ Passive "Identity handover completed; voting unverified" false }}{{ end }}
  {{ if eq .Voting.Status "confirmed" }}{{ Active (printf "✓ Voting check confirmed in %s %s (%s → %s)" (FormatDuration .Voting.Duration) (trimPrefix (SlotsSuffix .VotingSlotDelta) ", ") (FormatSlot .Voting.FirstSlot) (FormatSlot .Voting.LastSlot)) false }}{{ else }}{{ Purple "Post-failover voting check:" }} {{ if eq .Voting.Status "disabled" }}{{ LightGrey "disabled" }}{{ else if eq .Voting.Status "dry-run" }}{{ LightGrey "not performed (dry run)" }}{{ else }}{{ Passive "unverified" false }}{{ end }}{{ end }}
`)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{
		"IsDryRun":                       data.IsDryRun,
		"Voting":                         data.Voting,
		"VotingSlotDelta":                votingSlotDelta,
		"SuccessfullyCompleted":          data.SuccessfullyCompleted,
		"OrigActiveNode":                 data.OrigActiveNode,
		"OrigPassiveNode":                data.OrigPassiveNode,
		"LabelWidth":                     labelWidth,
		"OrigActiveSetIdentityDuration":  data.OrigActiveSetIdentityDuration,
		"HistorySyncDuration":            data.HistorySyncDuration,
		"VoteHistoryFileSizeBytes":       data.VoteHistoryFileSizeBytes,
		"OrigPassiveSetIdentityDuration": data.OrigPassiveSetIdentityDuration,
		"TotalDuration":                  data.TotalDuration,
		"FailoverStartSlot":              data.FailoverStartSlot,
		"FailoverEndSlot":                data.FailoverEndSlot,
		"SlotsDuration":                  data.SlotsDuration,
	}); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
