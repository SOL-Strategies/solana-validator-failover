# Validator client patches

This directory hosts source patches for the optional Agave/Jito
`identityTransitionStatus` RPC used by fast Agave-derived to native Firedancer
handoffs.

The failover binary does not download or apply patches. It reports the matching
immutable patch URL when a validator does not expose the RPC. Operators should
apply the patch to the documented source revision, build the validator, and
verify the RPC before using it for a fast handoff.

## Available patch lines

- [Agave/Jito 4.2.x](agave/admin-rpc-identity-transition-status/v4.2.x/)
- [Agave/Jito 4.3.x](agave/admin-rpc-identity-transition-status/v4.3.x/)

Patch URLs are intended to be consumed from a tagged
`solana-validator-failover` release so they remain immutable.
