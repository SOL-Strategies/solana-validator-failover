package failover

import (
	"fmt"
	"os"

	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/zeebo/xxh3"
)

// NodeInfo represents the information about a node that is needed to perform a failover
type NodeInfo struct {
	PublicIP                       string
	Hostname                       string
	Identities                     *identities.Identities
	TowerFile                      string
	TowerFileBytes                 []byte
	TowerFileHash                  string
	SetIdentityCommand             string
	ClientVersion                  string
	SolanaValidatorFailoverVersion string
	RPCAddress                     string
}

// SetTowerFileBytes sets the tower file bytes
func (n *NodeInfo) SetTowerFileBytes() error {
	towerFileBytes, err := os.ReadFile(n.TowerFile)
	if err != nil {
		return fmt.Errorf("failed to read tower file: %w", err)
	}
	n.TowerFileBytes = towerFileBytes
	n.setTowerFileHash()
	return nil
}

// SetTowerFileHash sets the tower file hash
func (n *NodeInfo) setTowerFileHash() {
	n.TowerFileHash = n.ComputeTowerFileHashFromBytes(n.TowerFileBytes)
}

// ComputeTowerFileHashFromBytes computes the tower file hash from the tower file bytes
func (n NodeInfo) ComputeTowerFileHashFromBytes(towerFileBytes []byte) string {
	hash := xxh3.Hash(towerFileBytes)
	return fmt.Sprintf("xxh3:%x", hash)
}
