package failover

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNativeTowerVoteSlot(t *testing.T) {
	metrics := `# HELP fd_voter_vote_slot Latest vote slot
fd_voter_vote_slot 123
fd_voter_vote_slot{identity="validator"} 456
fd_voter_vote_slot_count 999
fd_tower_vote_slot 321
`
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(metrics))
	require.NoError(t, err)
	require.Equal(t, uint64(456), slot)
}

func TestParseNativeTowerVoteSlotIgnoresNoVoteSentinel(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"fd_voter_vote_slot " + strconv.FormatUint(math.MaxUint64, 10) + "\n" +
			"fd_voter_vote_slot 42\n"))
	require.NoError(t, err)
	require.Equal(t, uint64(42), slot)
}

func TestParseNativeTowerVoteSlotMissing(t *testing.T) {
	_, err := parseNativeTowerVoteSlot(strings.NewReader("other_metric 42\n"))
	require.Error(t, err)
}

func TestParseNativeTowerVoteSlotAcceptsPrometheusNumericFormats(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"fd_voter_vote_slot 456.0\n" +
			"fd_voter_vote_slot 4.56e2\n"))
	if err != nil {
		t.Fatalf("expected numeric formats to parse: %v", err)
	}
	if slot != 456 {
		t.Fatalf("expected slot 456, got %d", slot)
	}
}

func TestParseNativeTowerVoteSlotPrefersVoterMetric(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"fd_voter_vote_slot 424\n" +
			"fd_tower_vote_slot 483\n" +
			"tower_vote_slot 500\n",
	))
	require.NoError(t, err)
	require.Equal(t, uint64(424), slot)
}

func TestParseNativeTowerVoteSlotRejectsNonIntegralSamples(t *testing.T) {
	_, err := parseNativeTowerVoteSlot(strings.NewReader("fd_voter_vote_slot 456.5\n"))
	if err == nil {
		t.Fatal("expected non-integral sample to be rejected")
	}
}
