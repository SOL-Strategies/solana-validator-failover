package solanavalidatorfailover

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sol-strategies/solana-validator-failover/internal/config"
	internalconstants "github.com/sol-strategies/solana-validator-failover/internal/constants"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
	"github.com/sol-strategies/solana-validator-failover/pkg/constants"
	"github.com/spf13/cobra"
)

var (
	// Validator available to all commands
	configPath string
	logLevel   string
	rootCmd    = &cobra.Command{
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

	// execute
	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err)
	}
}

func init() {
	cobra.OnInitialize(initLog)
}

func initLog() {
	// configure logger
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:          os.Stderr,
		TimeLocation: time.UTC,
		NoColor:      false,
		TimeFormat:   time.RFC3339Nano, // RFC3339 with UTC timezone and nanoseconds
		FormatLevel: func(i any) string {
			levelStr := i.(string)
			return style.LogLevels[levelStr].Bold(true).Width(5).Render(strings.ToUpper(levelStr))
		},
		FormatFieldName: func(i any) string {
			return style.RenderGreyString(i.(string)+"=", false)
		},
		FormatFieldValue: func(i any) string {
			// Zerolog passes non-string field values (bools, arrays, etc.) as raw
			// JSON-encoded []byte. Convert to string before further formatting so
			// that e.g. Bool("must_succeed", true) renders as "true" rather than
			// the byte sequence [116 114 117 101].
			var value string
			if b, ok := i.([]byte); ok {
				value = string(b)
			} else {
				value = fmt.Sprintf("%v", i)
			}
			isPassive := strings.HasPrefix(value, internalconstants.NodeRolePassive)
			isActive := strings.HasPrefix(value, internalconstants.NodeRoleActive)
			if isPassive {
				return style.RenderPassiveString(strings.TrimPrefix(value, internalconstants.NodeRolePassive), false)
			}
			if isActive {
				return style.RenderActiveString(strings.TrimPrefix(value, internalconstants.NodeRoleActive), false)
			}
			return value
		},
		FormatMessageFromEvent: func(evt map[string]any) zerolog.Formatter {
			return func(i any) string {
				levelStr := evt[zerolog.LevelFieldName].(string)
				return style.LogLevels[levelStr].Render(i.(string))
			}
		},
	}).With().Timestamp().Logger()
}

// configureLogger configures the logger
func persistentPreRun(cmd *cobra.Command, args []string) (err error) {
	// set zerolog level
	logLevel, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}
	zerolog.SetGlobalLevel(logLevel)

	return nil
}
