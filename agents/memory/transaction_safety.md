# AfterTurn recovery and concurrency

Tracking issue: https://github.com/Laisky/go-utils/issues/145

## Explicit transactional remedy

`NewTransactionalEngine` requires `storage.TransactionalEngine`, whose callback
provides an isolated view with atomic durable commit and cross-client
serialization. The supplied Linux `storage/transactionlocal` adapter implements
that contract using a kernel filesystem lock and copy-on-write generations with
an atomically published, synchronized pointer. An interface or process-local
mutex alone is not the remedy.

All lifecycle and management operations, including BeforeTurn compaction, use
the same transaction boundary. Every storage operation error aborts the call,
including errors historical code treats as best effort. A failed turn publishes
neither partial events nor an early processed marker. A post-commit lost
acknowledgement for AfterTurn is resolved by retrying the same retained TurnID.
Other operations do not gain exactly-once invocation semantics: inspect the
classified commit outcome before retrying. Prepared BeforeTurn/summary results
remain available with post-publication diagnostics.

See [transactional memory](../../docs/memory-transactions.md) and the
[backend contract](storage/transactionlocal/README.md) for examples, filesystem
assumptions, costs, migration and rollback. Existing JSON/JSONL bytes and paths
inside a project are unchanged; the backend root layout is explicitly new.
Quiesce all legacy writers before moving to a separate transactional destination.
Format compatibility does not permit mixed nontransactional writers.

Mutable transactions serialize the root and hold their lock through heuristic
calls. No-mutation transactions avoid staging; explicit read-only snapshots
allow concurrent readers. Set deadlines and bound dataset size. An optimistic revision protocol
that releases the lock around external calls remains a possible future design;
it is not implemented here. Callbacks must not reenter the backend. External
service invocations are not made exactly once by a storage transaction.

## Legacy and remote limits

`NewEngine` retains independent-write behavior. Its successful sequential replay
suppression lasts only while the TurnID remains in `ProcessedTurnIDs`, bounded
by `MaxProcessedTurns`. Partial failures, uncertain append acknowledgements and
independent concurrent engines may duplicate events or lose updates. A nil
result is not an exactly-once processing guarantee. Neither marking a turn early
nor wrapping an engine in a process-local mutex fixes those failure modes.

The unchanged local and MCP adapters do not advertise transactional capability.
The MCP append retry can repeat an applied mutation after a lost acknowledgement.
A remote provider must implement native atomic commits and idempotent operation
handling before satisfying the new interface. No deployed FileIO/MCP durability
has been tested or changed by this remedy.

Transactions do not repair already-corrupt or partially applied legacy turns.
Reconcile that state before migration. The bounded processed-ID window and
historical behavior for reused TurnIDs remain unchanged: callers must use a
unique immutable ID/input and retry only while its receipt is retained.

## Executable evidence

The original `memory_integrity_repro` cases remain deliberately opt-in because
they reproduce the unchanged legacy/MCP limitations:

```sh
go test -race -tags memory_integrity_repro ./agents/memory \
  -run '^TestAfterTurnRecovery' -count=1
```

They deterministically reproduce a failed runtime append after raw persistence,
lost metadata/index updates between distinct engines, duplicate identical turns,
and the real MCP client's lost-acknowledgement append retry. These failures are
not passing regressions for the legacy API.

The new transactional regressions run without that tag on Linux:

```sh
go test -race ./agents/memory -run '^TestTransactional' -count=1
go test -race ./agents/memory/storage/transactionlocal -count=1
```

Acceptance covers faults before and after every turn write, ignored metadata
read failures, lost commit acknowledgement, identical/distinct concurrent turns
through separate backend handles, compaction/maintenance, and complete
old-writer/new-reader plus new-writer/old-reader record compatibility. Backend
subprocess tests cover cross-process serialization, interruption before
publication and committed restart behavior. They test process death, not physical
power loss or a storage controller ignoring fsync.

The retained oracle compares full IDs, timestamps, provenance and payloads in
all four event views, exact tier/legacy facts and active-index values, both meta
copies, processed IDs, metrics and watermarks. Passing controls accept correct
sequential state and reject same-sized wrong fact maps, malformed JSONL tails,
missing/corrupt index files and invalid metadata versions. Direct strict reads
avoid hiding errors through recovery loaders. Those controls validate the oracle;
they do not supply transaction guarantees to legacy backends.
