package solanavalidatorfailover

import (
	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/internal/validator"
	"github.com/spf13/cobra"
)

var (
	// Validator available to all commands
	notADrill             bool
	noWaitForHealthy      bool
	noMinTimeToLeaderSlot bool
	skipHistoryTransfer   bool
	autoConfirm           bool
	rollbackEnabled       bool
	toPeer                string
	runCmd                = &cobra.Command{
		Use:          "run",
		Short:        "run a failover - automatically detects what to do based on the node's role (active or passive)",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			if loadedConfig == nil {
				log.Fatal("config was not loaded before running command")
			}

			v, err := validator.NewFromConfig(&loadedConfig.Validator)
			if err != nil {
				log.Fatal("failed to create validator", "err", err)
			}

			err = v.Failover(validator.FailoverParams{
				NotADrill:             notADrill, // ignored when run on active node
				NoWaitForHealthy:      noWaitForHealthy,
				NoMinTimeToLeaderSlot: noMinTimeToLeaderSlot, // ignored when run on passive node
				SkipHistoryTransfer:   skipHistoryTransfer,
				AutoConfirm:           autoConfirm,
				RollbackEnabled:       rollbackEnabled,
				ToPeer:                toPeer,
			})
			if err != nil {
				log.Fatal("failed to failover", "err", err)
			}
		},
	}
)

func init() {
	runCmd.Flags().BoolVar(&notADrill, "not-a-drill", false, "execute failover for real (not a drill)")
	runCmd.Flags().BoolVar(&noWaitForHealthy, "no-wait-for-healthy", false, "don't wait for node to report being healthy by calling <config.validator.rpc_address>/health")
	runCmd.Flags().BoolVar(&noMinTimeToLeaderSlot, "no-min-time-to-leader-slot", false, "when run on an active node, don't wait until it has no leader slots in the next <config.validator.min_time_to_leader_slot> (default: 5m) - ignored when run on a passive node")
	runCmd.Flags().BoolVar(&skipHistoryTransfer, "skip-history-transfer", false, "skip Alpenglow vote-history transfer (passive node controls this choice)")
	runCmd.Flags().Bool("skip-tower-sync", false, "unsupported; use --skip-history-transfer")
	_ = runCmd.Flags().MarkHidden("skip-tower-sync")
	runCmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "automatically answer yes to all prompts")
	runCmd.Flags().BoolVarP(&rollbackEnabled, "rollback-enabled", "r", false, "force-enable rollback regardless of the rollback.enabled config value")
	runCmd.Flags().StringVar(&toPeer, "to-peer", "", "when run on an active node, auto-select a peer by name or IP address (skips interactive prompt)")
	rootCmd.AddCommand(runCmd)
}
