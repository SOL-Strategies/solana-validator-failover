package failover

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/utils"
)

// RunRollbackToActive is called on the active node (which just switched to passive) to revert to active.
// It runs the set-identity-to-active command, then post-hooks.
// Post-hooks always run even if the command failed.
// Returns the set-identity command error (if any); hook errors are logged but not returned.
func RunRollbackToActive(cfg hooks.RollbackConfig, envMap map[string]string, isDryRun bool, logger *log.Logger) error {
	return runRollback(cfg.ToActive, envMap, "to-active", isDryRun, logger)
}

// RunRollbackToPassive is called on the passive node (which failed to become active) to re-assert passive.
// It runs the set-identity-to-passive command, then post-hooks.
// Post-hooks always run even if the command failed.
// Returns the set-identity command error (if any); hook errors are logged but not returned.
func RunRollbackToPassive(cfg hooks.RollbackConfig, envMap map[string]string, isDryRun bool, logger *log.Logger) error {
	return runRollback(cfg.ToPassive, envMap, "to-passive", isDryRun, logger)
}

func runRollback(dir hooks.RollbackDirectionConfig, envMap map[string]string, dirName string, isDryRun bool, logger *log.Logger) error {
	logger.Warnf("rollback %s: starting", dirName)

	// set-identity command
	var cmdErr error
	if dir.ResolvedCmd == "" {
		logger.Errorf("rollback %s: no command configured — cannot execute rollback set-identity", dirName)
	} else {
		logger.Warn(fmt.Sprintf("rollback %s: running set-identity command", dirName), "command", dir.ResolvedCmd)
		cmdErr = utils.RunCommand(utils.RunCommandParams{
			CommandSlice: strings.Split(dir.ResolvedCmd, " "),
			DryRun:       isDryRun,
			LogDebug:     logger.GetLevel() <= log.DebugLevel,
		})
		if cmdErr != nil {
			logger.Error(fmt.Sprintf("rollback %s: set-identity command failed", dirName), "err", cmdErr)
		} else {
			logger.Warnf("rollback %s: set-identity command succeeded", dirName)
		}
	}

	// post-rollback hooks — always run, even if cmd failed; errors logged, never fatal
	for i, hook := range dir.Hooks.Post {
		if err := hook.Run(envMap, "rollback-post", i+1, len(dir.Hooks.Post)); err != nil {
			logger.Error(fmt.Sprintf("rollback %s: post-hook %s failed", dirName, hook.Name), "err", err)
		}
	}

	if cmdErr != nil {
		logger.Errorf("rollback %s: FAILED — manual intervention may be required", dirName)
		return cmdErr
	}
	logger.Warnf("rollback %s: complete", dirName)
	return nil
}
