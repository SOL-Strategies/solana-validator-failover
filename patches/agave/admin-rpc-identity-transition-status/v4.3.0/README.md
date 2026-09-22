# Agave 4.3.x

The `0001-add-identity-transition-status-rpc.patch` file is generated against
Agave `v4.3.0-rc.1` (commit `2e10d67f909c23a9586e1b3d099f7b8ef810ccfe`). Apply
it from the root of that source tree with `git apply --check`, then `git apply`.

The patch adds the optional `identityTransitionStatus` JSON-RPC method. Tower
watermarks are published by ReplayStage only after its voting loop observes
the identity switch; Alpenglow watermarks are published by Votor after it
processes the corresponding identity event. The patch has been compile-checked
with Agave's pinned Rust toolchain, but operators should still test their build
before enabling fast Agave-derived to native Firedancer handoffs.

