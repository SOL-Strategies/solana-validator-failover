# Agave/Jito 4.2.x

The `0001-add-identity-transition-status-rpc.patch` file is generated against
Agave `v4.2.2` (commit `c9c6f3287e26f24e3476e13e751aebe710191e89`). Apply it
from the root of that source tree with `git apply --check`, then `git apply`.

The patch adds the optional `identityTransitionStatus` JSON-RPC method. Tower
watermarks are published by ReplayStage only after its voting loop observes
the identity switch; Alpenglow watermarks are published by Votor after it
processes the corresponding identity event. The patch has been compile-checked
with Agave's pinned Rust toolchain, but operators should still test their build
before enabling fast Agave-derived to native Firedancer handoffs.

The same diff is intended for Jito-Solana builds that retain the Agave 4.2.x
RPC and validator layout; verify with `git apply --check` against the exact
Jito source revision first.
