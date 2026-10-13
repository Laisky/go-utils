# Versioned RBAC policy storage and coordinated migration

Do not restore pre-#33 permission-broadening mutation behavior or permissive
constructor defaults. Current authorization semantics remain unchanged.

`RBACPolicyV1{Root: policy}` is an explicit storage API, implementing JSON and SQL
Value/Scan. Its representation is `["go-utils/rbac-policy",1,{...explicit grants...}]`.
A top-level array is intentional: historical struct readers reject it instead of
ignoring an object `version` field and interpreting a deny leaf as a subtree grant.
Known version/marker, explicit grants, identity, structure, depth, node and byte
budgets are checked. Unknown versions and legacy objects are rejected with no
partial receiver update. Always abort authorization on a read error: keeping an
old receiver unchanged for transactional safety is not permission to use stale grants.

Legacy `RBACPermissionElem` JSON/Value serialization is deprecated and emits only
exactly representable pre-#33 objects, without a grant field. Structural internal
nodes and subtree-grant leaves retain old/new interoperability. Deny-only leaves
and granted internal nodes cannot be represented; serialization returns
`ErrRBACPolicyVersionRequired` rather than writing unsafe unversioned data.
Legacy object readers remain available for trusted migration inputs. Marshal a
permission pointer, not a copied mutex-bearing struct value: copies are rejected
rather than bypassing the safe writer. Duplicate JSON fields/sibling identities
and cyclic trees are rejected by the versioned codec. No API
silently changes the persisted format or detects/falls back between formats.

## Deployment sequence

1. Inventory policy writers, readers, cache paths and rollback versions. Determine
   authoritative intended grants; do not infer intent from an already widened policy.
2. Provision a separate versioned column/keyspace/table. Keep old instances off it.
   Deploy version-aware readers first, making parse/unknown-version errors deny access.
3. Convert authoritative policy trees to RBACPolicyV1 explicitly. Compare effective
   permissions against the approved baseline, including future descendants and wildcard
   boundaries. Require no added permissions; coordinate any intentional changes.
4. Stop old writers and switch the authorized readers/writers together to the new
   storage. Do not dual-write a new deny-only state into the old object column.
5. Roll back only to version-aware readers after new policies exist. Otherwise pause
   writes/access and perform a reviewed migration; never strip version/grant fields.

The library does not run a database migration or decide actor-to-policy authorization.
Consumers must coordinate rollout. Restoring unsafe grants is not a compatibility fix.
Regression tests independently model the pre-#33 reader, demonstrate privilege
expansion before repair, preserve legacy-representable round trips, and exercise
strict versioned reads and every existing restriction/identity regression.
