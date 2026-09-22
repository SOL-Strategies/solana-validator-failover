# Firedancer 26.09.3 identity-labelled tower watermark

This patch targets the upstream Firedancer `v26.09.3` tag at commit
`a093ae71072a480a6bf90e89ee8382b2c7eab2f9`.

It adds the Prometheus metric:

```text
tower_vote_slot_by_identity{kind="tower",kind_id="0",identity="<base58 identity>"} <slot>
```

The metric retains the current and immediately previous identity. The previous
identity's value is captured when Firedancer processes the identity switch,
after signing is halted, and remains available after `set-identity` returns.

Apply and build:

```sh
git checkout v26.09.3
git apply 0001-add-identity-labelled-tower-watermark.patch
make -j
```

The failover client should query the series for the old active identity after
`set-identity` completes, then wait for that vote account's `lastVote` to reach
the captured slot at finalized commitment. The existing `tower_vote_slot`
metric is unchanged for compatibility.
