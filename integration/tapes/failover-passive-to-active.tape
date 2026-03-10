# failover-passive-to-active.tape
# VHS tape — records london (passive) receiving a dry-run failover from chicago (active).
# Shows: waiting for connection, failover plan, interactive confirmation, execution, summary.
# Output: docs/failover-passive-to-active.gif
#
# Prerequisites (run once before recording):
#   make build    # build the binary
#   make demo     # start mock-solana on localhost:8899

Output docs/failover-passive-to-active.gif

Set Shell "zsh"
Set FontSize 13
Set Width 1600
Set Height 900
Set PlaybackSpeed 1

Hide
Sleep 300ms
# Reset mock state: chicago starts as active
Type `curl -sf -X POST -H 'Content-Type: application/json' -d '{"action":"reset","target":"chicago"}' http://localhost:8899/action >/dev/null 2>&1`
Enter
Sleep 300ms
# Start chicago in background (dry run — no --not-a-drill)
Type `(sleep 3 && COLORTERM=truecolor TERM=xterm-256color ./bin/solana-validator-failover-dev-linux-amd64 run --config integration/configs/demo-chicago.yaml --to-peer london --yes) >/dev/null 2>&1 & disown`
Enter
Sleep 1s
Show

Ctrl+L
Sleep 300ms

# Run london (passive, dry run — no --not-a-drill, no --yes so the prompt is shown)
Type `./bin/solana-validator-failover-dev-linux-amd64 run --config integration/configs/demo-london.yaml`
Enter

# Timeline (real time):
#   0s    london starts QUIC server: "Listening on port 9898..."
#   3s    chicago connects: plan displayed + confirmation prompt appears
#   5s    user confirms Yes (Left Arrow selects Yes, Enter submits)
#   5–8s  dry-run executes: set-identity skipped, summary shown

# Wait for the plan to appear (chicago connects at ~3s)
Sleep 5s

# Confirm: huh prompt defaults to No — Left moves to Yes, Enter submits
Left
Sleep 500ms
Enter

# Wait for dry-run execution and summary
Sleep 10s
