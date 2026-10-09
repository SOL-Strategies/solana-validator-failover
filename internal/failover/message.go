package failover

import (
	"time"
)

// Message represents the message data that can be encoded/decoded
type Message struct {
	CanProceed                         bool
	ErrorMessage                       string
	ActiveNodeInfo                     NodeInfo
	PassiveNodeInfo                    NodeInfo
	IsDryRunFailover                   bool
	IsSuccessfullyCompleted            bool
	SkipHistoryTransfer                bool
	HandoffStrategy                    string
	VoteHistoryWillBeTransferred       bool
	HandoffAborted                     bool
	ActiveRollbackCommand              string
	PassiveRollbackCommand             string
	RollbackRequired                   bool
	ActiveRollbackEnabled              bool
	ActiveNodeSetIdentityStartTime     time.Time
	ActiveNodeSetIdentityEndTime       time.Time
	ActiveNodeSyncVoteHistoryStartTime time.Time
	ActiveNodeSyncVoteHistoryEndTime   time.Time
	PassiveNodeSetIdentityStartTime    time.Time
	PassiveNodeSetIdentityEndTime      time.Time
	PassiveNodeSyncVoteHistoryEndTime  time.Time
	FailoverStartSlot                  uint64
	FailoverEndSlot                    uint64
	// key is the identity pubkey
	CreditSamples CreditSamples
}
