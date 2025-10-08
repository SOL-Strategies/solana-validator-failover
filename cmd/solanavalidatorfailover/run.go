package solanavalidatorfailover

import (
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/config"
	"github.com/sol-strategies/solana-validator-failover/internal/validator"
	"github.com/spf13/cobra"
)

var (
	// Validator available to all commands
	notADrill             bool
	noWaitForHealthy      bool
	noMinTimeToLeaderSlot bool
	runCmd                = &cobra.Command{
		Use:          "run",
		Short:        "run a failover - automatically detects what to do based on the node's role (active or passive)",
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.NewFromFile(configPath)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to load config")
			}

			v, err := validator.NewFromConfig(&cfg.Validator)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to create validator")
			}

			err = v.Failover(validator.FailoverParams{
				NotADrill:                    notADrill, // ignored when run on active node
				NoWaitForHealthy:             noWaitForHealthy,
				NoWaitForMinTimeToLeaderSlot: noMinTimeToLeaderSlot,
			})
			if err != nil {
				log.Fatal().Err(err).Msg("failed to failover")
			}
		},
	}
)

func init() {
	runCmd.Flags().BoolVar(&notADrill, "not-a-drill", false, "execute failover for real (not a drill)")
	runCmd.Flags().BoolVar(&noWaitForHealthy, "no-wait-for-healthy", false, "don't wait for node to report being healthy by calling <config.validator.rpc_address>/health")
	runCmd.Flags().BoolVar(&noMinTimeToLeaderSlot, "no-min-time-to-leader-slot", false, "when run on an active node, don't wait until it has no leader slots in the next <config.validator.min_time_to_leader_slot> (default: 5m) - ignored when run on a passive node")
	rootCmd.AddCommand(runCmd)
}
