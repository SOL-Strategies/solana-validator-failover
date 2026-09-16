package failover

import (
	"testing"

	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/stretchr/testify/require"
)

func TestRenderIdentityCommandDirectionalNativeFields(t *testing.T) {
	template := `cmd {{ if not .FromNodeIsNativeFiredancer }}--require-tower{{ end }} {{ .HandoffStrategy }} {{ .ToNodeClientFamily }}`
	data := CommandTemplateData{
		FromNodeIsNativeFiredancer:      true,
		HandoffStrategy:                 HandoffStrategyOnchain,
		ToNodeClientFamily:              "agave",
		TowerFileAvailableAtDestination: false,
	}
	data.TowerFileWillBeTransferred = false
	got, err := RenderIdentityCommand(template, data)
	require.NoError(t, err)
	require.Equal(t, "cmd  onchain-reconcile agave", got)
}

func TestRenderIdentityCommandLegacyTowerFlag(t *testing.T) {
	template := `cmd{{ if .TowerFileAvailableAtDestination }} --require-tower{{ end }}`
	data := CommandTemplateData{TowerFileWillBeTransferred: true, TowerFileAvailableAtDestination: true}
	got, err := RenderIdentityCommand(template, data)
	require.NoError(t, err)
	require.Equal(t, "cmd --require-tower", got)
}

func TestRenderIdentityCommandSupportsNeqAlias(t *testing.T) {
	got, err := RenderIdentityCommand(`{{ if neq .FromNodeClientFamily "firedancer" }}--require-tower{{ end }}`, CommandTemplateData{FromNodeClientFamily: "agave"})
	require.NoError(t, err)
	require.Equal(t, "--require-tower", got)
}

func TestRenderIdentityCommandInvalidTemplate(t *testing.T) {
	_, err := RenderIdentityCommand(`{{ if .Missing }}`, CommandTemplateData{})
	require.Error(t, err)
}

func TestRenderIdentityCommandPreservesHostnameAlias(t *testing.T) {
	ids := &identities.Identities{Active: &identities.Identity{}, Passive: &identities.Identity{}}
	data := NewCommandTemplateData(NodeInfo{Hostname: "validator-a", Identities: ids}, NodeInfo{}, NodeInfo{Identities: ids}, NodeInfo{Identities: ids}, HandoffStrategyTowerFile, true, false)
	got, err := RenderIdentityCommand("{{ .Hostname }}", data)
	require.NoError(t, err)
	require.Equal(t, "validator-a", got)
}

func TestTowerTransferHonoursSkipTowerSync(t *testing.T) {
	s := &Stream{}
	s.SetHandoffStrategy(HandoffStrategyTowerFile)
	if !s.GetTowerFileWillBeTransferred() {
		t.Fatal("expected tower transfer for legacy handoff")
	}
	s.SetSkipTowerSync(true)
	if s.GetTowerFileWillBeTransferred() {
		t.Fatal("skip-tower-sync must disable transfer")
	}
	s.SetSkipTowerSync(false)
	s.SetHandoffStrategy(HandoffStrategyOnchain)
	if s.GetTowerFileWillBeTransferred() {
		t.Fatal("on-chain handoff must not transfer a tower")
	}
	// Verify setter order is also safe.
	s.SetHandoffStrategy(HandoffStrategyTowerFile)
	s.SetSkipTowerSync(true)
	if s.GetTowerFileWillBeTransferred() {
		t.Fatal("skip-tower-sync must win regardless of setter order")
	}
}

func TestRenderNegotiatedRollbackUsesNegotiatedIdentityFallbacks(t *testing.T) {
	ids := &identities.Identities{Active: &identities.Identity{}, Passive: &identities.Identity{}}
	active, passive, err := renderNegotiatedRollbackCommands(
		NodeInfo{ClientFamily: "agave", Identities: ids, SetIdentityActiveCommandTemplate: "active {{ .HandoffStrategy }} {{ .ToNodeClientFamily }}"},
		NodeInfo{ClientFamily: "firedancer", Identities: ids, SetIdentityPassiveCommandTemplate: "passive {{ .HandoffStrategy }} {{ .FromNodeClientFamily }}"},
		"passive-node-rollback", HandoffStrategyOnchain, false, false,
	)
	require.NoError(t, err)
	require.Equal(t, "active onchain-reconcile firedancer", active)
	require.Equal(t, "passive onchain-reconcile agave", passive)
}

func TestRenderNegotiatedRollbackKeepsSourceTowerSafetyWhenSkippingTransfer(t *testing.T) {
	ids := &identities.Identities{Active: &identities.Identity{}, Passive: &identities.Identity{}}
	active, _, err := renderNegotiatedRollbackCommands(
		NodeInfo{
			ClientFamily:                     "agave",
			Identities:                       ids,
			TowerFileSizeBytes:               128,
			SetIdentityActiveCommandTemplate: "active{{ if .TowerFileAvailableAtDestination }} --require-tower{{ end }}",
		},
		NodeInfo{ClientFamily: "agave", Identities: ids},
		"", HandoffStrategyTowerFile, false, false,
	)
	require.NoError(t, err)
	require.Equal(t, "active --require-tower", active)
}
