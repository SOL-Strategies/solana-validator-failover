// plan-preview renders the failover plan template with mock data so you can
// iterate on the template without running a real or dry-run failover.
//
// Usage:
//
//	go run ./cmd/plan-preview              # dry run, no hooks, with tower sync
//	go run ./cmd/plan-preview --hooks      # include example pre/post hooks
//	go run ./cmd/plan-preview --skip-tower # omit tower file sync step
//	go run ./cmd/plan-preview --real       # render as a real failover (not dry run)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sol-strategies/solana-validator-failover/internal/failover"
	"github.com/sol-strategies/solana-validator-failover/internal/hooks"
	"github.com/sol-strategies/solana-validator-failover/internal/identities"
	"github.com/sol-strategies/solana-validator-failover/internal/style"
)

func main() {
	withHooks := flag.Bool("hooks", false, "include example pre/post hooks")
	skipTower := flag.Bool("skip-tower", false, "skip tower file sync step")
	real := flag.Bool("real", false, "render as a real failover (not a dry run)")
	flag.Parse()

	activeNode := failover.NodeInfo{
		Hostname:           "sol-validator-1",
		PublicIP:           "203.0.113.10",
		ClientVersion:      "2.1.14",
		SetIdentityCommand: "agave-validator --ledger /mnt/ledger set-identity /home/solana/passive-1-identity.json",
		TowerFile:          "/mnt/accounts/tower/tower-1_9-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		TowerFileSizeBytes: 121856,
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-1-identity.json", PubKeyStr: "PassV1Kq8YxZd3NvQ7eLmT4bF9wR2cUjHnXsAoPiGkEy"},
		},
	}

	passiveNode := failover.NodeInfo{
		Hostname:           "sol-validator-2",
		PublicIP:           "203.0.113.20",
		ClientVersion:      "2.1.14",
		SetIdentityCommand: "agave-validator --ledger /mnt/ledger set-identity /home/solana/active-identity.json --require-tower",
		TowerFile:          "/mnt/accounts/tower/tower-1_9-456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM.bin",
		Identities: &identities.Identities{
			Active:  &identities.Identity{KeyFile: "/home/solana/active-identity.json", PubKeyStr: "456bAij7ryiCALQcYx4n47uUdop5d18camjTcedppAbM"},
			Passive: &identities.Identity{KeyFile: "/home/solana/passive-2-identity.json", PubKeyStr: "PassV2Lp9ZyXe4OwR8fMuS5cG1vT3dVkInBqHjNrEaFb"},
		},
	}

	var exampleHooks hooks.FailoverHooks
	if *withHooks {
		exampleHooks = hooks.FailoverHooks{
			Pre: hooks.PreHooks{
				WhenActive: hooks.Hooks{
					{Name: "notify-slack", Command: "curl", Args: []string{"-X", "POST", "https://hooks.slack.com/pre-active"}, MustSucceed: true},
				},
				WhenPassive: hooks.Hooks{
					{Name: "prepare-env", Command: "/usr/local/bin/prepare-takeover.sh", MustSucceed: false},
				},
			},
			Post: hooks.PostHooks{
				WhenActive: hooks.Hooks{
					{Name: "restart-monitoring", Command: "/usr/local/bin/restart-monitoring.sh"},
				},
				WhenPassive: hooks.Hooks{
					{Name: "alert-team", Command: "curl", Args: []string{"-X", "POST", "https://hooks.slack.com/post-passive"}},
				},
			},
		}
	}

	data := failover.PlanData{
		IsDryRun:        !*real,
		SkipTowerSync:   *skipTower,
		ActiveNodeInfo:  activeNode,
		PassiveNodeInfo: passiveNode,
		AppVersion:      "dev",
		Hooks:           exampleHooks,
	}

	rendered, err := failover.RenderFailoverPlan(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error rendering plan: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(style.RenderMessageString(rendered))
}
