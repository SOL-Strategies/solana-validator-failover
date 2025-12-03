package hooks

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

// Hook is a hook that is called before or after a failover
type Hook struct {
	Name        string            `mapstructure:"name"`
	Command     string            `mapstructure:"command"`
	Args        []string          `mapstructure:"args"`
	MustSucceed bool              `mapstructure:"must_succeed"`
	Environment map[string]string `mapstructure:"environment"`
}

// Hooks is a collection of hooks
type Hooks []Hook

// PreHooks is a collection of pre hooks
type PreHooks struct {
	WhenPassive Hooks `mapstructure:"when_passive"`
	WhenActive  Hooks `mapstructure:"when_active"`
}

// PostHooks is a collection of post hooks
type PostHooks struct {
	WhenPassive Hooks `mapstructure:"when_passive"`
	WhenActive  Hooks `mapstructure:"when_active"`
}

// FailoverHooks is a collection of hooks for pre and post failover
type FailoverHooks struct {
	Pre  PreHooks  `mapstructure:"pre"`
	Post PostHooks `mapstructure:"post"`
}

// HasPreHooksWhenActive returns true if there are any pre hooks when the validator is active
func (h FailoverHooks) HasPreHooksWhenActive() bool {
	return len(h.Pre.WhenActive) > 0
}

// HasPreHooksWhenPassive returns true if there are any pre hooks when the validator is passive
func (h FailoverHooks) HasPreHooksWhenPassive() bool {
	return len(h.Pre.WhenPassive) > 0
}

// HookTemplateData is the data structure available for hook templates
type HookTemplateData struct {
	// Failover state
	IsDryRunFailover bool
	ThisNodeRole     string
	PeerNodeRole     string

	// This node info
	ThisNodeName                   string
	ThisNodePublicIP               string
	ThisNodeActiveIdentityPubkey   string
	ThisNodeActiveIdentityKeyFile  string
	ThisNodePassiveIdentityPubkey  string
	ThisNodePassiveIdentityKeyFile string
	ThisNodeClientVersion          string
	ThisNodeRPCAddress             string

	// Peer node info
	PeerNodeName                  string
	PeerNodePublicIP              string
	PeerNodeActiveIdentityPubkey  string
	PeerNodePassiveIdentityPubkey string
	PeerNodeClientVersion         string
}

// newHookTemplateData creates a HookTemplateData from an envMap
func newHookTemplateData(envMap map[string]string) HookTemplateData {
	data := HookTemplateData{}

	// Parse boolean
	if envMap["IS_DRY_RUN_FAILOVER"] == "true" {
		data.IsDryRunFailover = true
	}

	// Parse roles
	data.ThisNodeRole = envMap["THIS_NODE_ROLE"]
	data.PeerNodeRole = envMap["PEER_NODE_ROLE"]

	// Parse this node info
	data.ThisNodeName = envMap["THIS_NODE_NAME"]
	data.ThisNodePublicIP = envMap["THIS_NODE_PUBLIC_IP"]
	data.ThisNodeActiveIdentityPubkey = envMap["THIS_NODE_ACTIVE_IDENTITY_PUBKEY"]
	data.ThisNodeActiveIdentityKeyFile = envMap["THIS_NODE_ACTIVE_IDENTITY_KEYPAIR_FILE"]
	data.ThisNodePassiveIdentityPubkey = envMap["THIS_NODE_PASSIVE_IDENTITY_PUBKEY"]
	data.ThisNodePassiveIdentityKeyFile = envMap["THIS_NODE_PASSIVE_IDENTITY_KEYPAIR_FILE"]
	data.ThisNodeClientVersion = envMap["THIS_NODE_CLIENT_VERSION"]
	data.ThisNodeRPCAddress = envMap["THIS_NODE_RPC_ADDRESS"]

	// Parse peer node info
	data.PeerNodeName = envMap["PEER_NODE_NAME"]
	data.PeerNodePublicIP = envMap["PEER_NODE_PUBLIC_IP"]
	data.PeerNodeActiveIdentityPubkey = envMap["PEER_NODE_ACTIVE_IDENTITY_PUBKEY"]
	data.PeerNodePassiveIdentityPubkey = envMap["PEER_NODE_PASSIVE_IDENTITY_PUBKEY"]
	data.PeerNodeClientVersion = envMap["PEER_NODE_CLIENT_VERSION"]

	return data
}

