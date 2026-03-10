# failover-active-to-passive.tape
# VHS tape — records chicago (active) initiating a dry-run failover to london (passive).
# Shows: detecting active role, connecting, waiting for signal, dry-run execution, summary.
# Output: docs/failover-active-to-passive.gif
#
# Prerequisites (run once before recording):
#   make build    # build the binary
#   make demo     # start mock-solana on localhost:8899

Output docs/failover-active-to-passive.gif

Set Shell "zsh"
Set FontSize 13
Set Width 1600
Set Height 420
Set PlaybackSpeed 1

Hide
Sleep 300ms
# Reset mock state: chicago starts as active
Type `curl -sf -X POST -H 'Content-Type: application/json' -d '{"action":"reset","target":"chicago"}' http://localhost:8899/action >/dev/null 2>&1`
Enter
Sleep 300ms
# Start london (passive, dry run — no --not-a-drill, --yes auto-confirm) in background
Type `(./bin/solana-validator-failover-dev-linux-amd64 run --config integration/configs/demo-london.yaml --yes) >/dev/null 2>&1 & disown`
Enter
Sleep 1s

Ctrl+L
Sleep 300ms

# Run chicago (active) — --to-peer skips the peer selector
Type `./bin/solana-validator-failover-dev-linux-amd64 run --config integration/configs/demo-chicago.yaml --to-peer london`
Enter
Sleep 1s
Show

# Timeline (real time):
#   0s     health check, detects ACTIVE role
#   0–1s   connects to london's QUIC server
#   1s     "Connected to london, waiting for failover signal..."
#   1–3s   london auto-confirms (--yes) and signals chicago to proceed
#   3–8s   dry-run executes: set-identity skipped, summary shown

Sleep 10s
