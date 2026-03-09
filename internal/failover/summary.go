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
	IsDryRun      bool
	SkipTowerSync bool

	// OrigActiveNode is the node that initiated the failover (was active, now passive).
	// OrigPassiveNode is the node that received the failover (was passive, now active).
	OrigActiveNode  NodeInfo
	OrigPassiveNode NodeInfo

	// Timing
	OrigActiveSetIdentityDuration  time.Duration
	TowerSyncDuration              time.Duration
	TowerFileSizeBytes             int64
	OrigPassiveSetIdentityDuration time.Duration
	TotalDuration                  time.Duration

	// Slots
	FailoverStartSlot uint64
	FailoverEndSlot   uint64
	SlotsDuration     uint64

	// Vote credit rank (optional, populated after credit monitoring)
	HasVoteRankData bool
	VoteRankDiff    int
	VoteRankFirst   int
	VoteRankLast    int
}

// RenderFailoverSummary renders the post-failover summary to a string.
func RenderFailoverSummary(data SummaryData) (string, error) {
	funcMap := template.FuncMap{
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
	if !data.SkipTowerSync {
		labelWidth = max(labelWidth, len("> tower"))
	}

	tpl, err := template.New("failoverSummary").Funcs(funcMap).Parse(`
  {{ Passive .OrigActiveNode.Hostname true }}
        role     = {{ Passive "passive" false }}
        identity = {{ Passive (truncPubkey .OrigActiveNode.Identities.Passive.PubKey) false }}
        ip       = {{ LightGrey .OrigActiveNode.PublicIP }}
        took     = {{ LightGrey (FormatDuration .OrigActiveSetIdentityDuration) }}
        at_slot  = {{ LightGrey (FormatSlot .FailoverStartSlot) }}
{{ if not .SkipTowerSync }}
  {{ LightGrey "tower" }}
        took     = {{ LightGrey (FormatDuration .TowerSyncDuration) }}
        size     = {{ LightGrey (FormatBytes .TowerFileSizeBytes) }}
{{ end }}
  {{ Active .OrigPassiveNode.Hostname true }}
        role     = {{ Active "active" false }}
        identity = {{ Active (truncPubkey .OrigPassiveNode.Identities.Active.PubKey) false }}
        ip       = {{ LightGrey .OrigPassiveNode.PublicIP }}
        took     = {{ LightGrey (FormatDuration .OrigPassiveSetIdentityDuration) }}
        at_slot  = {{ LightGrey (FormatSlot .FailoverEndSlot) }}
{{ if .HasVoteRankData }}
  {{ Purple "Vote credits:" }} rank {{ if gt .VoteRankDiff 0 }}{{ Active (printf "improved by +%d" .VoteRankDiff) false }}{{ else if lt .VoteRankDiff 0 }}{{ Passive (printf "worsened by %d" .VoteRankDiff) false }}{{ else }}{{ LightGrey "unchanged" }}{{ end }} ({{ .VoteRankFirst }} → {{ .VoteRankLast }})
{{ end }}
  {{ if .IsDryRun }}{{ Blue (printf "✓ Dry run complete in %s%s" (FormatDuration .TotalDuration) (SlotsSuffix .SlotsDuration)) }}{{ else }}{{ Active (printf "✓ Failover complete in %s%s" (FormatDuration .TotalDuration) (SlotsSuffix .SlotsDuration)) false }}{{ end }}
`)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{
		"IsDryRun":                       data.IsDryRun,
		"SkipTowerSync":                  data.SkipTowerSync,
		"OrigActiveNode":                 data.OrigActiveNode,
		"OrigPassiveNode":                data.OrigPassiveNode,
		"LabelWidth":                     labelWidth,
		"OrigActiveSetIdentityDuration":  data.OrigActiveSetIdentityDuration,
		"TowerSyncDuration":              data.TowerSyncDuration,
		"TowerFileSizeBytes":             data.TowerFileSizeBytes,
		"OrigPassiveSetIdentityDuration": data.OrigPassiveSetIdentityDuration,
		"TotalDuration":                  data.TotalDuration,
		"FailoverStartSlot":              data.FailoverStartSlot,
		"FailoverEndSlot":                data.FailoverEndSlot,
		"SlotsDuration":                  data.SlotsDuration,
		"HasVoteRankData":                data.HasVoteRankData,
		"VoteRankDiff":                   data.VoteRankDiff,
		"VoteRankFirst":                  data.VoteRankFirst,
		"VoteRankLast":                   data.VoteRankLast,
	}); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
