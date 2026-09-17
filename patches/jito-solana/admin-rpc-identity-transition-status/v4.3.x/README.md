# Jito-Solana 4.3.x

The `0001-add-identity-transition-status-rpc.patch` file is generated against
Jito-Solana `v4.3.0-rc.1-jito.1` (commit `e4e422ad313c2b9f98f08346d3757afbe0627d56`).
Apply it from the root of that source tree with `git apply --check`, then
`git apply`.

This is a Jito-specific patch. It adds the optional `identityTransitionStatus`
JSON-RPC method and publishes the final old-identity vote watermark only after
the voting loop processes the identity transition.
