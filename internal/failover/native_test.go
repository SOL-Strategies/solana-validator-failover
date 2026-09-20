package failover

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNativeTowerVoteSlot(t *testing.T) {
	metrics := `# HELP tower_vote_slot Highest voted slot in the local tower
tower_vote_slot{kind="tower",kind_id="0"} 442906160
tower_vote_slot{kind="other",kind_id="0"} 999999999
fd_voter_vote_slot 123
fd_tower_vote_slot 321
`
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(metrics))
	require.NoError(t, err)
	require.Equal(t, uint64(442906160), slot)
}

func TestParseNativeTowerVoteSlotPrefersOfficialMetric(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"tower_vote_slot{kind=\"tower\",kind_id=\"0\"} 424\n" +
			"fd_tower_vote_slot 483\n" +
			"fd_voter_vote_slot 500\n",
	))
	require.NoError(t, err)
	require.Equal(t, uint64(424), slot)
}

func TestParseNativeTowerVoteSlotIgnoresNoVoteSentinel(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"tower_vote_slot " + strconv.FormatUint(math.MaxUint64, 10) + "\n" +
			"tower_vote_slot 42\n"))
	require.NoError(t, err)
	require.Equal(t, uint64(42), slot)
}

func TestParseNativeTowerVoteSlotMissing(t *testing.T) {
	_, err := parseNativeTowerVoteSlot(strings.NewReader("other_metric 42\n"))
	require.Error(t, err)
}

func TestParseNativeTowerVoteSlotAcceptsPrometheusNumericFormats(t *testing.T) {
	slot, err := parseNativeTowerVoteSlot(strings.NewReader(
		"tower_vote_slot 456.0\n" +
			"tower_vote_slot 4.56e2\n"))
	if err != nil {
		t.Fatalf("expected numeric formats to parse: %v", err)
	}
	if slot != 456 {
		t.Fatalf("expected slot 456, got %d", slot)
	}
}

func TestParseNativeTowerVoteSlotRejectsNonIntegralSamples(t *testing.T) {
	_, err := parseNativeTowerVoteSlot(strings.NewReader("tower_vote_slot 456.5\n"))
	if err == nil {
		t.Fatal("expected non-integral sample to be rejected")
	}
}
