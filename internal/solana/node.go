package solana

import (
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gagliardetto/solana-go/rpc"
)

// Node represents a gossip node
type Node struct {
	gossipNode *rpc.GetClusterNodesResult
}

// IP returns the IP address of the gossip node
func (n *Node) IP() string {
	return strings.Split(*n.gossipNode.Gossip, ":")[0]
}

// Pubkey returns the pubkey of the gossip node - prefer its PascalCase counterpart PubKey
func (n *Node) Pubkey() string {
	log.Warn("Pubkey is deprecated (but still works) in favour of PubKey - using it for you...")
	return n.PubKey()
}

// PubKey is the PascalCase counterpart of Pubkey - it's what we should have always used but let's be honest about why not:
// @coderigo messed up in the early README and claimed PubKey was supported when it was really Pubkey
func (n *Node) PubKey() string {
	return n.gossipNode.Pubkey.String()
}

// Version returns the version of the gossip node
func (n *Node) Version() string {
	return *n.gossipNode.Version
}

// Refresh refreshes the gossip node using the provided gossip client
func (n *Node) Refresh(gossipClient ClientInterface) error {
	refreshedNode, err := gossipClient.NodeFromIP(n.IP())
	if err != nil {
		return err
	}
	n.gossipNode = refreshedNode.gossipNode
	return nil
}
