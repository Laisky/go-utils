# RBAC policy storage and coordinated migration

## Why a separate storage API is necessary

The historical pre-#33 reader inferred grants from the tree shape: a leaf granted
its subtree. The corrected authorization model separates structural nodes from
explicit `none` / `subtree` grants. A denied empty node cannot be represented in
the old format. An old reader ignores a `grant` field and can turn a persisted
explicit denial into a broad grant. Keeping a JSON object with an extra version
field does not solve this problem: old readers ignore unknown fields.

`NewPermissionTree`, revocation, intersections and other authorization operations
remain fail-closed. They are NOT restored to vulnerable defaults for compatibility.

## Explicit versioned API

Use `RBACPolicyV2{Root: permissionTree}` with `json.Marshal` / `json.Unmarshal`, or
its SQL `Value` / `Scan` methods, for explicit-grant persistence. Its format is:

```json
["go-utils/rbac",2,{"title":"root","key":"root","grant":"none"}]
```

Every node has an explicit grant. The tuple's shape makes historical object
readers return a decoding error rather than silently ignoring the version.
Readers reject unknown versions, legacy objects, duplicate/unknown fields,
missing/invalid grants, noncanonical identities, excessive size or depth, and
null nodes. Failed reads leave the previous policy untouched; there is no
implicit downgrade or retry with a legacy decoder.

Version 2 has an 8 MiB encoded-byte ceiling, 65,536-node ceiling, and the existing
128-level depth limit. Writers validate before serialization and readers reject
excessive work. These limits do not replace application authorization and quotas.

## Legacy APIs

The original pointer-based `RBACPermissionElem` JSON methods and `Value` / `Scan`
remain available but are deprecated for persistence. Compatible policies keep
the original object representation with no version or grant fields, and both
old/new reader directions preserve their grants. The writer returns
`ErrRBACPolicyMigrationRequired` without data if a policy cannot be represented
with the same grant meaning by an old leaf-based reader. It does not silently
convert a denied leaf into a grant or discard structural metadata.

This intentional refusal is the security exception to compatibility: there is
no safe backward-compatible encoding for every corrected grant state. Handle the
error by migrating storage, not by restoring broad defaults, swallowing errors,
or retrying an unguarded encoder. `RBACPermissionElem` contains a mutex and must
not be copied by value; use pointers and supported methods. Reflection-based
custom serializers, direct field access, and hand-built policy JSON bypass the
supported storage boundary and are not safe migration mechanisms.

Legacy `Scan` / `UnmarshalJSON` keep their current validation and explicit-grant
handling for an authorized import of historical records. They do not accept the
versioned tuple. A new `RBACPolicyV2` reader does not accept legacy objects.

## Migration and rollback sequence

1. Inventory all readers, writers, background jobs, caches and rollback binaries.
   Audit effective grants in authoritative policy data, especially unversioned
   objects written with explicit grant fields since #33. Do not infer intended
   permissions from missing fields or treat an old record as a version 2 policy.
2. Create a separate versioned column, key namespace, table, or topic for v2.
   Isolate it with application/storage access controls so old binaries cannot
   overwrite it. Deploy capable readers before enabling new writes. Verify that
   every reader treats decoding errors as denial, not as an empty/default grant.
3. Import trusted legacy objects explicitly with the legacy reader. Compare the
   resulting effective permission set with the audited expected set; require no
   expansion. Write the reviewed tree through `RBACPolicyV2`, then re-read and
   compare grants. Keep source and destination version ownership explicit.
4. Switch readers and writers together under an application-level migration gate.
   Do not dual-write a v2 denial to the legacy store: it may be unrepresentable.
   Retire old writers and invalidate incompatible caches before new semantics are
   required. An error never authorizes a fallback to a more permissive reader.
5. Roll back only to binaries that understand the active policy store. A return
   to pre-#33 consumers requires an independently audited policy that is exactly
   representable in their model and a coordinated storage cutover. Otherwise stop
   authorization traffic rather than silently broadening access.

This library supplies encoding guards, strict readers and regression evidence;
it cannot coordinate a deployment's database schema, consumer versions, caches
or application authorization. No existing policy store is rewritten automatically.

## Regression coverage

The behavior-first test reproduces a default-deny tree becoming a broad grant
when its unversioned JSON is interpreted by a historical leaf-based reader.
Controls verify safe legacy round trips in both directions. V2 tests cover all
64 configured grant sets, JSON and both SQL representations, historical-reader
rejection, strict malformed-input handling, unchanged state on failure,
concurrent snapshots, and resource limits. Existing exhaustive permission,
revocation, nested-union and normalization assertions are retained; tests of
explicit-grant persistence now select the versioned API deliberately.

### Copy and writer safety

Non-addressable copies of `RBACPermissionElem` are rejected by an embedded JSON
guard instead of bypassing the pointer serializer. Serialize a tree pointer or
`RBACPolicyV2`, never a copied mutex-bearing value. Writers reject duplicate child
identities before returning bytes, so they do not emit ambiguous policies that
the strict reader would reject. Both controls were reproduced on the recovered
candidate and retained from review of the other unreleased candidate.