// executeTemplate executes a template string with the given data
func executeTemplate(tmplStr string, data HookTemplateData) (string, error) {
	// If template string doesn't contain template syntax, return as-is
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	tmpl, err := template.New("hook").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderHookCommand renders a hook's command and arguments with templates applied for display purposes.
// This is used both for displaying the failover plan and during actual hook execution.
// Returns the rendered command string (command + args joined with spaces).
func RenderHookCommand(hook Hook, templateData HookTemplateData) (string, error) {
	// Execute template for command
	command, err := executeTemplate(hook.Command, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to execute command template: %w", err)
	}

	// Execute templates for args
	args := make([]string, len(hook.Args))
	for i, arg := range hook.Args {
		executedArg, err := executeTemplate(arg, templateData)
		if err != nil {
			return "", fmt.Errorf("failed to execute arg[%d] template: %w", i, err)
		}
		args[i] = executedArg
	}

	// Combine command and args
	if len(args) > 0 {
		return command + " " + strings.Join(args, " "), nil
	}
	return command, nil
}

// Run runs the hook
func (h Hook) Run(envMap map[string]string, hookType string, hookIndex int, totalHooks int) error {
	hookLogger := log.With().Logger()

	// Create template data from envMap
	templateData := newHookTemplateData(envMap)

	// Execute templates for command and args
	// Note: We keep this separate from RenderHookCommand to properly handle
	// arguments with spaces for exec.Command
	command, err := executeTemplate(h.Command, templateData)
	if err != nil {
		return fmt.Errorf("Hook %s failed to execute command template: %w", h.Name, err)
	}

	args := make([]string, len(h.Args))
	for i, arg := range h.Args {
		executedArg, err := executeTemplate(arg, templateData)
		if err != nil {
			return fmt.Errorf("Hook %s failed to execute arg[%d] template: %w", h.Name, i, err)
		}
		args[i] = executedArg
	}

	// run the command passing in custom env variables about the state using os.exec
	cmd := exec.Command(command, args...)

	// Build environment variables as a map first
	envVars := make(map[string]string)

	// Add custom environment variables from config first (with template support)
	// These can be overridden by SOLANA_VALIDATOR_FAILOVER_* variables below
	if h.Environment != nil {
		for envKey, envValue := range h.Environment {
			// Execute template for environment variable value
			executedValue, err := executeTemplate(envValue, templateData)
			if err != nil {
				return fmt.Errorf("Hook %s failed to execute environment variable %s template: %w", h.Name, envKey, err)
			}
			// Trim newlines and whitespace from the value
			cleanValue := strings.TrimSpace(executedValue)
			envVars[envKey] = cleanValue
		}
	}

	// Add standard failover environment variables last (so they can't be clobbered)
	for k, v := range utils.SortStringMap(envMap) {
		// Trim newlines and whitespace from the value
		cleanValue := strings.TrimSpace(v)
		envVars[fmt.Sprintf("SOLANA_VALIDATOR_FAILOVER_%s", k)] = cleanValue
	}

	// Append all environment variables to cmd.Env, ensuring all keys are uppercase
	for envKey, envValue := range utils.SortStringMap(envVars) {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", strings.ToUpper(envKey), envValue))
	}

	hookLogger.Debug().
		Str("command_template", h.Command).
		Str("command_executed", command).
		Str("args_template", fmt.Sprintf("[%s]", strings.Join(h.Args, ", "))).
		Str("args_executed", fmt.Sprintf("[%s]", strings.Join(args, ", "))).
		Str("env", fmt.Sprintf("[%s]", strings.Join(cmd.Env, ", "))).
		Msg("running hook")

	// Capture stdout and stderr separately
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Hook %s failed to create stdout pipe: %v", h.Name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("Hook %s failed to create stderr pipe: %v", h.Name, err)
	}

	// Start the command
	hookLogger.Info().
		Str("command", command).
		Str("args", fmt.Sprintf("[%s]", strings.Join(args, ", "))).
		Msgf("🪝  Running hook %s", h.Name)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Hook %s failed to start: %v", h.Name, err)
	}

	// get the command pid (only after successful start)
	pid := cmd.Process.Pid
	hookLogger.Debug().Int("pid", pid).Msg("hook process started")

	// Use WaitGroup to ensure goroutines complete before we return
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout and stderr in real-time using hookLogger
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				styledOutput := styledStreamOutputString("stdout", line, h.Name, hookType, hookIndex, totalHooks)
				hookLogger.Info().Msg(styledOutput)
			}
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				styledOutput := styledStreamOutputString("stderr", line, h.Name, hookType, hookIndex, totalHooks)
				hookLogger.Info().Msg(styledOutput)
			}
		}
	}()

	// Wait for the command to complete
	err = cmd.Wait()

	// Wait for streaming goroutines to finish
	wg.Wait()

	if err != nil {
		return fmt.Errorf("🪝 🔴 Hook %s failed: %v", h.Name, err)
	}

	hookLogger.Info().Msgf("🪝  Hook %s completed successfully", h.Name)
	return nil
}

