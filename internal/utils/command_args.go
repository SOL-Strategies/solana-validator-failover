package utils

import (
	"fmt"
	"strings"
	"unicode"
)

// CommandArgs splits a command into arguments, honoring quotes and escapes.
// It does not expand variables or invoke a shell.
func CommandArgs(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, c := range command {
		if escaped {
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			started = true
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if unicode.IsSpace(c) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(c)
		started = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("identity command contains an unfinished quote or escape")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("identity command is empty")
	}
	return args, nil
}

// RunIdentityCommand uses the same argument parser as handoff preflight.
func RunIdentityCommand(command string, dryRun, debug bool) error {
	args, err := CommandArgs(command)
	if err != nil {
		return err
	}
	return RunCommand(RunCommandParams{CommandSlice: args, DryRun: dryRun, LogDebug: debug})
}
