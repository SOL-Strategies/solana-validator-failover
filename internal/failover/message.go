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
	HandoffStrategy                  string
	TowerFileWillBeTransferred       bool
	IdentityTransitionRPCAvailable   bool
	SlotFallbackRequired             bool
	FallbackWaitSlots                uint64
	HandoffWarning                   string
	ProbeIdentityTransitionRPC       bool
	FrozenTowerSlot                  uint64
	ReconciliationComplete           bool
	ActiveRollbackCommand            string
	PassiveRollbackCommand           string
	RollbackRequired                 bool
	ActiveRollbackEnabled            bool
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
