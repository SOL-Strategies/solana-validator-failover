// summary-preview renders the post-failover summary template with mock data so
// you can iterate on the template without running a real or dry-run failover.
//
// Usage:
//
//	go run ./cmd/summary-preview                 # real failover, with history transfer
//	go run ./cmd/summary-preview --dry-run       # render as a dry run
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
	flag.Parse()

	totalDuration := 445 * time.Millisecond

	// Mirror the mock nodes from plan-preview so the two tools stay consistent.
	origActiveNode := failover.NodeInfo{
		Hostname:                 "sol-validator-1",
		PublicIP:                 "203.0.113.10",
		ClientVersion:            "4.3.0",
		VoteHistoryFile:          "/mnt/ledger/vote_history-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		VoteHistoryFileSizeBytes: 12_345,
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-1-identity.json", PubKeyStr: "PassV1Kq8YxZd3NvQ7eLmT4bF9wR2cUjHnXsAoPiGkEy"},
		},
	}

	origPassiveNode := failover.NodeInfo{
		Hostname:        "sol-validator-2",
		PublicIP:        "203.0.113.20",
		ClientVersion:   "4.3.0",
		VoteHistoryFile: "/mnt/ledger/vote_history-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-2-identity.json", PubKeyStr: "PassV2Lp9ZyXe4OwR8fMuS5cG1vT3dVkInBqHjNrEaFb"},
		},
	}

	data := failover.SummaryData{
		SuccessfullyCompleted: true,
		Voting:                failover.VotingResult{Status: "confirmed", FirstSlot: 300000045, LastSlot: 300000047, Duration: time.Second},
		IsDryRun:              *isDryRun,

		OrigActiveNode:  origActiveNode,
		OrigPassiveNode: origPassiveNode,

		OrigActiveSetIdentityDuration:  210 * time.Millisecond,
		HistorySyncDuration:            80 * time.Millisecond,
		VoteHistoryFileSizeBytes:       12_345,
		OrigPassiveSetIdentityDuration: 155 * time.Millisecond,
		TotalDuration:                  totalDuration,

		FailoverStartSlot: 300_000_042,
		FailoverEndSlot:   300_000_044,
		SlotsDuration:     2,
	}

	if *isDryRun {
		data.Voting = failover.VotingResult{Status: "dry-run"}
	}
	rendered, err := failover.RenderFailoverSummary(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rendering summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("INF post-failover state:")
	fmt.Print(style.RenderMessageString(rendered))
}
