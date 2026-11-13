# Transactional memory lifecycle

`NewEngine` retains the historical independent-write behavior and storage format.
A successful sequential replay is suppressed while its TurnID remains in the
bounded `ProcessedTurnIDs` list, but partially failed writes and concurrent
writers are not recoverable with that API alone. A mutex around one engine does
not solve process crashes or independent clients. Existing local and MCP storage
implementations do not become transactional automatically.

For durable atomic turns, explicitly use `memory.NewTransactionalEngine` with a
`storage.TransactionalEngine`. The constructor rejects backends that lack this
capability. `storage/transactionlocal` supplies a Linux implementation for a
local filesystem supporting `flock`, atomic rename, and file/directory `fsync`:

```go
backend, err := transactionlocal.NewEngine(local.Config{RootDir: "/data/memory-tx"})
if err != nil {
    return err
}
engine, err := memory.NewTransactionalEngine(backend, memory.Config{})
if err != nil {
    return err
}
// engine implements memory.Engine and memory.Management.
return engine.AfterTurn(ctx, turn)
```

Every AfterTurn operation now commits scaffold, raw/runtime/legacy events,
facts, active index, metrics, watermarks, and processed metadata together.
A failed mutation aborts the transaction even where the historical engine treats
that mutation as best effort. BeforeTurn, including compaction and recall
metrics, and every Management operation use the same transaction boundary.
Calls on independent engine/backend instances coordinate through the backend.
The local backend serializes mutations across the entire destination, a stronger
boundary than one session. A mutable transaction opens its current snapshot
without copying and stages a private generation only at its first mutation.
No-mutation calls do not copy, sync or publish a generation. The optional
`storage.ReadTransactionalEngine` capability provides shared read-only snapshots;
its views reject mutations. `ListDirWithAbstract` remains a lazy mutable
transaction because missing abstracts may legitimately be created. Model-assisted callbacks run inside that boundary and must honor
context deadlines; they must not re-enter the same backend or perform external
side effects that require storage rollback.

A callback error, panic, or cancellation observed before publication exposes no
changes. A crash before publication leaves the previous generation readable.
Atomicity does not imply that repeating an operation is safe.

Post-publication errors from the local backend carry `storage.CommitError`:

- `CommitOutcomeDurable`: the complete commit is durable; only housekeeping or
  acknowledgement failed. Do not repeat a non-idempotent operation to recover
  from this diagnostic.
- `CommitOutcomeUncertain`: the complete new generation was published, but its
  durability could not be confirmed. A restart may recover the old or new
  complete generation. The SDK does not silently replay a callback.

`storage.PublishedCommit(err)` recognizes these outcomes through wrapped errors.
Absence of a classification (including errors from other providers) does not
prove rollback. `BeforeTurn` and `ListDirWithAbstract` return their prepared
output together with a classified post-publication diagnostic, so callers can
inspect the result and outcome without repeating work just to retrieve output.
They return no output for pre-publication/ordinary failures. Storage errors
remain visible and are never silently discarded.

For `AfterTurn`, retry the same immutable TurnID/input after an uncertain result:
the retained marker suppresses replay if the first attempt committed. The
existing bounded processed-turn window remains; this is not unlimited historical
duplicate suppression. Do not reuse a TurnID with different input or retry it
after it has aged out of that window.

`BeforeTurn` recall metrics count explicit calls, so deliberate repeated calls
can increment them again. These metrics are operational counts, not durable
exactly-once receipts. Reconcile unknown outcomes before deciding to retry a
non-idempotent operation. Consolidation already deduplicates stable insight IDs;
unchanged-state retries do not add insights or increment its run counter again,
including after a lost acknowledgement. Changed facts can legitimately create
new insights. No new operation-ID requirement or record format is introduced.

The transaction contract requires isolated read-your-writes views, durable
atomic commit and cross-client serialization. Non-idempotent operations inside
the view must never blindly retry an ambiguous individual write. In particular,
the current MCP adapter's retried append protocol does not satisfy the contract;
a remote transactional implementation must provide server-side commit and
operation idempotency before advertising this capability. The new engine does
not claim to repair legacy data or give remote FileIO/MCP a transaction feature.

