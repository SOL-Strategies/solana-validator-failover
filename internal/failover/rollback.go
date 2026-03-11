package failover

import (
	"strings"

	"github.com/rs/zerolog"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

// RunRollbackToActive is called on the active node (which just switched to passive) to revert to active.
// It runs the set-identity-to-active command, then post-hooks.
// Post-hooks always run even if the command failed.
// Returns the set-identity command error (if any); hook errors are logged but not returned.
func RunRollbackToActive(cfg hooks.RollbackConfig, envMap map[string]string, isDryRun bool, logger zerolog.Logger) error {
	return runRollback(cfg.ToActive, envMap, "to-active", isDryRun, logger)
}

// RunRollbackToPassive is called on the passive node (which failed to become active) to re-assert passive.
// It runs the set-identity-to-passive command, then post-hooks.
// Post-hooks always run even if the command failed.
// Returns the set-identity command error (if any); hook errors are logged but not returned.
func RunRollbackToPassive(cfg hooks.RollbackConfig, envMap map[string]string, isDryRun bool, logger zerolog.Logger) error {
	return runRollback(cfg.ToPassive, envMap, "to-passive", isDryRun, logger)
}

func runRollback(dir hooks.RollbackDirectionConfig, envMap map[string]string, dirName string, isDryRun bool, logger zerolog.Logger) error {
	logger.Warn().Msgf("rollback %s: starting", dirName)

	// set-identity command
	var cmdErr error
	if dir.ResolvedCmd == "" {
		logger.Error().Msgf("rollback %s: no command configured — cannot execute rollback set-identity", dirName)
	} else {
		logger.Warn().Str("command", dir.ResolvedCmd).Msgf("rollback %s: running set-identity command", dirName)
		cmdErr = utils.RunCommand(utils.RunCommandParams{
			CommandSlice: strings.Split(dir.ResolvedCmd, " "),
			DryRun:       isDryRun,
			LogDebug:     logger.Debug().Enabled(),
		})
		if cmdErr != nil {
			logger.Error().Err(cmdErr).Msgf("rollback %s: set-identity command failed", dirName)
		} else {
			logger.Warn().Msgf("rollback %s: set-identity command succeeded", dirName)
		}
	}

	// post-rollback hooks — always run, even if cmd failed; errors logged, never fatal
	for i, hook := range dir.Hooks.Post {
		if err := hook.Run(envMap, "rollback-post", i+1, len(dir.Hooks.Post)); err != nil {
			logger.Error().Err(err).Msgf("rollback %s: post-hook %s failed", dirName, hook.Name)
		}
	}

	if cmdErr != nil {
		logger.Error().Msgf("rollback %s: FAILED — manual intervention may be required", dirName)
		return cmdErr
	}
	logger.Warn().Msgf("rollback %s: complete", dirName)
	return nil
}
