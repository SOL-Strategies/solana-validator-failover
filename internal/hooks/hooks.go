package hooks

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

// Hook is a hook that is called before or after a failover
type Hook struct {
	Name        string   `mapstructure:"name"`
	Command     string   `mapstructure:"command"`
	Args        []string `mapstructure:"args"`
	MustSucceed bool     `mapstructure:"must_succeed"`
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

// Run runs the hook
func (h Hook) Run(envMap map[string]string, hookType string, hookIndex int, totalHooks int) error {
	hookLogger := log.With().Logger()
	// run the command passing in custom env variables about the state using os.exec
	cmd := exec.Command(h.Command, h.Args...)
	for k, v := range utils.SortStringMap(envMap) {
		// Trim newlines and whitespace from the value
		cleanValue := strings.TrimSpace(v)
		cmd.Env = append(cmd.Env, fmt.Sprintf("SOLANA_VALIDATOR_FAILOVER_%s=%s", k, cleanValue))
	}

	hookLogger.Debug().
		Str("command", h.Command).
		Str("args", fmt.Sprintf("[%s]", strings.Join(h.Args, ", "))).
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
		Str("command", h.Command).
		Str("args", fmt.Sprintf("[%s]", strings.Join(h.Args, ", "))).
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
		streamOutput(hookLogger, stdout, "stdout", h.Name, hookType, hookIndex, totalHooks)
	}()
	go func() {
		defer wg.Done()
		streamOutput(hookLogger, stderr, "stderr", h.Name, hookType, hookIndex, totalHooks)
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

// streamOutput streams output from a pipe to the logger in real-time
func streamOutput(logger zerolog.Logger, pipe io.ReadCloser, streamType string, hookName string, hookType string, hookIndex int, totalHooks int) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			// Use styled output with the requested format
			styledOutput := styledStreamOutputString(streamType, line, hookName, hookType, hookIndex, totalHooks)
			// Log without adding hook name as a structured field
			logger.Info().Msg(styledOutput)
		}
	}

	if err := scanner.Err(); err != nil {
		// Only log if it's not a "file already closed" error, which is expected
		if !strings.Contains(err.Error(), "file already closed") {
			logger.Error().Err(err).Msg("error reading hook output")
		}
	}
}

// Define styles using lipgloss - matching the reference repository colors
var (
	stderrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("124"))
	stdoutStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("28"))
	prefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Grey for prefix
)

// styledStreamOutputString creates styled output for stream content with the requested format
func styledStreamOutputString(stream string, text string, hookName string, hookType string, hookIndex int, totalHooks int) string {
	// Format: 🪝 hooks:<pre|post>:[1/1 <hook-name>]: <script output>
	prefix := fmt.Sprintf("🪝  hooks:%s:[%d/%d %s]:", hookType, hookIndex, totalHooks, hookName)
	styledPrefix := prefixStyle.Render(prefix)

	// Apply color to the script output based on stream type
	var outputStyle lipgloss.Style
	if stream == "stderr" {
		outputStyle = stderrStyle
	} else {
		outputStyle = stdoutStyle
	}

	styledOutput := outputStyle.Render(text)

	return fmt.Sprintf("%s %s", styledPrefix, styledOutput)
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
