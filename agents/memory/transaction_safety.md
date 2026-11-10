# AfterTurn recovery and concurrency: unresolved backend requirement

Tracking issue: https://github.com/Laisky/go-utils/issues/145

## Current guarantee and limitation

`StandardEngine.AfterTurn` suppresses a sequential repeat of a successfully
committed turn while its ID remains in `MemoryMeta.ProcessedTurnIDs`. It does
**not** provide atomic application, crash recovery, or cross-instance
serialization. `MaxProcessedTurns` bounds even this sequential deduplication
window. Do not interpret a nil result as an exactly-once delivery guarantee for
concurrent calls. A retry following a partial failure can duplicate raw events.

Serializing calls in the application reduces concurrent lost updates but does
not fix partial persistence, uncertain write outcomes, or process crashes.
Marking a turn processed before its writes would lose data rather than fix this.
Both the local and MCP backends expose independent file operations. Their current
public contract has no transaction, conditional create, compare-and-swap, fencing,
or durable lock primitive. No new correctness guarantee is shipped here.

## Executable acceptance reproductions

The build-tagged tests in `after_turn_recovery_repro_test.go` express the desired
behavior and intentionally fail against the current implementation. They are
explicitly opt-in so an unresolved finding does not break the normal test gate:

```
go test -race -tags memory_integrity_repro ./agents/memory \
  -run '^TestAfterTurnRecovery' -count=1
```

These are not passing regressions or a repair. They reproduce:

- A successful raw append followed by a failed runtime append. Retrying the same
  input duplicates the raw event while the other three event views contain one.
- Two separate engines consuming identical metadata and active-index snapshots.
  Both calls succeed, but one processed ID and one independent fact are lost.
- Two separate engines processing the same turn after identical initial reads.
  Both calls succeed and duplicate its raw event.

The concurrency barriers capture both snapshots *before* either write can
proceed. No sleep or scheduler luck is needed. A bounded context makes a broken
barrier fail instead of hanging the suite. The fixture's mutex protects individual
mock operations only; it intentionally does not hide logical storage races.

Once a concrete remedy is implemented, apply these acceptance cases to the new
API and remove the build tag for its regression tests. Preserve legacy format
compatibility tests rather than changing old assertions to hide a break.

## Recommended repair boundary

A proper implementation needs an explicit transactional backend capability and
an opt-in engine API, or a separately versioned durable commit protocol with
backend-enforced conditional writes. An interface alone would transfer the whole
problem to users without fixing either supplied backend; it is not offered as a
solution here.

A transaction-backed engine must include the *entire* operation: scaffold reads
and creation, processed-turn check, input-delta selection, all event and fact
writes, active-index replacement, watermarks, metrics, and both metadata copies.
It must use one transaction-bound storage handle throughout. Metadata is committed
with data, never used as an early marker. Every storage failure must abort even
where the legacy engine currently treats metrics or watermarks as best effort.

BeforeTurn is not read-only: it can compact context and update metrics. The same
transaction discipline must cover BeforeTurn, maintenance, consolidation, and
other mutations, not just AfterTurn. Readers needing a consistent multi-file view
must use a transaction snapshot too.

Backend acceptance requirements:

1. Serializable coordination for a project/session across processes and distinct
   backend and memory-engine instances. A client-side mutex is insufficient.
2. Atomic durable commit of all append, replace, create, and delete operations.
   Failure, cancellation, or process termination before commit leaves no partial
   visible state. A failed storage operation poisons the transaction even if a
   callback accidentally ignores its error.
3. An acknowledged commit survives restart. Lost commit acknowledgements may be
   retried using the same TurnID and input while the deduplication record is
   retained. Arbitrarily late retries require a durable retained receipt, which
   changes the current bounded-retention policy and needs explicit design.
4. Callbacks are not transparently repeated around external heuristic calls.
   External service invocations are not made exactly once by a storage transaction.
5. Failpoints before and after every persistent write, at commit, and at lost
   acknowledgement. Include restart/recovery tests and two-process tests against
   the real backend, not just a transactional mock.
6. Concurrent identical and distinct turns, compaction, and maintenance preserve
   all committed facts, events, counters, and processed IDs without duplication.
7. Backend limits, error semantics, isolation and durability are independently
   verified for both local files and the deployed MCP server. A server change may
   be necessary for MCP; do not emulate CAS with Stat followed by Write.

## Compatibility, migration, and rollback

Keep the existing `Engine`, `StandardEngine`, and storage interfaces operational.
Introduce the stronger behavior through an explicit API and reject unsupported
backends before any writes. Existing JSON/JSONL formats can remain byte-compatible
with transactions, but durable backend journals need their own versioned private
namespace and documented recovery ownership.

Quiesce *all* legacy writers before opting a project into transactions. Do not
mix nontransactional writers or older maintenance processes with the new engine.
Qualify old-writer/new-reader and new-writer/old-reader behavior using frozen
fixtures and the same archive codec/configuration. Format compatibility does not
make a rolling mixed-writer deployment safe.

Before rollback, stop transactional writers, recover any pending transactions,
verify a complete committed snapshot, and then resume legacy writers. Rolling
back relinquishes the stronger atomicity and concurrency guarantees. If a new
commit protocol changes file semantics rather than only the backend's commit
mechanism, use separate versioned storage and an explicit migration/export path;
old readers must not silently consume new semantics.
