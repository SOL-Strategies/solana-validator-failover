package solanavalidatorfailover

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	appLogging "github.com/sol-strategies/solana-validator-failover/internal/logging"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRootTestConfig(t *testing.T, level, format string) string {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	content := "log:\n  level: " + level + "\n  format: " + format + "\nupdate:\n  check_on_startup: false\n"
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))
	return configFile
}

func resetRootTestState(t *testing.T) {
	t.Helper()
	previousConfigPath := configPath
	previousLogLevel := logLevel
	previousNoUpdateCheck := noUpdateCheck
	previousLoadedConfig := loadedConfig
	t.Cleanup(func() {
		configPath = previousConfigPath
		logLevel = previousLogLevel
		noUpdateCheck = previousNoUpdateCheck
		loadedConfig = previousLoadedConfig
		log.SetOutput(os.Stderr)
		appLogging.Configure("info", "text", "")
	})
}

func TestPersistentPreRun_UsesConfigLogging(t *testing.T) {
	resetRootTestState(t)
	configPath = writeRootTestConfig(t, "debug", "json")
	logLevel = ""
	noUpdateCheck = false
	appLogging.Configure("info", "text", "")
	var output bytes.Buffer
	log.SetOutput(&output)

	err := persistentPreRun(rootCmd, nil)
	require.NoError(t, err)
	log.Debug("configured-from-file")

	require.NotNil(t, loadedConfig)
	assert.Equal(t, "debug", loadedConfig.Log.Level)
	assert.Equal(t, log.DebugLevel, log.GetLevel())
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output.String())), &record))
	assert.Equal(t, "configured-from-file", record["msg"])
}

func TestPersistentPreRun_CLILevelOverridesConfig(t *testing.T) {
	resetRootTestState(t)
	configPath = writeRootTestConfig(t, "warn", "text")
	logLevel = "debug"
	noUpdateCheck = false

	err := persistentPreRun(rootCmd, nil)

	require.NoError(t, err)
	assert.Equal(t, log.DebugLevel, log.GetLevel())
	assert.Equal(t, "warn", loadedConfig.Log.Level)
}

func TestRemovedTransferAndTowerFlags(t *testing.T) {
	for _, flag := range []string{"skip-tower-sync", "skip-history-transfer"} {
		require.Nil(t, runCmd.Flags().Lookup(flag))
		require.ErrorContains(t, runCmd.ParseFlags([]string{"--" + flag}), "unknown flag")
	}
	runCmd.Flags().VisitAll(func(flag *pflag.Flag) { require.NotContains(t, flag.Name, "tower") })
}
