package failover

import (
	"time"
)

// Message represents the message data that can be encoded/decoded
type Message struct {
	CanProceed                       bool
	ErrorMessage                     string
	ActiveNodeInfo                   NodeInfo
	PassiveNodeInfo                  NodeInfo
	IsDryRunFailover                 bool
	IsSuccessfullyCompleted          bool
	SkipTowerSync                    bool
	RollbackRequired                 bool
	ActiveNodeSetIdentityStartTime   time.Time
	ActiveNodeSetIdentityEndTime     time.Time
	ActiveNodeSyncTowerFileStartTime time.Time
	ActiveNodeSyncTowerFileEndTime   time.Time
	PassiveNodeSetIdentityStartTime  time.Time
	PassiveNodeSetIdentityEndTime    time.Time
	PassiveNodeSyncTowerFileEndTime  time.Time
	FailoverStartSlot                uint64
	FailoverEndSlot                  uint64
	// key is the identity pubkey
	CreditSamples CreditSamples
}
