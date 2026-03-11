package failover

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
)

// PlanData holds all data needed to render the failover plan template.
type PlanData struct {
	IsDryRun            bool
	SkipTowerSync       bool
	ActiveNodeInfo      NodeInfo
	PassiveNodeInfo     NodeInfo
	AppVersion          string
	Hooks               hooks.FailoverHooks
	Rollback            hooks.RollbackConfig
	ActivePreHookData   hooks.HookTemplateData
	ActivePostHookData  hooks.HookTemplateData
	PassivePreHookData  hooks.HookTemplateData
	PassivePostHookData hooks.HookTemplateData
}

// RenderFailoverPlan renders the failover confirmation plan to a string.
func RenderFailoverPlan(data PlanData) (string, error) {
	// step is a closure counter so each conditional section gets the correct ordinal.
	step := 0
	funcMap := template.FuncMap{
		// Step increments and returns the current step number.
		"Step": func() int {
			step++
			return step
		},
		// truncPubkey uses ASCII "..." so byte-length == display-width, enabling printf padding.
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
		// FormatBytes converts bytes to a human-readable size string.
		"FormatBytes": func(n int64) string {
			return humanize.Bytes(uint64(n))
		},
		// planSummaryLines builds the multi-line Plan: value. Each component
		// (identity changes, tower sync, hooks) sits on its own line. Continuation
		// lines are indented to align with the value start after "   Plan: ".
		// The indent is 11 chars: 2 (template leading spaces) + 8 (label) + 1 (space).
		"planSummaryLines": func(activeHostname, passiveHostname string, skipTowerSync bool, h hooks.FailoverHooks, rollback hooks.RollbackConfig) string {
			const indent = "           " // 11 spaces

			identityLine := fmt.Sprintf("2 identity changes (%s %s, %s %s)",
				activeHostname, style.RenderPassiveString("passive", false),
				passiveHostname, style.RenderActiveString("active", false))

			lines := []string{identityLine}

			if !skipTowerSync {
				lines = append(lines, "1 tower sync")
			}

			type entry struct {
				label string
				n     int
			}
			entries := []entry{
				{"pre-passive", len(h.Pre.WhenActive)},
				{"pre-active", len(h.Pre.WhenPassive)},
				{"post-active", len(h.Post.WhenActive)},
				{"post-passive", len(h.Post.WhenPassive)},
			}
			total := 0
			var parts []string
			for _, e := range entries {
				if e.n > 0 {
					total += e.n
					parts = append(parts, fmt.Sprintf("%d %s", e.n, e.label))
				}
			}
			if total > 0 {
				lines = append(lines, fmt.Sprintf("%d hooks (%s)", total, strings.Join(parts, ", ")))
			}

			if rollback.Enabled {
				lines = append(lines, "1 rollback plan (if failover fails)")
			}

			return strings.Join(lines, "\n"+indent)
		},
		"RenderHooks": func(hooksList hooks.Hooks, nodeName string, hookType string) string {
			if len(hooksList) == 0 {
				return ""
			}
			var hookData hooks.HookTemplateData
			switch {
			case nodeName == data.ActiveNodeInfo.Hostname && hookType == "pre":
				hookData = data.ActivePreHookData
			case nodeName == data.ActiveNodeInfo.Hostname:
				hookData = data.ActivePostHookData
			case hookType == "pre":
				hookData = data.PassivePreHookData
			default:
				hookData = data.PassivePostHookData
			}

			var result strings.Builder
			for i, hook := range hooksList {
				hookCmd, err := hooks.RenderHookCommand(hook, hookData)
				if err != nil {
					hookCmd = hook.Command
					if len(hook.Args) > 0 {
						hookCmd += " " + strings.Join(hook.Args, " ")
					}
				}
				mustSucceedStr := ""
				if hook.MustSucceed {
					mustSucceedStr = style.RenderLightWarningString(" (must succeed)")
				}
				fmt.Fprintf(&result, "    [%d/%d] %s%s: %s\n",
					i+1, len(hooksList),
					hooks.PrefixStyle.Render(hook.Name),
					mustSucceedStr,
					hooks.PrefixStyle.Render(hookCmd))
			}
			return result.String()
		},
	}
	maps.Copy(funcMap, style.TemplateFuncMap())

	// The LHS of role and identity lines is padded to the same width (15 chars) so
	// the → arrows align. truncPubkey always returns exactly 15 ASCII chars (8+...+4),
	// and printf "%-15s" pads the role strings to the same width.
	tpl, err := template.New("failoverPlan").Funcs(funcMap).Parse(`{{ if .Hooks.Pre.WhenActive }}
  {{ Purple (printf "%d — run hooks %s pre-passive" (Step) .ActiveNodeInfo.Hostname) }}
{{ RenderHooks .Hooks.Pre.WhenActive .ActiveNodeInfo.Hostname "pre" }}{{- end }}
  {{ $n := Step }}{{ Purple (printf "%d — set %s " $n .ActiveNodeInfo.Hostname) }}{{ Passive "passive" false }}
      {{ Warning "~" }} role      = {{ Active (printf "%-15s" "active") false }}  →  {{ Passive (printf "%-15s" "passive") false }}{{ if .IsDryRun }}  {{ LightGrey "(dry run)" }}{{ end }}
      {{ Warning "~" }} identity  = {{ Active (printf "%-15s" (truncPubkey .ActiveNodeInfo.Identities.Active.PubKey)) false }}  →  {{ Passive (printf "%-15s" (truncPubkey .ActiveNodeInfo.Identities.Passive.PubKey)) false }}{{ if .IsDryRun }}  {{ LightGrey "(dry run)" }}{{ end }}
        ip        = {{ LightGrey .ActiveNodeInfo.PublicIP }}
        version   = {{ LightGrey .ActiveNodeInfo.ClientVersion }}
        cmd       = {{ LightGrey .ActiveNodeInfo.SetIdentityCommand }}
{{- if not .SkipTowerSync }}

  {{ Purple (printf "%d — sync tower file" (Step)) }}
        source      = {{ LightGrey (printf "%s:%s" .ActiveNodeInfo.Hostname .ActiveNodeInfo.TowerFile) }}
      {{ Active "+" false }} destination = {{ LightGrey (printf "%s:%s" .PassiveNodeInfo.Hostname .PassiveNodeInfo.TowerFile) }}{{ if gt .ActiveNodeInfo.TowerFileSizeBytes 0 }}
        size        = {{ LightGrey (FormatBytes .ActiveNodeInfo.TowerFileSizeBytes) }}{{ end }}
{{- end }}
{{ if .Hooks.Pre.WhenPassive }}
  {{ Purple (printf "%d — run hooks %s pre-active" (Step) .PassiveNodeInfo.Hostname) }}
{{ RenderHooks .Hooks.Pre.WhenPassive .PassiveNodeInfo.Hostname "pre" }}{{- end }}
  {{ $n := Step }}{{ Purple (printf "%d — set %s " $n .PassiveNodeInfo.Hostname) }}{{ Active "active" false }}
      {{ Warning "~" }} role      = {{ Passive (printf "%-15s" "passive") false }}  →  {{ Active (printf "%-15s" "active") false }}{{ if .IsDryRun }}  {{ LightGrey "(dry run)" }}{{ end }}
      {{ Warning "~" }} identity  = {{ Passive (printf "%-15s" (truncPubkey .PassiveNodeInfo.Identities.Passive.PubKey)) false }}  →  {{ Active (printf "%-15s" (truncPubkey .PassiveNodeInfo.Identities.Active.PubKey)) false }}{{ if .IsDryRun }}  {{ LightGrey "(dry run)" }}{{ end }}
        ip        = {{ LightGrey .PassiveNodeInfo.PublicIP }}
        version   = {{ LightGrey .PassiveNodeInfo.ClientVersion }}
        cmd       = {{ LightGrey .PassiveNodeInfo.SetIdentityCommand }}
{{- if .Hooks.Post.WhenActive }}

  {{ Purple (printf "%d — run hooks %s post-active" (Step) .PassiveNodeInfo.Hostname) }}
{{ RenderHooks .Hooks.Post.WhenActive .PassiveNodeInfo.Hostname "post" }}{{- end }}{{- if .Hooks.Post.WhenPassive }}
  {{ Purple (printf "%d — run hooks %s post-passive" (Step) .ActiveNodeInfo.Hostname) }}
{{ RenderHooks .Hooks.Post.WhenPassive .ActiveNodeInfo.Hostname "post" }}{{- end }}
{{- if .Rollback.Enabled }}

  {{ Warning "Rollback" }} if failover fails:
      {{ Warning "!" }} {{ Purple .ActiveNodeInfo.Hostname }} → {{ Active "active" false }}: {{ LightGrey .Rollback.ToActive.ResolvedCmd }}
      {{ Warning "!" }} {{ Purple .PassiveNodeInfo.Hostname }} → {{ Passive "passive" false }}: {{ LightGrey .Rollback.ToPassive.ResolvedCmd }}
{{- end }}
  {{ HRule }}
  {{ Purple "   Plan:" }} {{ planSummaryLines .ActiveNodeInfo.Hostname .PassiveNodeInfo.Hostname .SkipTowerSync .Hooks .Rollback }}
  {{ Purple "Version:" }} {{ LightGrey .AppVersion }}
  {{ if .IsDryRun }}{{ Blue "   Note:" }} dry run — re-run with {{ LightGrey "--not-a-drill" }} on the passive node to do for realsies.{{ else }}{{ Warning "Warning:" }} This is a real failover — identities will be changed on both nodes.{{ end }}
  {{ HRule }}
`)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]any{
		"IsDryRun":        data.IsDryRun,
		"SkipTowerSync":   data.SkipTowerSync,
		"PassiveNodeInfo": data.PassiveNodeInfo,
		"ActiveNodeInfo":  data.ActiveNodeInfo,
		"AppVersion":      data.AppVersion,
		"Hooks":           data.Hooks,
		"Rollback":        data.Rollback,
	}); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
