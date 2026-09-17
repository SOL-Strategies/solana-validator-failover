# Jito-Solana 4.2.x

The `0001-add-identity-transition-status-rpc.patch` file is generated against
Jito-Solana `v4.2.2-jito` (commit `c110e4c607e33b1a3acc4af17cb624007a4539f0`).
Apply it from the root of that source tree with `git apply --check`, then
`git apply`.

This is a Jito-specific patch. It adds the optional `identityTransitionStatus`
JSON-RPC method and publishes the final old-identity vote watermark only after
the voting loop processes the identity transition.