## Migration and rollback

The local backend has a new explicit destination layout; it is not a drop-in
replacement pointed at an existing local-storage root. JSON/JSONL records inside
its active generation retain the existing paths and contents. Existing readers
can read exported records without a new format decoder. Legacy writers are safe
only when invoked inside a backend transaction's callback; wrapping each
individual Write in a transaction does not make an entire turn atomic.

1. Stop all writers, compaction and maintenance against the source destination.
   Back up its complete content. Reconcile any pre-existing partial turns or
   duplicates before migration; this API does not infer a missing legacy commit.
2. Create a separate empty transactional destination. Copy the legacy project
   files through one `backend.Transaction` callback using its supplied Engine
   view. Read back and compare the complete project tree before switching.
3. Switch every reader and writer together to the transactional engine/backend.
   Do not mix old nonparticipating processes with new writers. Keep the source
   offline as the rollback snapshot; it will become stale after new commits.
4. To roll back after new writes, stop all participants and export the active
   generation through backend reads to a separate legacy destination. Compare
   complete contents, then restart legacy clients there. Reverting only the
   executable while sharing the transaction root is unsupported.

A custom backend may allow coordinated rolling deployment when both old and new
application versions participate in its transaction protocol. Record-format
compatibility alone does not authorize mixed nontransactional writers. Keeping
an old writer inside `Transaction` and using the old reader on committed records
is covered by compatibility tests.

## Verification scope

The retained `memory_integrity_repro` tests intentionally demonstrate limitations
of the unchanged legacy and MCP paths. Their failures are not fixed by silently
switching those APIs. The new untagged Linux acceptance tests exercise rollback
before and after every AfterTurn write, post-commit lost acknowledgement,
independent concurrent engines with identical/distinct turns, complete record
compatibility and concurrent turn/compaction/maintenance. They reuse the strict
historical recovery oracle, including its corrupt-value negative control.
The backend tests separately exercise process locking and crash publication.

The local implementation copies a complete generation on the first mutation of
each write transaction. Unchanged files retain permissions and modification times,
so an unrelated session write does not refresh their Stat/summary timestamps.
Mutating calls still have a storage and latency cost proportional to the dataset,
making this implementation suitable only for bounded local datasets.
It is not a scalable database or a distributed/NFS transaction service. Consult
the backend documentation for its filesystem assumptions and retained generations.

Initialization rejects group/world-writable or foreign-owned transaction roots,
and unsafe writable ancestry (trusted sticky directories such as `/tmp` are
allowed). It does not chmod caller directories. Runtime checks reject replaced
roots and unsafe lock/control files. This enforcement applies to the new
transaction backend only; existing local/MCP APIs and caller-selected payload
permissions are unchanged. See the backend README for trusted-owner and import
assumptions. Public management input validation runs before backend admission,
and the ten-second summary deadline covers locking and any lazy staging.

Initialization synchronizes only newly created directories and their necessary
parent entries; existing roots do not require opening unrelated ancestors for
fsync. Snapshot copying supports readable/traversable readonly subdirectories by
restoring their original permissions and times after private staging. Cleanup
retires only the validated previous active snapshot: unrelated prefix matches and
unreferenced crash leftovers are never automatically deleted. Reserve space for
those leftovers and inspect them only during coordinated operator maintenance.

The local backend now persists versioned private retirement-ownership metadata
before replacing a snapshot. This allows reopening after a crash or cleanup
failure leaves an explicitly imported/custom predecessor. It never adopts an
unrelated prefixed directory from its name, and never garbage-collects leftovers
during reopening. Publication, previous-removal durability and ownership-record
removal have separate barriers. Logical memory records and the `current` pointer
format remain unchanged; this metadata adds no caller operation-ID requirement.
Upgrade all participating transactionlocal writers together. Earlier revisions
retain their known failure-state limitation; portable rollback/recovery uses a
quiescent verified active-generation export, not inferred ownership of siblings.
See the backend README for sealed-record, partial-write and physical-identity
backup limits. The legacy `NewEngine` continues best-effort Search on BeforeTurn;
`NewTransactionalEngine` aborts on a storage Search error and returns no prepared
input or committed recall metrics.
