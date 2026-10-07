# RBAC grant-state migration

## Why the representation changes

A structural node and a grant are different authorization states. Removing the
last child of a structural node must not turn that node into a grant covering
all its descendants.

`RBACPermissionElem.Grant` persists that distinction:

| Value | Meaning |
| --- | --- |
| `RBACGrantNone` (`"none"`) | Structural node; this node grants nothing. Children may grant. |
| `RBACGrantSubtree` (`"subtree"`) | This node grants its full key and descendants. A key ending in `.*` grants descendants only. |
| Empty, omitted | Legacy input only: a leaf is a grant and an internal node is structural. |

Use explicit modes for new policy definitions. `NewPermissionTree` now returns
an explicit non-granting root. To deliberately grant all root permissions:

```go
p := NewPermissionTree()
p.Grant = RBACGrantSubtree
```

`FillDefault` and JSON decoding materialize legacy state. Mutations capture
legacy state before changing children. Never remove a child's slice manually
and expect that to constitute a secure permission revocation.

## Restriction guarantees

Let `S(T)` be the keys accepted by `T.HasPerm2` (excluding the empty required
key, which deliberately requires no permission).

- `S(A.Intersection(B))` is a subset of both original inputs.
- `S(A.OverwriteBy(B, true))` is the same semantic intersection; display titles
  come from matching nodes in B.
- `S(A.Cut(k))` is a subset of the original A, and overlaps with k are revoked.

For ordinary well-formed hierarchical trees, the intersection is exact. It
supports genuine ancestor grants, descendant-only wildcard grants, and disjoint
sets. Common structural nodes can remain empty without granting anything.
Malformed out-of-root FullKey values are conservatively dropped.

The canonical grant-set comparison uses full permission identities, not only
local child keys. `OverwriteBy(..., false)` updates titles, not FullKey or grant
state. Both intersection operations snapshot the other input before locking the
receiver and share one semantic implementation.

## Cut and broad grants

This is a positive-grant model; it has no deny exceptions and no exact-only
(non-descendant) grant. It cannot express "all of root.sys except root.sys.read".

When `Cut("root.sys.read")` intersects a broad `root.sys` grant, the entire broad
grant is revoked. Independently represented, unaffected grants remain. This can
remove more access than requested, but cannot silently leave the revoked access
active. The same conservative rule applies to wildcard cuts. The root object
remains, but its authorization can be revoked. A bare `*` means the receiver's
descendants.

Applications needing exact subtraction from broad grants need a separately
designed exclusion representation. Do not recreate broad parents to work around
over-revocation.

## Persistence and compatibility

Clone, JSON, and SQL Value/Scan preserve explicit grant state. An empty structural
node must serialize with `"grant":"none"`. A reused JSON/SQL receiver is decoded
into fresh state before replacement; absent legacy fields cannot retain an old
broad grant or old children. Failed decoding leaves the previous state intact.

**Do not mix old authorization readers with new persisted trees.** Older readers
ignore the new field and still treat an empty parent as a grant. Upgrade all
authorization evaluators and policy mutators together, or use a versioned storage
boundary that prevents older consumers from accepting the new representation.
Do not round-trip new trees through serializers that discard the grant field.

Already-persisted legacy leaves are intrinsically ambiguous: an intentional broad
grant and a parent emptied by the old bug have identical data. The new code cannot
recover intent from those records. Rebuild affected effective permissions from
trusted role assignments or another authoritative policy source, and invalidate
cached derived permission trees.

## Compatibility changes to review

1. A newly constructed empty tree denies access rather than implicitly granting root.
2. Cut can conservatively revoke a whole broad grant; cutting the receiver's own
   full key clears authorization without deleting the receiver object.
3. Semantic intersection can materialize a descendant branch from B when A has
   a broad grant, even if the branch did not previously exist as a node in A.
4. OverwriteBy changes titles only; full permission identities are not display metadata.
5. JSON now persists explicit modes, and decoding replaces omitted fields rather
   than merging authorization state into a reused object.
6. Legacy HasPerm still has its older structural matching semantics; it is not
   interchangeable with HasPerm2. This change does not redesign that deprecated API.

Before release, run the entire repository suite with the supported toolchain and
real JSON/error/logging dependencies, plus consumer policy/persistence tests.
