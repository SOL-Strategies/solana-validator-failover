package constants

import (
	"github.com/solana-foundation/solana-go/v2/rpc"
)

var (
	// SolanaClusters is a map of solana clusters to their rpc urls
	SolanaClusters = map[string]rpc.Cluster{
		rpc.MainNetBeta.Name: rpc.MainNetBeta,
		rpc.TestNet.Name:     rpc.TestNet,
		rpc.DevNet.Name:      rpc.DevNet,
		rpc.LocalNet.Name:    rpc.LocalNet,
	}

	// SolanaClusterNames is a list of solana cluster names
	SolanaClusterNames []string

	// NodeRolePassive is the role of a passive node
	NodeRolePassive = "passive"

	// NodeRoleActive is the role of an active node
	NodeRoleActive = "active"

	// ClientTypeAgave is the type of agave-validator client
	ClientTypeAgave = "agave"

	// ClientTypeFiredancer is the type of firedancer client
	ClientTypeFiredancer = "firedancer"
)

func init() {
	SolanaClusterNames = make([]string, 0, len(SolanaClusters))
	for name := range SolanaClusters {
		SolanaClusterNames = append(SolanaClusterNames, name)
	}
}
