package failover

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sol-strategies/solana-validator-failover/internal/solana"
	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
	"github.com/stretchr/testify/require"
)

func TestVerifyVotingIgnoresOldDuplicateAndRegressingSlots(t *testing.T) {
	identity := solanago.NewWallet().PublicKey()
	vote := solanago.NewWallet().PublicKey()
	sequence := []uint64{99, 100, 103, 103, 102, 104}
	calls := 0
	client := solana.NewMockClient().WithGetSlotContext(func(ctx context.Context, c rpc.CommitmentType) (uint64, error) {
		require.Equal(t, rpc.CommitmentFinalized, c)
		return 98, nil
	}).WithGetVoteAccountState(func(pub string, local bool, c rpc.CommitmentType) (*rpc.VoteAccountsResult, error) {
		require.False(t, local)
		require.Equal(t, vote.String(), pub)
		require.Equal(t, rpc.CommitmentFinalized, c)
		slot := sequence[calls]
		calls++
		// Epoch-credit markers and reward amount resets must not affect participation.
		return &rpc.VoteAccountsResult{VotePubkey: vote, NodePubkey: identity, LastVote: slot, EpochCredits: [][]uint64{{^uint64(0), ^uint64(0), ^uint64(0)}}}, nil
	})
	result, err := verifyVoting(context.Background(), client, identity.String(), vote.String(), 100, VotingConfig{true, time.Second, time.Millisecond})
	require.NoError(t, err)
	require.Equal(t, "confirmed", result.Status)
	require.Equal(t, uint64(103), result.FirstSlot)
	require.Equal(t, uint64(104), result.LastSlot)
	require.Equal(t, 6, calls)
}
func TestVerifyVotingDisabledDoesNotQueryRPC(t *testing.T) {
	result, err := verifyVoting(context.Background(), nil, "", "", 100, VotingConfig{})
	require.NoError(t, err)
	require.Equal(t, "disabled", result.Status)
}
func TestVerifyVotingTimesOutAndRetriesRPC(t *testing.T) {
	identity := solanago.NewWallet().PublicKey()
	vote := solanago.NewWallet().PublicKey()
	for _, mode := range []string{"stalled", "unavailable", "missing"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			mock := solana.NewMockClient().WithGetCurrentSlot(func() (uint64, error) { return 100, nil }).WithGetVoteAccountState(func(string, bool, rpc.CommitmentType) (*rpc.VoteAccountsResult, error) {
				calls++
				switch mode {
				case "unavailable":
					return nil, errors.New("unavailable")
				case "missing":
					return nil, nil
				}
				return &rpc.VoteAccountsResult{VotePubkey: vote, NodePubkey: identity, LastVote: 101}, nil
			})
			result, err := verifyVoting(context.Background(), mock, identity.String(), vote.String(), 99, VotingConfig{true, 15 * time.Millisecond, time.Millisecond})
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, "unverified", result.Status)
			require.Greater(t, calls, 1)
		})
	}
}
func TestVerifyVotingIdentityMismatchAndCancellation(t *testing.T) {
	identity := solanago.NewWallet().PublicKey()
	vote := solanago.NewWallet().PublicKey()
	mock := solana.NewMockClient().WithGetVoteAccountState(func(string, bool, rpc.CommitmentType) (*rpc.VoteAccountsResult, error) {
		return &rpc.VoteAccountsResult{VotePubkey: vote}, nil
	})
	_, err := verifyVoting(context.Background(), mock, identity.String(), vote.String(), 0, VotingConfig{true, time.Second, time.Millisecond})
	require.ErrorContains(t, err, "identity changed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = verifyVoting(ctx, mock, identity.String(), vote.String(), 0, VotingConfig{true, time.Second, time.Millisecond})
	require.ErrorIs(t, err, context.Canceled)
}
func TestVerifyVotingBoundsRequestsAndRetriesInitialSlot(t *testing.T) {
	calls := 0
	mock := solana.NewMockClient().WithGetSlotContext(func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("transient")
		}
		<-ctx.Done()
		return 0, ctx.Err()
	})
	_, err := verifyVoting(context.Background(), mock, "", "", 0, VotingConfig{true, 15 * time.Millisecond, time.Millisecond})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 2, calls)
}
