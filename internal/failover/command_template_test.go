package failover

import (
	"testing"

	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/stretchr/testify/require"
)

func TestRenderIdentityCommandDirectionalNativeFields(t *testing.T) {
	template := `cmd {{ if not .FromNodeIsNativeFiredancer }}--require-tower{{ end }} {{ .HandoffStrategy }} {{ .ToNodeClientFamily }}`
	data := CommandTemplateData{
		FromNodeIsNativeFiredancer: true,
		HandoffStrategy:            HandoffStrategyVoteHistory,
		ToNodeClientFamily:         "agave",
	}
	data.VoteHistoryWillBeTransferred = false
	got, err := RenderIdentityCommand(template, data)
	require.NoError(t, err)
	require.Equal(t, "cmd  vote-history agave", got)
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
	data := NewCommandTemplateData(NodeInfo{Hostname: "validator-a", Identities: ids}, NodeInfo{}, NodeInfo{Identities: ids}, NodeInfo{Identities: ids}, HandoffStrategyVoteHistory, true, false)
	got, err := RenderIdentityCommand("{{ .Hostname }}", data)
	require.NoError(t, err)
	require.Equal(t, "validator-a", got)
}

func TestHistoryTransferHonoursSkipHistoryTransfer(t *testing.T) {
	s := &Stream{}
	s.SetHandoffStrategy(HandoffStrategyVoteHistory)
	if !s.GetVoteHistoryWillBeTransferred() {
		t.Fatal("expected history transfer")
	}
	s.SetSkipHistoryTransfer(true)
	if s.GetVoteHistoryWillBeTransferred() {
		t.Fatal("skip-history-transfer must disable transfer")
	}
	s.SetSkipHistoryTransfer(false)
	s.SetHandoffStrategy(HandoffStrategyVoteHistory)
	if !s.GetVoteHistoryWillBeTransferred() {
		t.Fatal("expected history transfer")
	}
	// Verify setter order is also safe.
	s.SetHandoffStrategy(HandoffStrategyVoteHistory)
	s.SetSkipHistoryTransfer(true)
	if s.GetVoteHistoryWillBeTransferred() {
		t.Fatal("skip-history-transfer must win regardless of setter order")
	}
}

func TestRenderNegotiatedRollbackUsesNegotiatedIdentityFallbacks(t *testing.T) {
	ids := &identities.Identities{Active: &identities.Identity{}, Passive: &identities.Identity{}}
	active, passive, err := renderNegotiatedRollbackCommands(
		NodeInfo{ClientFamily: "agave", Identities: ids, SetIdentityActiveCommandTemplate: "active {{ .HandoffStrategy }} {{ .ToNodeClientFamily }}"},
		NodeInfo{ClientFamily: "firedancer", Identities: ids, SetIdentityPassiveCommandTemplate: "passive {{ .HandoffStrategy }} {{ .FromNodeClientFamily }}"},
		"passive-node-rollback", HandoffStrategyVoteHistory, false, false,
	)
	require.NoError(t, err)
	require.Equal(t, "active vote-history firedancer", active)
	require.Equal(t, "passive vote-history agave", passive)
}
