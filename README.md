# solana-validator-failover

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Simple p2p Solana validator failovers. This tool helps automate *planned* failvoers. To automate *unexpected* failovers, see [solana-validator-ha](https://github.com/SOL-Strategies/solana-validator-ha).

![solanna-validator-failover-passive-to-active-png](vhs/failover-passive-to-active.png)

A simple QUIC-based program to failover between Solana validators safely and quickly. [This post](https://blog.solstrategies.io/quic-solana-validator-failovers-738d712ac737) explains some background in more detail. In summary this program orchestrates the three-step process of failing over from an active (voting) to a passive (non-voting) validator:

1. active validator sets identity to passive
2. tower file synced from active to passive validator
3. passive validator sets identity to active

Start a failover server on the passive node:

![solanna-validator-failover-passive-to-active](vhs/failover-passive-to-active.gif)

Start a failover client on the active node to hand over to the passive node:

![solanna-validator-failover-active-to-passive](vhs/failover-active-to-passive.gif)

Convenience safety checks, bells, and whistles:

- Check and wait for validator health before failing over
- Wait for the estimated best slot time to failover
- Wait for no leader slots in the near future (if things go sideways - make it hurt a little less by not being leader 😬)
- Post-failover vote credit rank monitoring
- Pre/post failover hooks
- Customizable validator client and set identity commands to support (most) any validator client

## Usage

```shell
# on any node declared in solana-validator-failover.yaml
# run the failover - a passive node will send a request to the active one to take over
# By default it runs in dry-run mode, to run for real, run on the passive node with `--not-a-drill`
solana-validator-failover run
```

By default, `run` runs in dry-run mode where only the tower file is synced between nodes and set identity commands are mocked. This is to safeguard against fat fingers (we've all been there) and also to give an idea of the expected total failover time under current network conditions. When ready, re-run on the passive node with `--not-a-drill` to do it for realsies.

⚠️ WARNING: _who_ you run this program as matters - the user:
- requires permissions to run set identity commands for the validator
- requires permissions to read/write the tower file - check inherited tower file permissions are what you expect after a dry-run

## Installation

Build from source or download the built package for your system from the [releases](https://github.com/SOL-Strategies/solana-validator-failover/releases) page. If your arch isn't listed, ping us.

## Prerequisites

1. A (_preferrably private_ and low-latency) UDP route between active and passive validators. Latency can vary lots across setups, so YMMV, though QUIC should give a good head start.
2. Some focus and appreciation of what you're doing - these can be high pucker factor operations regardless of tooling.

## Configuration

```yaml
# default --config=~/solana-validator-failover/solana-validator-failover.yaml
validator:
  # path of validator program to use when issuing set-identity commands
  # default: agave-validator
  bin: agave-validator

  # (required) cluster this validator runs on
  #            one of: mainnet-beta, testnet, devnet, localnet
  cluster: mainnet-beta

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
  rpc_address: http://localhost:8899

  # tower file config
  tower:
    # (required) directory hosting the tower file
    dir: /mnt/accounts/tower

    # when passive, delete the towerfile if one exists before starting a failover server
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

    # golang template strings for command to set identity to active/passive
    # use this to set the appropriate command/args for your validator as required
    # available to this template will be:
    # {{ .Bin }}        - a resolved absolute path to the binary referenced in validator.bin
    # {{ .Identities }} - an object that has Active/Passive properties referencing
    #                     the loaded identities from validator.identities
    # {{ .LedgerDir }}  - a resolved absolute path to validator.ledger_dir
    # defaults shown below
    set_identity_active_cmd_template:  "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Active.KeyFile }} --require-tower"
    set_identity_passive_cmd_template: "{{ .Bin }} --ledger {{ .LedgerDir }} set-identity {{ .Identities.Passive.KeyFile }}"

    # failover peers - keys are vanity hostnames to help you review program output better
    peers:
      backup-validator-region-x:
        # host and port to connect to failover server
        address: backup-validator-region-x.some-private.zone:9898

    # duration string representing the minimum amount of time before the active node is due to
    # be the leader, if the failover is initiated below this threshold it will wait until this
    # window has passed to begin failing over
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
      # hooks to run after failover - errors in post hooks displayed but do nothing
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
```

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

## Laundry/wish list:

- [ ] TLS config
- [ ] Refactor to make e2e testing easier - current setup not optimal
- [ ] Rollbacks (to the extent it's possible)
