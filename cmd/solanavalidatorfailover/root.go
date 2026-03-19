package solanavalidatorfailover

import (
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/sol-strategies/solana-validator-failover/internal/config"
	"github.com/sol-strategies/solana-validator-failover/internal/logging"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
	"github.com/sol-strategies/solana-validator-failover/internal/updater"
	"github.com/sol-strategies/solana-validator-failover/pkg/constants"
	"github.com/spf13/cobra"
)

var (
	// Validator available to all commands
	configPath    string
	logLevel      string
	noUpdateCheck bool
	updateCh      chan string
	rootCmd       = &cobra.Command{
		Aliases: []string{},
		Use:     style.RenderPurpleString(constants.AppName),
		Version: constants.AppVersion,
		Short: fmt.Sprintf(
			"%s (%s) - ⚡ %s",
			style.RenderPurpleString(constants.AppName),
			style.RenderPurpleString(constants.AppVersion),
			style.RenderActiveString("p2p solana validator failover", false),
		),
		Long: fmt.Sprintf(`
%s - %s

Version:
    %s
`, style.RenderPurpleString(constants.AppName),
			style.RenderActiveString("⚡ p2p solana validator failover", false),
			style.RenderPurpleString(constants.AppVersion),
		),
		PersistentPreRunE: persistentPreRun,
	}
)

// Execute ...
func Execute() {
	// config flag
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", config.DefaultConfigPath, "path to config file")
	// log level flag
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "log level")
	// update check flag
	rootCmd.PersistentFlags().BoolVarP(&noUpdateCheck, "no-update-check", "n", false, "skip update check")

	updateCh = updater.StartBackgroundCheck(constants.AppVersion)

	// execute
	if err := rootCmd.Execute(); err != nil {
		log.Fatal("command failed", "err", err)
	}
}

func init() {
	// Suppress quic-go's UDP receive buffer size warning — informational only,
	// doesn't affect functionality. See https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true") //nolint:errcheck
}

func persistentPreRun(cmd *cobra.Command, args []string) error {
	logging.Configure(logLevel)

	if !noUpdateCheck {
		updater.PrintWarningIfAvailable(updateCh)
	}

	return nil
}
