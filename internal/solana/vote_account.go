package solana

import (
	"context"
	"fmt"

	solanago "github.com/solana-foundation/solana-go/v2"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

// ResolveVoteAccount validates an explicit account or discovers a unique account
// for the active identity, including delinquent validators. No credits are ranked.
func (c *Client) ResolveVoteAccount(ctx context.Context, identity, configured string) (*rpc.VoteAccountsResult, error) {
	if _, err := solanago.PublicKeyFromBase58(identity); err != nil {
		return nil, err
	}
	if configured != "" {
		account, err := c.GetVoteAccountState(ctx, configured, false, rpc.CommitmentFinalized)
		if err != nil {
			return nil, err
		}
		if account.NodePubkey.String() != identity {
			return nil, fmt.Errorf("vote account %s does not belong to identity %s", configured, identity)
		}
		return account, nil
	}
	accounts, err := c.networkGetVoteAccounts(ctx, &rpc.GetVoteAccountsOpts{Commitment: rpc.CommitmentFinalized})
	if err != nil {
		return nil, err
	}
	if accounts == nil {
		return nil, fmt.Errorf("vote account RPC returned no result")
	}
	var matches []rpc.VoteAccountsResult
	for _, account := range append(accounts.Current, accounts.Delinquent...) {
		if account.NodePubkey.String() == identity {
			matches = append(matches, account)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("expected exactly one vote account for identity %s, found %d; configure validator.client.vote_account explicitly", identity, len(matches))
	}
	return &matches[0], nil
}
