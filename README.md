# solana-validator-failover

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Simple p2p Solana validator failovers. This tool helps automate **planned** failovers. To automate **unexpected** failovers see [solana-validator-ha](https://github.com/SOL-Strategies/solana-validator-ha).

![solana-validator-failover](docs/failover.png)

A QUIC-based program that orchestrates safe, fast failovers between Solana validators. [This post](https://blog.solstrategies.io/quic-solana-validator-failovers-738d712ac737) covers the background in more detail. In summary, it coordinates three steps across both nodes:

1. Active validator sets identity to passive
2. Tower file synced from active to passive validator
3. Passive validator sets identity to active

Convenience safety checks, bells, and whistles:

- Check and wait for validator health before failing over
- Wait for the estimated best slot time to failover
- Wait for no leader slots in the near future (if things go sideways — make it hurt a little less by not being leader 😬)
- Post-failover vote credit rank monitoring
- Pre/post failover hooks
- Customizable validator client and set identity commands to support (most) any validator client

## How it works

Running `solana-validator-failover run` on either node **automatically detects the node's role** (active or passive) from gossip and does the right thing:

- **Passive node** → starts a QUIC server, waits for the active node to connect
- **Active node** → connects to the passive peer as a QUIC client and orchestrates the handover

**You run the command on both nodes.** Start the passive node first so it is listening when the active node connects.

![solana-validator-failover passive-to-active](docs/failover-passive-to-active.gif)

![solana-validator-failover active-to-passive](docs/failover-active-to-passive.gif)

## Usage

```shell
# 1. Run on the passive node first — starts a server waiting for the active node
solana-validator-failover run --not-a-drill

# 2. Run on the active node — connects to the passive peer and initiates the handover
solana-validator-failover run
```

By default, `run` executes in **dry-run mode**: the tower file is synced and all timings are recorded, but set-identity commands are not executed. This is useful for gauging failover speed under real network conditions without committing. Pass `--not-a-drill` on the **passive** node to execute for real.

> ⚠️ **Who you run this as matters.** The user must have:
> - Permission to run set-identity commands for the validator
> - Read/write permission on the tower file — verify inherited permissions after a dry-run

### Flags

#### `run` flags

| Flag                           | Default | Description                                                                                                                                                       |
| ------------------------------ | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--not-a-drill`                | `false` | Execute failover for real. Effective on the passive node; ignored on the active node.                                                                             |
| `--no-wait-for-healthy`        | `false` | Skip waiting for the node to report healthy at `<rpc_address>/health`.                                                                                            |
| `--no-min-time-to-leader-slot` | `false` | Skip waiting for the active node to have no leader slots in the next `min_time_to_leader_slot` window. Effective on the active node; ignored on the passive node. |
| `--skip-tower-sync`            | `false` | Skip syncing the tower file from active to passive. The passive node must not have an existing tower file.                                                        |
| `-y, --yes`                    | `false` | Skip all interactive confirmation prompts.                                                                                                                        |
| `--to-peer <name\|ip>`         | —       | When run on the active node, auto-select a peer by its configured name or IP address, skipping the interactive selector. Ignored on the passive node.             |

#### Persistent flags

| Flag                      | Default                                                      | Description                                   |
| ------------------------- | ------------------------------------------------------------ | --------------------------------------------- |
| `-c, --config <path>`     | `~/solana-validator-failover/solana-validator-failover.yaml` | Path to config file.                          |
| `-l, --log-level <level>` | `info`                                                       | Log level (`debug`, `info`, `warn`, `error`). |
| `-n, --no-update-check`   | `false`                                                      | Skip the startup update check. Overrides `update.check_on_startup` in the config file. |

### Peer selection

The active node **always prompts you to select a peer**, even when only one is configured. Use `--to-peer <name|ip>` to skip the prompt — useful for scripted or non-interactive failovers:

```shell
# Skip peer selection prompt by name
solana-validator-failover run --to-peer backup-validator-region-x

# Fully non-interactive (skip peer selection and all confirmation prompts)
solana-validator-failover run --to-peer backup-validator-region-x --yes
```

## Installation

### Download binary

Download and install the latest [release](https://github.com/SOL-Strategies/solana-validator-failover/releases) binary for your system.

### From source

1. **Clone the repository:**
   ```bash
   git clone https://github.com/sol-strategies/solana-validator-failover.git
   cd solana-validator-failover
   ```

2. **Build the application:**
   ```bash
   make build
   # or manually:
   go build -o bin/solana-validator-failover ./cmd/solanavalidatorfailover
   ```

3. **Copy the binary to where you need it:**
   ```bash
   cp ./bin/solana-validator-failover /usr/local/bin/solana-validator-failover
   ```

## Prerequisites

1. **A (preferably private) low-latency UDP route** between active and passive validators. Latency varies across setups, so YMMV, though QUIC should give a good head start.

2. **Some focus and appreciation of what you're doing** — these can be high pucker factor operations regardless of tooling.

3. **Local validator started with `--full-rpc-api`** — this tool calls `getClusterNodes` on the local RPC, which requires the validator to be started with the `--full-rpc-api` flag (Agave/Firedancer).

## Configuration

```yaml
# default --config=~/solana-validator-failover/solana-validator-failover.yaml
validator:
  # path of validator program to use when issuing set-identity commands
  # default: agave-validator
  bin: agave-validator

  # (required) cluster this validator runs on
  #            well-known clusters: mainnet-beta, testnet, devnet, localnet
  #            any other value is treated as a custom cluster (requires cluster_rpc_url)
  cluster: mainnet-beta

  # (required for custom clusters) RPC URL for the cluster - must support getClusterNodes.
  # For well-known clusters the built-in URL is used. For custom clusters, or if you need
  # to override the default (e.g. to use a private RPC), set this explicitly.
  # cluster_rpc_url: <solana_compatible_rpc_endpoint>

  # optional display name used in failover plans, logs, and hook templates
  # defaults to OS hostname if not set
  # name: london

  # average slot duration, used to estimate time to next leader slot
  # default: 400ms
  # average_slot_duration: 400ms

  # this validator's identities
  identities:
    # (required or active_pubkey) path to identity file to use when ACTIVE
    # when supplied with active_pubkey, active takes precedence
    active: /home/solana/active-validator-identity.json
    # (required or active) base58 encoded pubkey to use when ACTIVE
    # when supplied with active, active takes precedence
    active_pubkey: 111111ActivePubkey1111111111111111111111111
    # (required) path to identity file to use when PASSIVE
    # when supplied with passive_pubkey, passive takes precedence
    passive: /home/solana/passive-validator-identity.json
    # (required or passive) base58 encoded pubkey to use when PASSIVE
    # when supplied with passive, passive takes precedence
    passive_pubkey: 111111PassivePubkey1111111111111111111111111

  # (required) ledger directory made available to set-identity command templates
  ledger_dir: /mnt/ledger

  # local rpc address of node this program runs on
  # default: http://localhost:8899
  # note: the validator must be started with --full-rpc-api (required for getClusterNodes)
  rpc_address: http://localhost:8899

  # tower file config
  tower:
    # (required) directory hosting the tower file
    dir: /mnt/accounts/tower

    # when passive, delete the tower file if one exists before starting a failover server
    # default: false
    auto_empty_when_passive: false

    # golang template to identify the tower file within tower.dir
    # available to the template is an .Identities object
    # default: "tower-1_9-{{ .Identities.Active.PubKey }}.bin"
    file_name_template: "tower-1_9-{{ .Identities.Active.PubKey }}.bin"

  # failover configuration
  failover:
    # failover server config (runs on passive node taking over from active node)
    server:
      # default: 9898 - QUIC (udp) port to listen on
      port: 9898

    # (optional) mutual TLS for the QUIC connection between validators.
    # When disabled (the default), the connection uses an ephemeral self-signed
    # certificate — encrypted but unauthenticated.
    # When enabled, both nodes must present a certificate signed by the shared CA.
    #
    # Certificate requirements:
    # - ca_cert: the same CA certificate must be present on both nodes
    # - cert/key: each node's certificate must include a SAN matching the address
    #   used in failover.peers — an IP SAN if the address is an IP, a DNS SAN if a hostname
    #
    # Generating certs (example using openssl):
    #   # CA
    #   openssl ecparam -name prime256v1 -genkey -noout -out ca.key
    #   openssl req -new -x509 -key ca.key -out ca.crt -days 3650 -subj "/CN=failover-ca"
    #
    #   # Node cert with IP SAN (use DNS:hostname instead if peers use FQDNs)
    #   openssl ecparam -name prime256v1 -genkey -noout -out node.key
    #   openssl req -new -key node.key -out node.csr -subj "/CN=validator-node"
    #   openssl x509 -req -in node.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    #     -out node.crt -days 3650 -extfile <(printf "subjectAltName=IP:192.0.2.1")
    tls:
      # default: false
      enabled: false
      # path to the shared CA certificate (must be identical on both nodes)
      ca_cert: /etc/solana-failover/tls/ca.crt
      # path to this node's certificate (signed by ca_cert)
      cert: /etc/solana-failover/tls/node.crt
      # path to this node's private key
      key: /etc/solana-failover/tls/node.key

    # golang template strings for command to set identity to active/passive
    # use this to set the appropriate command/args for your validator as required
    # available to this template will be:
    # {{ .Bin }}        - a resolved absolute path to the binary referenced in validator.bin
    # {{ .Identities }} - an object that has Active/Passive properties referencing
    #                     the loaded identities from validator.identities
    # {{ .LedgerDir }}  - a resolved absolute path to validator.ledger_dir
    # defaults shown below
    set_identity_active_cmd_template: "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Active.KeyFile }} --require-tower"
    set_identity_passive_cmd_template: "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Passive.KeyFile }}"

    # failover peers - keys are vanity names shown in program output and usable with --to-peer
    # configure one peer per passive validator you may want to fail over to
    peers:
      backup-validator-region-x:
        # host and port to connect to failover server
        address: backup-validator-region-x.some-private.zone:9898

    # duration string representing the minimum amount of time before the active node is due to
    # be the leader; if the failover is initiated below this threshold it will wait until this
    # window has passed before connecting to the passive peer
    # default: 5m
    min_time_to_leader_slot: 5m

    # post-failover monitoring config
    monitor:
      # monitoring of credit rank pre and post failover
      credit_samples:
        # number of credit samples to take
        # default: 5
        count: 5
        # interval duration between samples
        # default: 5s
        interval: 5s

    # (optional) Hooks to run pre/post failover and when active or passive.
    # They will run sequentially in the order they are declared.
    #
    # Template interpolation is supported in command, args, and environment variable values using Go text/template syntax.
    # The template data structure provides access to failover state and node information (see template fields below).
    #
    # The specified command program will receive environment variables:
    # 1. Custom environment variables from the 'environment' map (if specified)
    # 2. Standard SOLANA_VALIDATOR_FAILOVER_* variables (set last, will override custom if duplicated there)
    #
    # Available template fields for interpolation in command, args, and environment values:
    # ------------------------------------------------------------------------------------------------------------
    # {{ .IsDryRunFailover }}                    - bool: true if this is a dry run failover
    # {{ .ThisNodeRole }}                        - string: "active" or "passive"
    # {{ .ThisNodeName }}                        - string: hostname of this node
    # {{ .ThisNodePublicIP }}                    - string: public IP of this node
    # {{ .ThisNodeActiveIdentityPubkey }}        - string: pubkey this node uses when active
    # {{ .ThisNodeActiveIdentityKeyFile }}       - string: path to keyfile from validator.identities.active
    # {{ .ThisNodePassiveIdentityPubkey }}       - string: pubkey this node uses when passive
    # {{ .ThisNodePassiveIdentityKeyFile }}      - string: path to keyfile from validator.identities.passive
    # {{ .ThisNodeClientVersion }}               - string: gossip-reported solana validator client semantic version for this node
    # {{ .ThisNodeRPCAddress }}                  - string: local validator RPC URL from config (validator.rpc_address)
    # {{ .PeerNodeRole }}                        - string: "active" or "passive"
    # {{ .PeerNodeName }}                        - string: hostname of peer node
    # {{ .PeerNodePublicIP }}                    - string: public IP of peer node
    # {{ .PeerNodeActiveIdentityPubkey }}        - string: pubkey peer uses when active
    # {{ .PeerNodePassiveIdentityPubkey }}       - string: pubkey peer uses when passive
    # {{ .PeerNodeClientVersion }}               - string: gossip-reported solana validator client semantic version for peer node
    #
    # Standard environment variables passed to hook commands (SOLANA_VALIDATOR_FAILOVER_*):
    # ------------------------------------------------------------------------------------------------------------
    # SOLANA_VALIDATOR_FAILOVER_IS_DRY_RUN_FAILOVER                     = "true|false"
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_ROLE                          = "active|passive"
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_NAME                          = hostname of this node
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_PUBLIC_IP                     = public IP of this node
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_ACTIVE_IDENTITY_PUBKEY        = pubkey this node uses when active
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_ACTIVE_IDENTITY_KEYPAIR_FILE  = path to keyfile from validator.identities.active
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_PASSIVE_IDENTITY_PUBKEY       = pubkey this node uses when passive
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_PASSIVE_IDENTITY_KEYPAIR_FILE = path to keyfile from validator.identities.passive
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_CLIENT_VERSION                = gossip-reported solana validator client semantic version for this node
    # SOLANA_VALIDATOR_FAILOVER_THIS_NODE_RPC_ADDRESS                   = local validator RPC URL from config (validator.rpc_address)
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_ROLE                          = "active|passive"
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_NAME                          = hostname of peer
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_PUBLIC_IP                     = public IP of peer
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_ACTIVE_IDENTITY_PUBKEY        = pubkey peer uses when active
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_PASSIVE_IDENTITY_PUBKEY       = pubkey peer uses when passive
    # SOLANA_VALIDATOR_FAILOVER_PEER_NODE_CLIENT_VERSION                = gossip-reported solana validator client semantic version for peer node
    hooks:
      # hooks to run before failover - errors in pre hooks optionally abort failover
      pre:
        # run before failover when validator is active
        when_active:
          - name: x # vanity name
            command: ./scripts/some_script.sh # command to run (supports template interpolation)
            args: ["--role={{ .ThisNodeRole }}", "{{ .ThisNodeName }}"] # args support template interpolation
            must_succeed: true # aborts failover on failure
            environment: # optional map of custom environment variables (values support template interpolation)
              MY_VAR: "{{ .ThisNodeName }}"
              PEER_IP: "{{ .PeerNodePublicIP }}"
        # run before failover when validator is passive
        when_passive:
          - name: x # vanity name
            command: ./scripts/some_script.sh # command to run (supports template interpolation)
            args: ["--role={{ .ThisNodeRole }}", "{{ .ThisNodeName }}"] # args support template interpolation
            must_succeed: true # aborts failover on failure
            environment: # optional map of custom environment variables (values support template interpolation)
              MY_VAR: "{{ .ThisNodeName }}"
              PEER_IP: "{{ .PeerNodePublicIP }}"
      # hooks to run after failover - errors in post hooks are displayed but do not affect the failover result
      post:
        # run after failover when validator is active
        when_active:
          - name: x # vanity name
            command: ./scripts/some_script.sh # command to run (supports template interpolation)
            args: ["--role={{ .ThisNodeRole }}", "{{ .ThisNodeName }}"] # args support template interpolation
            environment: # optional map of custom environment variables (values support template interpolation)
              MY_VAR: "{{ .ThisNodeName }}"
              PEER_IP: "{{ .PeerNodePublicIP }}"
        # run after failover when validator is passive
        when_passive:
          - name: x # vanity name
            command: ./scripts/some_script.sh # command to run (supports template interpolation)
            args: ["--role={{ .ThisNodeRole }}", "{{ .ThisNodeName }}"] # args support template interpolation
            environment: # optional map of custom environment variables (values support template interpolation)
              MY_VAR: "{{ .ThisNodeName }}"
              PEER_IP: "{{ .PeerNodePublicIP }}"
    # (optional) Automatic rollback configuration.
    # When enabled, if a failover fails after the active node has already switched to passive,
    # the passive node signals the active node to revert. Both nodes attempt to return to their
    # original roles.
    #
    # IMPORTANT LIMITATIONS — read before enabling:
    # - Rollback is only triggered by an explicit signal from the passive node. If the network
    #   connection drops after the passive node successfully sets its identity to active, no
    #   automatic rollback occurs (to prevent the risk of two active validators). The operator
    #   must check gossip and intervene manually.
    # - If the rollback itself fails, the cluster may still be left without an active leader.
    #   Rollback failures are logged at ERROR level with manual recovery commands.
    # - Both nodes must have rollback enabled and configured identically for coordination to work.
    # - Rollback hooks are always run (pre then post), even if the set-identity command fails.
    rollback:
      # default: false — opt-in
      enabled: false

      # Configuration for reverting the active node (which switched to passive) back to active.
      # Triggered when the passive node signals that it failed to become active.
      to_active:
        # Go template for the rollback set-identity command.
        # Supports the same template fields as set_identity_active_cmd_template.
        # When empty, defaults to set_identity_active_cmd_template.
        cmd_template: ""
        hooks:
          # run after the rollback set-identity command (always runs, even if cmd failed)
          post:
            - name: notify-rollback-to-active
              command: ./scripts/notify_rollback.sh
              args: ["to-active"]

      # Configuration for re-asserting the passive node's passive identity when it failed to
      # become active. Triggered on the passive node when set-identity-to-active fails.
      to_passive:
        # Go template for the rollback set-identity command.
        # Supports the same template fields as set_identity_passive_cmd_template.
        # When empty, defaults to set_identity_passive_cmd_template.
        cmd_template: ""
        hooks:
          # run after the rollback set-identity command (always runs, even if cmd failed)
          post:
            - name: notify-rollback-to-passive
              command: ./scripts/notify_rollback.sh
              args: ["to-passive"]

# update check configuration
update:
  # check for a new release on startup and print a warning if one is available
  # default: true
  # override with the --no-update-check CLI flag
  check_on_startup: true
```

## Rollback

`failover.rollback` is an opt-in feature that attempts to automatically revert both nodes to their original roles if a failover fails after identities have started changing.

### When it triggers

Rollback is only triggered by an **explicit signal** from the passive node. Specifically: after the active node has switched to passive and sent the tower file, if the passive node's `set-identity-to-active` command fails, it signals the active node to revert before exiting.

### What it does

| Node                                             | Rollback action                                                           |
| ------------------------------------------------ | ------------------------------------------------------------------------- |
| Active node (was active, became passive)         | Runs `set-identity-to-active` command → `rollback.to_active` post-hooks   |
| Passive node (tried and failed to become active) | Runs `set-identity-to-passive` command → `rollback.to_passive` post-hooks |

Post-hooks always run even if the set-identity command fails. Pre hooks are intentionally not supported for rollback: a pre hook with `must_succeed: true` could block the rollback set-identity command from running, which would defeat the purpose of rollback.

### Limitations

**No auto-rollback on connection drop.** If the network connection drops after the passive node *successfully* set its identity to active (but before the client received confirmation), the active node does **not** automatically rollback. Auto-rollback in this scenario would risk creating two active validators. Instead, a `CRITICAL` log is emitted with the manual recovery command, and the operator must check gossip to determine the actual cluster state.

**Rollback can itself fail.** If the rollback set-identity command fails, both nodes may still be passive. Rollback failures are logged at `ERROR` level with the manual recovery command. There is no retry — operators must intervene.

**Both nodes must be configured identically.** Rollback config is local to each node's config file. Both nodes must have `rollback.enabled: true` and the correct commands configured.

### Rollback is shown in the failover plan

When `rollback.enabled: true`, the pre-failover plan shows the rollback commands that would run on each node if the failover fails, giving operators visibility before they confirm.

## Developing

```shell
# build in docker with live-reload on file changes
make dev
```

## Building

```shell
# build locally
make build

# or build from docker
make build-compose
```
