package failover

import (
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
)

// NodeInfo represents the information about a node that is needed to perform a failover
type NodeInfo struct {
	Bin                               string
	LedgerDir                         string
	ClientConfigPath                  string
	PublicIP                          string
	Hostname                          string
	Identities                        *identities.Identities
	VoteHistoryFile                   string
	VoteHistoryImportFile             string
	VoteHistoryFileSizeBytes          int64
	SetIdentityCommand                string
	SetIdentityCommandTemplate        string
	SetIdentityActiveCommandTemplate  string
	SetIdentityPassiveCommandTemplate string
	RollbackToActiveCommandTemplate   string
	RollbackToPassiveCommandTemplate  string
	ClientVersion                     string
	ClientVersionRPC                  string
	ClientFamily                      string
	ConsensusMode                     string
	IsNativeFiredancer                bool
	VoteAccount                       string
	SolanaValidatorFailoverVersion    string
	RPCAddress                        string
	Consensus                         string
	ConsensusGenesisSlot              uint64
}

// HistoryDestinationFile is the path restored by the destination command.
func (n NodeInfo) HistoryDestinationFile() string {
	if n.IsNativeFiredancer {
		return n.VoteHistoryImportFile
	}
	return n.VoteHistoryFile
}
