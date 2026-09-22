# Validator client patches

This directory hosts source patches for the optional Agave/Jito
`identityTransitionStatus` RPC used by fast Agave-derived to native Firedancer
handoffs. Agave and Jito-Solana use separate patch series because their source
trees are not byte-for-byte compatible.

The failover binary does not download or apply patches. It reports the matching
immutable patch URL when a validator does not expose the RPC. Operators should
apply the patch to the documented source revision, build the validator, and
verify the RPC before using it for a fast handoff.

## Available patch lines

- [Agave 4.3.0](agave/admin-rpc-identity-transition-status/v4.3.0/)
- [Jito-Solana 4.3.0](jito-solana/admin-rpc-identity-transition-status/v4.3.0/)
- [Firedancer 26.09.3 identity-labelled tower watermark](firedancer/tower-vote-watermark/v26.09.3/)

Patch URLs are intended to be consumed from a tagged
`solana-validator-failover` release so they remain immutable.
