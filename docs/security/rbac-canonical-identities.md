# RBAC canonical identities (#111)

`FullKey` is a derived structural identity, not a second permission selector.
JSON and SQL `Scan` now validate the entire detached tree before replacing live
state. A nonempty descendant `full_key` must equal its parent identity plus its
single-segment `key`; omitted full keys are resolved structurally. Keys containing
`.` and malformed dotted identities are rejected. `Valid` checks the same
invariant, and authorization/mutation paths do not promote forged identities.

A standalone subtree may retain an anchored root such as key `sys` with full key
`root.sys`, preserving existing subtree persistence and round trips. This does not
authorize importing an arbitrary root: the embedding application must still decide
which root and grants a principal is permitted to submit. Missing-key legacy JSON
remains decodable when it supplies no identity, but is not valid authorization.

The deprecated `HasPerm` entrypoint also rejects forged directly constructed
trees; this follow-up was reproduced independently before the guard was added.

The bounded reproduction on `042437b6015f5ba8f185ef91b05b2bd3a5a420f3` imported a
`reader` child with `full_key=root.admin`; both JSON and SQL Scan incorrectly
allowed `root.admin.delete`. Tests now require an error, unchanged previous state,
and denial of forged direct/mutated grants, while legitimate anchored round trips
remain valid. Run `go test -race -run TestRBAC .`.

Before rollout, validate persisted policies and correct inconsistent paths from
an authoritative source. Do not automatically broaden or infer missing grants.
No database migration is required. Reverting this patch restores the unsafe import
behavior and requires an outer trusted-policy boundary.
