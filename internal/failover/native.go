package failover

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ReadNativeTowerVoteSlot reads Firedancer's highest locally retained tower
// vote. This is deliberately separate from the RPC last-vote value: the
// latter only proves that a vote has landed on chain.
func ReadNativeTowerVoteSlot(ctx context.Context, address string) (uint64, error) {
	if address == "" {
		return 0, fmt.Errorf("native Firedancer metrics address is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(address, "/")+"/metrics", nil)
	if err != nil {
		return 0, fmt.Errorf("create Firedancer metrics request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("query Firedancer metrics: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("Firedancer metrics returned HTTP %d", resp.StatusCode)
	}
	return parseNativeTowerVoteSlot(resp.Body)
}

func parseNativeTowerVoteSlot(reader io.Reader) (uint64, error) {
	const noVote = math.MaxUint64
	// tower_vote_slot is Firedancer's current native tower watermark. The other
	// names are compatibility fallbacks for older or nonstandard builds; never
	// combine values from different metric families.
	metricNames := []string{
		"tower_vote_slot",
		"fd_tower_vote_slot",
		"fd_voter_vote_slot",
	}
	highestByMetric := make(map[string]uint64, len(metricNames))
	foundByMetric := make(map[string]bool, len(metricNames))
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		rawMetric := parts[0]
		metric := rawMetric
		labels := ""
		if brace := strings.IndexByte(metric, '{'); brace >= 0 {
			labels = metric[brace:]
			metric = metric[:brace]
		}
		validMetric := false
		for _, name := range metricNames {
			if metric == name {
				validMetric = true
				break
			}
		}
		if !validMetric {
			continue
		}
		if metric == "tower_vote_slot" && labels != "" && !strings.Contains(labels, `kind="tower"`) {
			continue
		}
		slot, parseErr := parsePrometheusUint(parts[1])
		if parseErr == nil && slot != noVote && (!foundByMetric[metric] || slot > highestByMetric[metric]) {
			highestByMetric[metric] = slot
			foundByMetric[metric] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read Firedancer metrics: %w", err)
	}
	for _, metric := range metricNames {
		if foundByMetric[metric] {
			return highestByMetric[metric], nil
		}
	}
	return 0, fmt.Errorf("Firedancer vote slot metric not found")
}

// parsePrometheusUint accepts the decimal and scientific formats permitted for
// Prometheus samples, but only returns non-negative integral values that fit in
// a Solana slot number.
func parsePrometheusUint(value string) (uint64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || math.Trunc(number) != number {
		return 0, fmt.Errorf("invalid non-negative integer sample %q", value)
	}
	// ParseFloat rounds values near the uint64 boundary. Reject 2^64 and above
	// explicitly before converting so an overflow cannot wrap or be accepted.
	if number >= math.Pow(2, 64) {
		return 0, fmt.Errorf("sample %q exceeds uint64", value)
	}
	return uint64(number), nil
}

func waitForNativeTowerVote(ctx context.Context, address string, poll time.Duration) (uint64, error) {
	return waitForNativeTowerVoteAtLeast(ctx, address, poll, 0)
}

func waitForNativeTowerVoteAtLeast(ctx context.Context, address string, poll time.Duration, minimum uint64) (uint64, error) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	for {
		slot, err := ReadNativeTowerVoteSlot(ctx, address)
		if err == nil && slot >= minimum {
			return slot, nil
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

// waitForNativeTowerVoteAtLeastStable adds a post-demotion observation
// barrier. Firedancer's metrics endpoint can briefly expose the pre-transition
// watermark while the identity switch settles, so one qualifying sample is
// not sufficient evidence of the frozen tower tip.
func waitForNativeTowerVoteAtLeastStable(ctx context.Context, address string, poll time.Duration, minimum uint64) (uint64, error) {
	return waitForNativeTowerVoteAtLeastStableWithProgress(ctx, address, poll, minimum, nil)
}

func waitForNativeTowerVoteAtLeastStableWithProgress(ctx context.Context, address string, poll time.Duration, minimum uint64, progress func(slot uint64, err error, stableSamples int)) (uint64, error) {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	var previous uint64
	havePrevious := false
	stableSamples := 0
	for {
		slot, err := ReadNativeTowerVoteSlot(ctx, address)
		if progress != nil {
			progress(slot, err, stableSamples)
		}
		if err == nil && slot >= minimum {
			if havePrevious && slot == previous {
				return slot, nil
			}
			previous = slot
			havePrevious = true
			stableSamples++
		} else {
			havePrevious = false
			stableSamples = 0
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}
