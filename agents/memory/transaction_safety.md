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
- A single logical call through the shipped MCP write/retry adapter: a synthetic
  transport persists the raw append and returns a retryable error instead of its
  acknowledgement. The adapter retries and duplicates that event. This verifies
  client behavior only, not the deployed MCP server or filesystem durability.

The concurrency barriers capture both snapshots *before* either write can
proceed. No sleep or scheduler luck is needed. A bounded context makes a broken
barrier fail instead of hanging the suite. The fixture's mutex protects individual
mock operations only; it intentionally does not hide logical storage races.

The shared oracle compares exact IDs, timestamps, user provenance and payloads in
all four event views, exact tier/legacy fact records and active-index entries,
both metadata copies, processed IDs, metrics and watermarks. It accepts either
record order for distinct turns (exact record multisets, not physical append
ordering), but requires consistent latest-turn
markers and complete counts. Ordinary passing controls verify correct sequential
state and reject a same-sized corrupt fact map, malformed JSONL tails, missing
or corrupt index files and invalid metadata versions. Reads use direct storage
and strict decoding, never recovery loaders that could skip or repair damage.
MCP controls distinguish no fault
and failure-before-append from a lost acknowledgement. These controls validate the
test harness, not transaction safety.

Compile tagged cases without executing expected failures:
`go test -tags memory_integrity_repro ./agents/memory -run '^$'`.

Once a concrete remedy is implemented, apply these acceptance cases to the new
API and remove the build tag for its regression tests. Preserve legacy format
compatibility tests rather than changing old assertions to hide a break.

## Recommended repair boundary

A proper implementation needs an explicit transactional backend capability and
an opt-in engine API, or a separately versioned durable commit protocol with
backend-enforced conditional writes. An interface alone would transfer the whole
problem to users without fixing either supplied backend; it is not offered as a
solution here.

A transaction-backed engine must atomically validate and commit the *entire* persistence operation: scaffold reads
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

## Proposed heuristic concurrency strategy (not implemented)

Use an optimistic, backend-enforced session revision. Read a consistent snapshot
and its revision, then release the read transaction before calling the external
heuristic. The revision covers all inputs to its decisions, including fact
changes, compaction, maintenance and consolidation. Hold no persistence
transaction or session lock across an LLM/network call.

After the result arrives, begin a short transaction, recheck the durable turn
receipt and input identity, and compare the revision. If unchanged, validate and
apply the staged result and commit required state atomically. If changed, abort
without persisting stale decisions and return an explicit conflict. Never
silently rerun the heuristic or overwrite newer facts. A caller-controlled,
bounded retry may explicitly request a new snapshot and recomputation; that
invokes and may charge the external service again.

Resolve uncertain commit acknowledgements through the durable receipt before any
recomputation. Transactions do not make external calls exactly once. Neither
current backend provides this protocol; a local mutex or Stat-then-Write check
is not a replacement.

Acceptance must cover: a revision change during a blocked heuristic produces a
conflict with zero staged writes and exactly one external invocation; explicit
recomputation uses new facts and a second invocation; unchanged snapshots commit
once; lost acknowledgements resolve receipts without another heuristic call;
maintenance/consolidation invalidate stale snapshots. The stronger API must
reject unsupported backends; the legacy API remains unchanged.

## Remaining acceptance matrix

The production remedy must also cover retries across UTC shard boundaries,
TurnID reuse with different payloads (explicit conflict policy), processed-ID
eviction at MaxProcessedTurns, and compaction/maintenance races. Define which
records are authoritative and which indexes/metrics can be rebuilt from a
committed revision; do not publish a mixed-revision reader view.

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