// Define styles using lipgloss - matching the reference repository colors
var (
	stderrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("124")) // red
	stdoutStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("28"))  // green
	StdStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250")) // light grey
	PrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))   // Grey for prefix
)

// styledStreamOutputString creates styled output for stream content with the requested format
func styledStreamOutputString(stream string, text string, hookName string, hookType string, hookIndex int, totalHooks int) string {
	// Format: 🪝 hooks:<pre|post>:[1/1 <hook-name>]: ▶ <script output>
	prefix := fmt.Sprintf("🪝  hooks:%s:[%d/%d %s]:", hookType, hookIndex, totalHooks, hookName)
	styledPrefix := PrefixStyle.Render(prefix)

	// Apply color to the script output based on stream type
	var cursorStyle lipgloss.Style
	if stream == "stderr" {
		cursorStyle = stderrStyle
	} else {
		cursorStyle = stdoutStyle
	}
	styledCursor := cursorStyle.Render("▶")

	return fmt.Sprintf("%s %s %s", styledPrefix, styledCursor, StdStyle.Render(text))
}

// RunPreWhenPassive runs the pre hooks when the validator is passive
func (h FailoverHooks) RunPreWhenPassive(envMap map[string]string) error {
	for i, hook := range h.Pre.WhenPassive {
		err := hook.Run(envMap, "pre", i+1, len(h.Pre.WhenPassive))
		if err != nil && hook.MustSucceed {
			return err
		}
		if err != nil {
			log.Error().Err(err).Msgf("pre hook %s failed - must_succeed is false, continuing...", hook.Name)
		}
	}
	return nil
}

// RunPreWhenActive runs the pre hooks when the validator is active
func (h FailoverHooks) RunPreWhenActive(envMap map[string]string) error {
	for i, hook := range h.Pre.WhenActive {
		err := hook.Run(envMap, "pre", i+1, len(h.Pre.WhenActive))
		if err != nil && hook.MustSucceed {
			return err
		}
		if err != nil {
			log.Error().Err(err).Msgf("pre hook %s failed - must_succeed is false, continuing...", hook.Name)
			continue
		}
	}
	return nil
}

// RunPostWhenPassive runs the post hooks when the validator is passive
func (h FailoverHooks) RunPostWhenPassive(envMap map[string]string) {
	for i, hook := range h.Post.WhenPassive {
		err := hook.Run(envMap, "post", i+1, len(h.Post.WhenPassive))
		if err != nil {
			log.Error().Err(err).Msgf("post hook %s failed", hook.Name)
		}
	}
}

// RunPostWhenActive runs the post hooks when the validator is active
func (h FailoverHooks) RunPostWhenActive(envMap map[string]string) {
	for i, hook := range h.Post.WhenActive {
		err := hook.Run(envMap, "post", i+1, len(h.Post.WhenActive))
		if err != nil {
			log.Error().Err(err).Msgf("post hook %s failed", hook.Name)
		}
	}
}
