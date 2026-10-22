package failover

import (
	"context"
	"fmt"
	"time"

	"github.com/sol-strategies/solana-validator-failover/internal/solana"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// VotingConfig controls optional certificate participation verification.
type VotingConfig struct {
	Enabled      bool
	Timeout      time.Duration
	PollInterval time.Duration
}

// MonitorConfig holds post-failover monitoring configuration.
type MonitorConfig struct{ Voting VotingConfig }

// VotingResult distinguishes an identity handover from certified participation.
type VotingResult struct {
	Status      string
	VoteAccount string
	Watermark   uint64
	FirstSlot   uint64
	LastSlot    uint64
	Duration    time.Duration
}

// verifyVoting uses standard finalized cluster RPC. Under Alpenglow, lastVote
// advances when reward/finalization certificates include this vote account.
func verifyVoting(ctx context.Context, client solana.ClientInterface, identity, voteAccount string, watermark uint64, cfg VotingConfig) (result VotingResult, err error) {
	result = VotingResult{Status: "disabled", VoteAccount: voteAccount, Watermark: watermark}
	if !cfg.Enabled {
		return result, nil
	}
	if cfg.Timeout <= 0 || cfg.PollInterval <= 0 {
		return result, fmt.Errorf("invalid voting verification durations")
	}
	start := time.Now()
	defer func() { result.Duration = time.Since(start) }()
	result.Status = "unverified"
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	var lastErr error
	var captured bool
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("certified voting not confirmed: %w (last observation: %v)", err, lastErr)
		}
		if !captured {
			slot, err := client.GetCurrentSlotContextWithCommitment(ctx, rpc.CommitmentFinalized)
			if err != nil {
				lastErr = err
			} else {
				result.Watermark = max(result.Watermark, slot)
				captured = true
			}
		}
		if captured {
			account, err := client.GetVoteAccountState(ctx, voteAccount, false, rpc.CommitmentFinalized)
			switch {
			case err != nil:
				lastErr = err
			case account == nil:
				lastErr = fmt.Errorf("vote account not returned")
			case account.VotePubkey.String() != voteAccount || account.NodePubkey.String() != identity:
				return result, fmt.Errorf("vote account or node identity changed during voting verification")
			case account.LastVote > result.Watermark && account.LastVote > result.LastSlot:
				if result.FirstSlot == 0 {
					result.FirstSlot = account.LastVote
				} else {
					result.LastSlot = account.LastVote
					result.Status = "confirmed"
					return result, nil
				}
				result.LastSlot = account.LastVote
			default:
				lastErr = fmt.Errorf("lastVote=%d, need two advancing slots beyond %d", account.LastVote, result.Watermark)
			}
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("certified voting not confirmed: %w (last observation: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}
