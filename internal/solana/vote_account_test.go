package solana

import (
	"context"
	"errors"
	"testing"

	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResolveVoteAccount(t *testing.T) {
	identity := solanago.NewWallet().PublicKey()
	vote := solanago.NewWallet().PublicKey()
	account := rpc.VoteAccountsResult{VotePubkey: vote, NodePubkey: identity}
	for _, tc := range []struct {
		name       string
		accounts   *rpc.GetVoteAccountsResult
		configured string
		rpcErr     error
		wantErr    bool
	}{
		{name: "current", accounts: &rpc.GetVoteAccountsResult{Current: []rpc.VoteAccountsResult{account}}},
		{name: "delinquent", accounts: &rpc.GetVoteAccountsResult{Delinquent: []rpc.VoteAccountsResult{account}}},
		{name: "ambiguous", accounts: &rpc.GetVoteAccountsResult{Current: []rpc.VoteAccountsResult{account, account}}, wantErr: true},
		{name: "missing", accounts: &rpc.GetVoteAccountsResult{}, wantErr: true},
		{name: "explicit", accounts: &rpc.GetVoteAccountsResult{Current: []rpc.VoteAccountsResult{account}}, configured: vote.String()},
		{name: "wrong identity", accounts: &rpc.GetVoteAccountsResult{Current: []rpc.VoteAccountsResult{{VotePubkey: vote}}}, configured: vote.String(), wantErr: true},
		{name: "RPC unavailable", rpcErr: errors.New("unavailable"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _, network := createTestClient()
			network.On("GetVoteAccounts", mock.Anything, mock.MatchedBy(func(opts *rpc.GetVoteAccountsOpts) bool { return opts.Commitment == rpc.CommitmentFinalized })).Return(tc.accounts, tc.rpcErr)
			got, err := client.ResolveVoteAccount(context.Background(), identity.String(), tc.configured)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, vote, got.VotePubkey)
			}
		})
	}
}
func TestVotingAccountRPCFallback(t *testing.T) {
	client, _, primary := createTestClient()
	secondary := &MockRPCClient{}
	vote := solanago.NewWallet().PublicKey()
	primary.On("GetVoteAccounts", mock.Anything, mock.Anything).Return((*rpc.GetVoteAccountsResult)(nil), errors.New("unavailable"))
	secondary.On("GetVoteAccounts", mock.Anything, mock.Anything).Return(&rpc.GetVoteAccountsResult{Current: []rpc.VoteAccountsResult{{VotePubkey: vote, LastVote: 123}}}, nil)
	client.networkRPCClients = []RPCClientInterface{primary, secondary}
	account, err := client.GetVoteAccountState(context.Background(), vote.String(), false, rpc.CommitmentFinalized)
	require.NoError(t, err)
	require.Equal(t, uint64(123), account.LastVote)
}
