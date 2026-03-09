// summary-preview renders the post-failover summary template with mock data so
// you can iterate on the template without running a real or dry-run failover.
//
// Usage:
//
//	go run ./cmd/summary-preview                 # real failover, with tower sync
//	go run ./cmd/summary-preview --dry-run       # render as a dry run
//	go run ./cmd/summary-preview --skip-tower    # omit tower file sync step
//	go run ./cmd/summary-preview --credits       # include vote credit rank data
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sol-strategies/solana-validator-failover/internal/failover"
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
)

func main() {
	isDryRun := flag.Bool("dry-run", false, "render as a dry run")
	skipTower := flag.Bool("skip-tower", false, "skip tower file sync step")
	withCredits := flag.Bool("credits", false, "include vote credit rank data")
	flag.Parse()

	// Mirror the mock nodes from plan-preview so the two tools stay consistent.
	origActiveNode := failover.NodeInfo{
		Hostname:       "sol-validator-1",
		PublicIP:       "203.0.113.10",
		ClientVersion:  "2.1.14",
		TowerFile:      "/mnt/accounts/tower/tower-1_9-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		TowerFileBytes: make([]byte, 12_345),
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-1-identity.json", PubKeyStr: "PassV1Kq8YxZd3NvQ7eLmT4bF9wR2cUjHnXsAoPiGkEy"},
		},
	}

	origPassiveNode := failover.NodeInfo{
		Hostname:      "sol-validator-2",
		PublicIP:      "203.0.113.20",
		ClientVersion: "2.1.14",
		TowerFile:     "/mnt/accounts/tower/tower-1_9-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-2-identity.json", PubKeyStr: "PassV2Lp9ZyXe4OwR8fMuS5cG1vT3dVkInBqHjNrEaFb"},
		},
	}

	data := failover.SummaryData{
		IsDryRun:      *isDryRun,
		SkipTowerSync: *skipTower,

		OrigActiveNode:  origActiveNode,
		OrigPassiveNode: origPassiveNode,

		OrigActiveSetIdentityDuration:  210 * time.Millisecond,
		TowerSyncDuration:              80 * time.Millisecond,
		TowerFileSizeBytes:             12_345,
		OrigPassiveSetIdentityDuration: 155 * time.Millisecond,
		TotalDuration:                  445 * time.Millisecond,

		FailoverStartSlot: 300_000_042,
		FailoverEndSlot:   300_000_044,
		SlotsDuration:     2,
	}

	if *withCredits {
		data.HasVoteRankData = true
		data.VoteRankDiff = 2
		data.VoteRankFirst = 45
		data.VoteRankLast = 43
	}

	rendered, err := failover.RenderFailoverSummary(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rendering summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("INF post-failover state:")
	fmt.Print(style.RenderMessageString(rendered))
}
