# Agent Memory V2: Audit, Comparison, and Upgrade Roadmap

## Menu

- [Agent Memory V2: Audit, Comparison, and Upgrade Roadmap](#agent-memory-v2-audit-comparison-and-upgrade-roadmap)
  - [Menu](#menu)
  - [1. Scope and Method](#1-scope-and-method)
  - [2. Current `go-utils` Memory: What It Actually Is](#2-current-go-utils-memory-what-it-actually-is)
    - [2.1 Core Design Identity](#21-core-design-identity)
    - [2.2 Online Path](#22-online-path)
    - [2.3 Offline Path](#23-offline-path)
    - [2.4 Data Model Summary](#24-data-model-summary)
    - [2.5 Important Implementation Realities](#25-important-implementation-realities)
  - [3. Google Always-On Memory Agent: What It Actually Is](#3-google-always-on-memory-agent-what-it-actually-is)
    - [3.1 Core Design Identity](#31-core-design-identity)
    - [3.2 Functional Decomposition](#32-functional-decomposition)
    - [3.3 Storage Model](#33-storage-model)
    - [3.4 Cognitive Loop](#34-cognitive-loop)
    - [3.5 Practical Limits in the Google Reference](#35-practical-limits-in-the-google-reference)
  - [4. Principle-Level Comparison](#4-principle-level-comparison)
    - [4.1 The Short Version](#41-the-short-version)
    - [4.2 Principle Comparison Table](#42-principle-comparison-table)
    - [4.3 Deeper Methodology Differences](#43-deeper-methodology-differences)
      - [A. What counts as memory](#a-what-counts-as-memory)
      - [B. When cognition happens](#b-when-cognition-happens)
      - [C. How recall is kept safe](#c-how-recall-is-kept-safe)
      - [D. How knowledge evolves](#d-how-knowledge-evolves)
  - [5. Implementation Gap Analysis](#5-implementation-gap-analysis)
    - [5.1 Current Strengths to Preserve](#51-current-strengths-to-preserve)
    - [5.2 Current Gaps Relative to the Google Methodology](#52-current-gaps-relative-to-the-google-methodology)
      - [Gap 1: No background semantic consolidation](#gap-1-no-background-semantic-consolidation)
      - [Gap 2: No explicit observation layer](#gap-2-no-explicit-observation-layer)
      - [Gap 3: Approximate deduplication at scale](#gap-3-approximate-deduplication-at-scale)
      - [Gap 4: No deletion or supersession semantics](#gap-4-no-deletion-or-supersession-semantics)
      - [Gap 5: Maintenance summaries are operational, not semantic](#gap-5-maintenance-summaries-are-operational-not-semantic)
      - [Gap 6: Session-local search only](#gap-6-session-local-search-only)
      - [Gap 7: Caller-supplied history and engine recall are not reconciled](#gap-7-caller-supplied-history-and-engine-recall-are-not-reconciled)
  - [6. Recommended V2 Principles](#6-recommended-v2-principles)
    - [6.1 Principle 1: Separate online recall from offline consolidation](#61-principle-1-separate-online-recall-from-offline-consolidation)
    - [6.2 Principle 2: Avoid a persisted observation layer until it is justified](#62-principle-2-avoid-a-persisted-observation-layer-until-it-is-justified)
    - [6.3 Principle 3: Make deduplication exact where it must be exact](#63-principle-3-make-deduplication-exact-where-it-must-be-exact)
    - [6.4 Principle 4: Add memory state transitions](#64-principle-4-add-memory-state-transitions)
    - [6.5 Principle 5: Keep bounded prompt assembly as a hard invariant](#65-principle-5-keep-bounded-prompt-assembly-as-a-hard-invariant)
    - [6.6 Principle 6: Make consolidation policy-driven](#66-principle-6-make-consolidation-policy-driven)
    - [6.7 Principle 7: Reconcile caller context with engine recall explicitly](#67-principle-7-reconcile-caller-context-with-engine-recall-explicitly)
  - [7. Recommended V2 Architecture](#7-recommended-v2-architecture)
    - [7.1 Proposed Layers](#71-proposed-layers)
    - [7.2 Proposed Record Families](#72-proposed-record-families)
    - [7.3 Proposed Storage Topology](#73-proposed-storage-topology)
    - [7.4 Proposed Context Assembly Contract](#74-proposed-context-assembly-contract)
  - [8. Upgrade Roadmap](#8-upgrade-roadmap)
    - [8.0 Confidence and Scope Reduction](#80-confidence-and-scope-reduction)
    - [8.1 Phase 1: Correctness Hardening](#81-phase-1-correctness-hardening)
    - [8.2 Phase 2: Minimal Background Consolidation](#82-phase-2-minimal-background-consolidation)
    - [8.3 Phase 3: Optional Observation Materialization](#83-phase-3-optional-observation-materialization)
    - [8.4 Phase 4: Optional Recall Policy Tuning](#84-phase-4-optional-recall-policy-tuning)
    - [8.5 Phase 5: Explicitly Out of Scope for V2](#85-phase-5-explicitly-out-of-scope-for-v2)
  - [9. Quantitative Evaluation and Acceptance Criteria](#9-quantitative-evaluation-and-acceptance-criteria)
    - [9.1 Implemented Evaluation Assets](#91-implemented-evaluation-assets)
    - [9.2 Metrics](#92-metrics)
    - [9.3 Scenario Set](#93-scenario-set)
    - [9.4 Current V1 Baseline](#94-current-v1-baseline)
    - [9.5 V2 Acceptance Gates](#95-v2-acceptance-gates)
    - [9.6 Commands](#96-commands)
  - [10. Recommended Final Direction](#10-recommended-final-direction)
  - [11. Bottom Line](#11-bottom-line)

## 1. Scope and Method

This document is not a rewrite of [agents_memory.md](./agents_memory.md). It is an implementation audit and strategy paper for the next generation of the agent memory subsystem.

The analysis is grounded in two sources:

1. The real implementation under `agents/memory` and `agents/memory/storage` in this repository.
2. Google Cloud's `always-on-memory-agent` reference implementation under `gemini/agents/always-on-memory-agent`, primarily `README.md` and `agent.py`.

The goal is to compare principles, not just APIs. The important question is not whether both systems "store memory", but what each system believes memory is, when memory should be processed, how recall is bounded, and where operational risk sits.

## 2. Current `go-utils` Memory: What It Actually Is

### 2.1 Core Design Identity

The current implementation is best described as a bounded, turn-centric memory substrate for agent runtimes.

Its defining traits are:

- Memory is attached to the turn lifecycle through `BeforeTurn` and `AfterTurn`.
- Prompt assembly is bounded and defensive.
- Persistence is append-first and filesystem-shaped.
- Facts are treated as small structured records with lifecycle tiers.
- Maintenance is operational, not cognitive.

This is a very different design center from an always-on autonomous memory daemon. The current engine is optimized for predictable request-time behavior and storage portability.

### 2.2 Online Path

At request time, the engine performs four jobs:

1. Load recent runtime context from canonical storage, with legacy fallback.
2. Load active facts from tier shards, rank them, and cap the recall set.
3. Optionally search the current session subtree and inject search hits as prompt references.
4. Return a bounded input package: `[memory_reference?] + recent_context + current_input`.

This path is intentionally conservative:

- It injects one `developer` reference block rather than mutating the user conversation directly.
- It clips recalled chunks before prompt injection.
- It treats search as best effort.
- It can compact runtime context without failing the turn.

### 2.3 Offline Path

The offline side exists, but it is primarily operational:

- Compact runtime context when it grows too large.
- Archive old raw shards.
- Sweep expired facts.
- Refresh directory summary files.

What it does not yet do is equally important:

- It does not derive higher-order insights across many memories.
- It does not maintain explicit memory graphs or relationships.
- It does not run background semantic consolidation.
- It does not re-score or rewrite memory based on later evidence.

### 2.4 Data Model Summary

The current engine stores three memory-adjacent data classes:

1. Runtime context events.
2. Tiered structured facts.
3. Operational metadata and summaries.

That means the system already distinguishes between:

- hot prompt context,
- durable but queryable memory facts,
- cold historical logs.

This separation is strong and should be preserved in any V2 design.

### 2.5 Important Implementation Realities

The audit surfaced several details that matter for V2 planning:

- `BaseInstructions` and `UserID` exist in the API shape but are currently unused.
- Search is session-local because `BeforeTurn` searches only under `/memory/{session_id}`.
- Runtime compaction is structural, not semantic. Older events are replaced by one `compact_summary` event.
- Directory `.abstract` and `.overview` files are deterministic filesystem summaries, not model-generated knowledge artifacts.
- The heuristic tool schema includes `deleted_fact_ids`, but deletions are ignored by the current engine.
- `meta/watermarks.json` is scaffolded, but not meaningfully advanced yet.
- The most important limitation: `AfterTurn` deduplication compares new facts against `loadRecallFacts`, which is ranked and capped by `RecallFactsLimit`. This bounds latency, but it means duplicate prevention is approximate rather than exhaustive at scale.

## 3. Google Always-On Memory Agent: What It Actually Is

> <https://github.com/GoogleCloudPlatform/generative-ai/tree/main/gemini/agents/always-on-memory-agent>

### 3.1 Core Design Identity

Google's reference implementation is best described as an always-on memory service with LLM-native ingestion, periodic consolidation, and query-time synthesis over a single persistent store.

Its defining traits are:

- Memory is a continuously running service, not just a turn hook.
- Ingestion is multimodal.
- Consolidation is a first-class background task.
- Querying is a separate specialist capability.
- Storage is centralized in SQLite rather than sharded filesystem records.

This design is closer to a personal knowledge daemon than to a bounded memory adapter for request pipelines.

### 3.2 Functional Decomposition

The Google system is explicitly decomposed into three specialist agents plus an orchestrator:

1. `ingest_agent`
2. `consolidate_agent`
3. `query_agent`
4. `memory_orchestrator`

That decomposition matters because it encodes a methodology:

- ingestion is not recall,
- consolidation is not retrieval,
- retrieval is not storage.

In the current `go-utils` engine, those responsibilities still partially collapse into the same lifecycle path.

### 3.3 Storage Model

Google's implementation uses SQLite with three main tables:

- `memories`
- `consolidations`
- `processed_files`

Each memory record contains:

- source
- raw text
- summary
- entities
- topics
- connections
- importance
- created_at
- consolidated flag

This is a document-memory model, not a fact-tier model.

### 3.4 Cognitive Loop

The most important principle in Google's implementation is not the database. It is the background consolidation loop.

Every interval, the system:

1. Reads unconsolidated memories.
2. Asks the model to find patterns and relationships.
3. Writes a synthesized consolidation record.
4. Marks source memories as consolidated.
5. Adds explicit connection metadata back to memories.

This is the clearest methodological difference from the current `go-utils` implementation.

### 3.5 Practical Limits in the Google Reference

The Google design is strong conceptually, but the reference implementation is intentionally simple:

- Query reads recent memories rather than using a strong bounded recall planner.
- There is no real long-term storage tiering.
- There is no expiration model comparable to `L0/L1/L2`.
- There is no strong turn-idempotent log model like the current `AfterTurn` path.
- There is no storage portability comparable to local vs MCP backends.
- The reference is effectively single-service and single-store, not a reusable subsystem contract.

So Google contributes a methodology more than a hardened storage architecture.

## 4. Principle-Level Comparison

### 4.1 The Short Version

The current `go-utils` system answers: how do we safely attach memory to an agent turn?

Google's system answers: how do we keep memory thinking in the background even when no user turn is active?

Those are complementary questions, not competing ones.

### 4.2 Principle Comparison Table

| Dimension                | Current `go-utils` Memory                              | Google Always-On Memory Agent                            |
| ------------------------ | ------------------------------------------------------ | -------------------------------------------------------- |
| Primary identity         | Turn lifecycle substrate                               | Always-on memory service                                 |
| Main optimization target | Bounded prompt assembly and operational safety         | Continuous ingestion and cross-memory synthesis          |
| Runtime entrypoint       | `BeforeTurn` / `AfterTurn`                             | File watcher, timer loop, HTTP API                       |
| Storage abstraction      | Pluggable storage interface, filesystem topology       | SQLite tables in one service                             |
| Memory unit              | Structured fact + event log                            | Document-like memory record + consolidation record       |
| Recall path              | Ranked facts + recent context + optional search chunks | Read memories and consolidation history, then synthesize |
| Forgetting model         | Tier expiry and shard archiving                        | No real forgetting model in the reference                |
| Consolidation model      | Operational maintenance only                           | Periodic semantic consolidation                          |
| Relationship model       | Implicit only                                          | Explicit `connections` metadata                          |
| Multimodal support       | Not first-class in current memory engine               | First-class via Gemini multimodal ingestion              |
| Boundedness discipline   | Strong                                                 | Weak to moderate                                         |
| Idempotent persistence   | Strong                                                 | Limited                                                  |
| Migration compatibility  | Present                                                | Not a concern in the reference                           |

### 4.3 Deeper Methodology Differences

#### A. What counts as memory

Current `go-utils` memory is fact-centric and event-centric.

- Facts are normalized, tiered, and recall-ranked.
- Event logs preserve append-only traces.

Google memory is observation-centric.

- A memory begins as a summarized observation with entities, topics, importance, and source.
- Facts are not the primary storage abstraction.

This means the current engine is better at bounded durable recall, while the Google reference is better at knowledge-work style accumulation.

#### B. When cognition happens

In the current system, cognition is mostly synchronous with the user turn:

- extract a few facts,
- recall a few facts,
- optionally do best-effort search,
- persist the turn.

In the Google system, cognition is also asynchronous:

- ingestion can happen outside a conversation,
- consolidation definitely happens outside a conversation,
- the memory system can improve itself while idle.

This is the main missing principle in the current engine.

#### C. How recall is kept safe

The current system is much stronger on prompt safety and boundedness:

- recall limits,
- clipped snippets,
- developer reference block,
- threshold-based compaction.

Google's reference prioritizes simplicity of behavior over strict budget control.

For production agent runtimes, the current `go-utils` discipline is the better base.

#### D. How knowledge evolves

The current system is mostly additive:

- append logs,
- append facts,
- dedupe best effort,
- expire by tier.

Google's system is reconstructive:

- take multiple memories,
- derive patterns,
- store an insight,
- attach links,
- mark source items as processed.

That reconstructive principle is the most valuable import for V2.

## 5. Implementation Gap Analysis

### 5.1 Current Strengths to Preserve

The next generation should not discard the strengths already present:

- Strong storage abstraction.
- Good operational ergonomics for filesystem and MCP-backed environments.
- Append-first event persistence.
- Bounded prompt assembly.
- Tiered fact retention.
- Clear migration compatibility patterns.
- Robust idempotency around turn writes.

These are not accidental details. They are the pieces that make the current subsystem reusable.

### 5.2 Current Gaps Relative to the Google Methodology

The biggest gaps are not syntactic. They are missing loops and missing models.

#### Gap 1: No background semantic consolidation

The engine can maintain storage, but it cannot improve memory quality over time unless new turns arrive.

Impact:

- no cross-turn insight synthesis,
- no relation discovery,
- no concept compression,
- no autonomous memory refinement.

#### Gap 2: No explicit observation layer

Today the engine jumps quickly from turn text to memory facts. It lacks a richer intermediate memory object such as:

- observation summary,
- source references,
- entities,
- topics,
- importance,
- derived links.

That makes the system efficient, but also thin.

#### Gap 3: Approximate deduplication at scale

Because `AfterTurn` compares against a recall-capped existing fact set, the system can drift into duplicate fact growth once a session accumulates enough facts.

This is the most important storage-correctness issue for V2.

#### Gap 4: No deletion or supersession semantics

The LLM heuristic schema already hints at deletion, but the engine does not implement it.

Consequences:

- stale facts can expire, but they cannot be actively superseded in a first-class way,
- contradictory facts are not explicitly tracked,
- there is no tombstone or replacement record model.

#### Gap 5: Maintenance summaries are operational, not semantic

`.abstract` and `.overview` are useful for operators, but they are not memory products in the cognitive sense.

That is fine for today, but V2 should distinguish:

- operator summaries,
- semantic summaries,
- recall summaries.

#### Gap 6: Session-local search only

The current search scope is aligned with per-session isolation, but it prevents selective cross-session recall or cross-project memory strategies.

V2 needs explicit policy-controlled scope, not hardcoded locality.

#### Gap 7: Caller-supplied history and engine recall are not reconciled

Many LLM API callers already send selected chat history with each request. In practice, those items may already contain outputs that previously flowed through `BeforeTurn` and `AfterTurn`.

The current contract cannot represent the difference between:

- current-turn delta,
- caller-supplied historical context,
- engine-recalled historical context.

Consequences:

- `BeforeTurn` can assemble duplicate prompt context when the caller already brings history,
- `AfterTurn` can only trim the engine's own generated prefix, so replayed caller history has no first-class dedup model,
- recent-window recall and semantic recall cannot be budgeted against caller history deterministically.

V2 needs explicit context reconciliation rather than implicit prefix stripping.

## 6. Recommended V2 Principles

The next generation should not become a clone of the Google reference. It should combine the current engine's rigor with the Google model's cognitive loop.

### 6.1 Principle 1: Separate online recall from offline consolidation

Keep `BeforeTurn` fast and bounded.

Move the following work out of the critical path:

- cross-memory clustering,
- relationship inference,
- observation summarization,
- stale-memory review,
- contradiction detection.

This keeps latency predictable while still allowing memory quality to improve between turns.

### 6.2 Principle 2: Avoid a persisted observation layer until it is justified

The current engine already has one raw evidence layer: append-only events.

That means V2 should not immediately add another persisted intermediate model unless we can show one of these is true:

- raw-event scans have become too expensive for background consolidation,
- semantic consolidation needs richer source normalization than raw events can provide,
- recall quality measurably improves when observations exist as first-class records.

The correct first step is a **derived observation model** inside the consolidator.

Suggested derived shape:

- `observation_id`
- `source_turn_id`
- `summary`
- `raw_ref`
- `entities`
- `topics`
- `importance`
- `status` such as `new`, `consolidated`, `superseded`

If this model proves necessary over time, it can later be materialized into storage. Until then, facts should become one output of memory processing, but observations do not need to become another mandatory stored record family.

### 6.3 Principle 3: Make deduplication exact where it must be exact

Turn-time prompt recall can remain capped.

Memory correctness logic should not.

V2 should maintain an exact active-memory index for identity-sensitive operations such as:

- fact upsert,
- deletion,
- supersession,
- contradiction detection.

This can still be storage-agnostic if implemented as a compact secondary index per session.

### 6.4 Principle 4: Add memory state transitions

Memory should not only exist or expire. It should transition.

Suggested states:

- `active`
- `consolidated`
- `superseded`
- `contradicted`
- `expired`
- `deleted`

This is where the current tier model and the Google consolidation model can meet.

### 6.5 Principle 5: Keep bounded prompt assembly as a hard invariant

This is the part the current system already gets right.

Even after V2 adds richer memory products, recall injection should still be:

- explicit,
- scoped,
- clipped,
- ranked,
- budget-aware.

The online path should never devolve into "load everything and let the model figure it out".

### 6.6 Principle 6: Make consolidation policy-driven

Google's timer loop is valuable, but V2 should be more explicit about policy.

Consolidation should be triggered by policies such as:

- every N minutes,
- after N new observations,
- when a session becomes idle,
- before archival,
- before retention sweep.

This keeps consolidation controllable in both local and MCP-backed deployments.

### 6.7 Principle 7: Reconcile caller context with engine recall explicitly

V2 should treat caller-provided conversation history as a separate context source, not as opaque current input.

The core engine should preserve backward compatibility, but its normalized request model should distinguish:

- `current_turn_items`,
- `caller_history_items`,
- engine-recalled `recent_items`,
- engine-recalled semantic memory such as facts or insights.

Recommended contract:

- keep `CurrentInput` for backward compatibility,
- add optional `ConversationItems` or an equivalent full-list input,
- make the current-turn boundary explicit with `CurrentInputCount`, `CurrentInputStart`, or an equivalent field,
- require oldest-to-newest ordering,
- never rely only on "the last item is latest" inside the engine,
- assign stable item identity using `turn_id + item_index` when present, falling back to a normalized content hash.

An SDK helper can still expose a convenience mode where the caller passes one flat slice and marks the trailing items as the current turn. The engine-internal contract should remain explicit.

This gives V2 exact overlap suppression, predictable prompt assembly, and safe delta persistence even when callers still choose to bring their own history.

## 7. Recommended V2 Architecture

### 7.1 Proposed Layers

V2 should evolve into five layers:

1. Turn lifecycle layer.
2. Online recall assembly layer.
3. Offline consolidation layer.
4. Exact active-memory index layer.
5. Storage backend layer.

### 7.2 Proposed Record Families

Core V2 should store four record families:

1. Raw events
2. Runtime context snapshots
3. Facts
4. Insights and links

Optional later additions:

1. Persisted observations
2. Memory deletion tombstones
3. Contradiction records
4. Recall feedback metrics

### 7.3 Proposed Storage Topology

The current topology can be extended rather than replaced:

- `/events/raw/...`
- `/runtime/context/...`
- `/memory_tiers/...`
- `/insights/...`
- `/indexes/active_facts.json` or equivalent shard set

The pluggable storage contract can remain intact.

Important scope control:

- initial V2 should not add `/observations/...` as a required storage family,
- derived observations should be computed from raw events during consolidation first,
- observation persistence should be added only if metrics show that the derived approach is too slow or too lossy.

### 7.4 Proposed Context Assembly Contract

The online assembly path should normalize all context sources before ranking or clipping.

Recommended flow:

1. Normalize input into `caller_history_items` and `current_turn_items`.
2. Build an exclusion set keyed by stable item identity or normalized content hash.
3. Load a recent window from canonical storage order, using persisted timestamps or append order rather than lexical `TurnID` sorting.
4. Load semantic hits from older session history using hybrid lexical plus embedding retrieval.
5. Merge caller history, recent window items, semantic hits, and fact or insight references with exact deduplication.
6. Apply a source-aware budget, keeping recency continuity and caller-explicit context ahead of lower-confidence semantic hits.
7. Return prepared input plus provenance counters that explain what was kept or dropped.

Important implementation notes:

- When the caller already supplies full recent history, steps 3 and 4 should mainly act as gap-filling and dedup stages rather than as blind additional recall.
- `AfterTurn` should reuse the same normalized identities to persist only the current-turn delta, never the caller's replayed history.
- Embeddings for caller-supplied history should be reused from storage when possible. V2 should avoid recomputing expensive history embeddings on every hot-path request.

## 8. Upgrade Roadmap

### 8.0 Confidence and Scope Reduction

V2 is not uniformly better than V1. It is only better if we scope it narrowly.

On net, I am:

- about **0.70 confident** that the revised core V2 outperforms V1 overall,
- about **0.45 confident** that the original broader roadmap would outperform V1 overall in the near term.

Confidence by area:

- **High confidence (about 0.85):** exact active-fact indexing, explicit supersession or deletion semantics, history-aware prompt deduplication and persistence trimming, better watermarks, and better metrics. These directly address real weaknesses in the current implementation, especially the fact that `AfterTurn` compares candidates against `loadRecallFacts`, while `loadRecallFacts` is ranked and capped by `RecallFactsLimit`, and the fact that caller-supplied history is not a first-class part of the context model today.
- **Medium confidence (about 0.65):** a minimal background consolidation loop that works over existing raw events and fact shards without changing the hot path. This imports the strongest idea from the Google design while preserving the current runtime discipline.
- **Low confidence (below 0.40):** first-class persisted observations, a richer recall-policy engine, or multimodal ingestion as part of the initial V2 commitment. These may become useful later, but the current codebase does not justify them yet.

Revised core V2 scope:

- Phase 1 is required.
- Phase 2 is justified if implemented minimally.
- Phases 3 to 5 are optional follow-on work, not part of the base V2 promise.

### 8.1 Phase 1: Correctness Hardening

Objective: remove the most important architectural debt without changing the public model too much.

Deliverables:

- Add an exact active-fact index for `AfterTurn` deduplication.
- Extend `BeforeTurn` input normalization so callers can provide optional history plus an explicit current-turn boundary while preserving the current `CurrentInput` path.
- Add exact item-level overlap suppression for prompt assembly and exact delta trimming for `AfterTurn` persistence, based on stable item identities or normalized hashes.
- Implement first-class delete and supersede semantics.
- Advance `watermarks.json` meaningfully.
- Add metrics for compaction, recall count, dedupe decisions, consolidation lag, prompt-duplicate drops, and persisted-history trims.

Expected outcome:

- preserve current ergonomics,
- eliminate approximate dedup as the main correctness risk,
- prevent duplicated prompt context or replayed history writes when callers bring their own context,
- create the operational data needed to decide whether later phases are justified.

### 8.2 Phase 2: Minimal Background Consolidation

Objective: add the missing second loop without introducing new primary storage families yet.

Deliverables:

- Add a background consolidation API or scheduler.
- Derive temporary observations from raw events at consolidation time rather than persisting them immediately.
- Consolidate raw evidence into insight records, contradiction markers, or supersession actions.
- Add a canonical recent-window loader based on persisted event order or watermarks rather than `TurnID` sorting assumptions.
- Reuse background-maintained indexes or embeddings so history-aware recall does not require full re-embedding of caller-supplied context on every request.
- Keep the default latest-only `BeforeTurn` and `AfterTurn` behavior essentially unchanged.

Expected outcome:

- memory quality can improve without waiting for another user turn,
- the system gains reconstructive memory behavior,
- complexity stays materially lower than the original plan.

### 8.3 Phase 3: Optional Observation Materialization

Objective: persist observations only if Phase 2 proves that derived observations are too expensive or too lossy.

Deliverables:

- Introduce observation records only after metrics show real need.
- Add observation indexes only if repeated raw-event scans become a measurable bottleneck.
- Let `BeforeTurn` recall observations separately only if that demonstrably improves recall quality.

Expected outcome:

- richer memory products only when the complexity is justified,
- no mandatory storage-topology expansion before there is evidence it pays off.

### 8.4 Phase 4: Optional Recall Policy Tuning

Objective: tune recall assembly only after Phase 1 and Phase 2 metrics show ranking quality is the next bottleneck.

Deliverables:

- Different recall budgets for facts, observations, and insights.
- Scope rules by session, user, or project.
- Freshness and trust weighting.
- History-source weighting between caller-supplied context, recent-window recall, and semantic hits.
- Optional contradiction suppression or warning behavior.

Expected outcome:

- recall stays bounded while becoming more expressive.

### 8.5 Phase 5: Explicitly Out of Scope for V2

Objective: keep platform expansion out of the initial V2 commitment.

Deferred topics:

- external ingestion endpoints,
- file and document adapters,
- observation extraction from non-text artifacts,
- source-aware retention and recall.

Expected outcome:

- V2 remains focused on fixing correctness and adding one justified offline loop,
- the team avoids turning a memory subsystem upgrade into a broader platform rewrite.

## 9. Quantitative Evaluation and Acceptance Criteria

Architectural claims are not enough. V2 should be accepted only if it beats V1 on a stable quantitative suite.

### 9.1 Implemented Evaluation Assets

The repository now contains an executable evaluation harness:

- `agents/memory/eval_harness_test.go`
- `agents/memory/eval_baseline_test.go`
- `agents/memory/eval_benchmark_test.go`

These files do three jobs:

1. Define reusable quantitative metrics.
2. Capture the current V1 baseline.
3. Define the comparison and acceptance rules that V2 must satisfy.

### 9.2 Metrics

| Metric | Definition | Interpretation |
| --- | --- | --- |
| `fact_recall_recall` | `matched_expected_facts / expected_facts` | Whether the engine recalls the facts it must recall |
| `fact_recall_precision` | `matched_expected_facts / recalled_facts` | Whether recall stays relevant rather than noisy |
| `expired_fact_suppression_rate` | `1 - recalled_expired_targets / expired_targets` | Whether expired short-term facts stop leaking into recall |
| `durable_fact_survival_rate` | `recalled_durable_targets / durable_targets` | Whether long-lived memory survives maintenance |
| `idempotency_score` | `1` when replaying the same `TurnID` produces no duplicate writes, else `0` | Whether retry safety remains intact |
| `session_isolation_leak_rate` | `leaked_cross_session_recalls / isolation_checks` | Whether default recall leaks between sessions |
| `compaction_guard_score` | `1` when compaction is triggered and recorded under pressure, else `0` | Whether bounded prompt assembly still works |
| `duplicate_growth_ratio` | `persisted_records_for_same_identity_value / expected_unique_records` | Measures approximate-dedup drift under scale |
| `exact_dedup_score` | `1 / duplicate_growth_ratio` | Higher is better; `1.0` means exact dedup |

For the history-aware context contract proposed here, V2 should extend the harness with:

- `prompt_duplicate_rate`: `duplicate_prompt_items / prepared_prompt_items`
- `persisted_history_echo_rate`: `replayed_history_items_persisted_as_new / history_reconciliation_checks`

Performance metrics are also collected by benchmark:

- `BeforeTurn ns/op`
- `BeforeTurn allocs/op`
- `AfterTurn ns/op`
- `AfterTurn allocs/op`

Those are not strict CI gates by themselves, but they are mandatory regression indicators for V2.

### 9.3 Scenario Set

The implemented suite covers five deterministic scenarios:

1. `targeted_recall_and_retention`
   - Writes identity, preference, and a same-day task.
   - Measures targeted recall precision and recall before expiry.
   - Advances time, runs maintenance, then measures durable survival and expired-task suppression.
2. `idempotent_replay`
   - Replays the exact same `TurnID`.
   - Measures whether raw log and runtime context stay duplicate-free.
3. `duplicate_growth_under_scaled_fact_volume`
   - Uses a custom heuristic client that emits one stable fact plus many high-relevance distractors.
   - This intentionally exposes the current V1 weakness caused by dedup comparing against `loadRecallFacts`, which is ranked and capped.
4. `session_isolation`
   - Confirms the default engine does not leak recall from one session into another.
5. `compaction_guard`
   - Forces prompt pressure and verifies that compaction still writes `compact_summary`.

V2 should extend this suite with two more scenarios:

6. `caller_supplied_history_reconciliation`
   - Passes a mixed request that already includes historical items plus a new trailing turn.
   - Verifies that `BeforeTurn` emits no duplicate item identities and that `AfterTurn` persists only the current-turn delta.
7. `latest_only_vs_full_history_equivalence`
   - Populates canonical storage, then compares a latest-only request against a full-history request for the same semantic task.
   - Verifies that duplicate-free assembled context preserves the same required recall facts and recent continuity.

### 9.4 Current V1 Baseline

Measured on the current implementation with `go test ./agents/memory -run TestMemoryQuantitativeEvaluationBaseline -v`:

| Metric | Current V1 Baseline |
| --- | --- |
| `fact_recall_recall` | `1.0000` |
| `fact_recall_precision` | `1.0000` |
| `durable_fact_survival_rate` | `1.0000` |
| `expired_fact_suppression_rate` | `1.0000` |
| `idempotency_score` | `1.0000` |
| `session_isolation_leak_rate` | `0.0000` |
| `compaction_guard_score` | `1.0000` |
| `duplicate_growth_ratio` | `6.0000` |
| `exact_dedup_score` | `0.1667` |

Interpretation:

- V1 is already strong on recall correctness, isolation, compaction, expiry, and idempotency.
- V1 is weak on exact dedup under scaled fact volume.
- That means Phase 1 is not speculative. It is directly tied to the one measurable correctness gap in the current engine.
- The current baseline does not yet include `prompt_duplicate_rate` or `persisted_history_echo_rate`; those should be added when the history-reconciliation scenarios land.

Local benchmark reference captured with `go test ./agents/memory -run '^$' -bench 'BenchmarkMemoryEngine(BeforeTurn|AfterTurn)$' -benchmem -count=1`:

| Benchmark | Local Reference |
| --- | --- |
| `BenchmarkMemoryEngineBeforeTurn` | `221292 ns/op`, `94153 B/op`, `607 allocs/op` |
| `BenchmarkMemoryEngineAfterTurn` | `11164153 ns/op`, `4345875 B/op`, `34584 allocs/op` |

These performance numbers are environment-dependent. They are baseline references, not absolute pass or fail values.

### 9.5 V2 Acceptance Gates

V2 should only be accepted if it satisfies both of these conditions:

1. It does not regress V1 guardrails.
2. It materially improves the dedup-correctness metrics that V1 currently fails.

Required quality gates after the harness is extended with the history-reconciliation metrics:

- `fact_recall_recall >= 1.00`
- `fact_recall_precision >= 1.00`
- `durable_fact_survival_rate >= 1.00`
- `expired_fact_suppression_rate >= 1.00`
- `idempotency_score >= 1.00`
- `session_isolation_leak_rate <= 0.00`
- `compaction_guard_score >= 1.00`
- `exact_dedup_score >= 0.95`
- `duplicate_growth_ratio <= 1.05`
- `prompt_duplicate_rate <= 0.00`
- `persisted_history_echo_rate <= 0.00`

Required V2-vs-V1 comparison rule:

- For guardrails, V2 must be `>=` V1 on positive metrics and `<=` V1 on negative metrics.
- For dedup correctness, V2 must be strictly better than V1 and also satisfy the absolute gate above.

Performance guardrails for the hot path:

- `BeforeTurn ns/op <= 1.10 x V1 local baseline`
- `BeforeTurn allocs/op <= 1.10 x V1 local baseline`
- `AfterTurn ns/op <= 1.15 x V1 local baseline`
- `AfterTurn allocs/op <= 1.15 x V1 local baseline`

Rationale:

- Phase 1 may increase write-path work slightly because exact indexing is more expensive than approximate lookup.
- That cost is acceptable only within a bounded regression envelope.
- Phase 2 must keep semantic consolidation off the hot path, so it should not materially affect these benchmarks.

### 9.6 Commands

Baseline metric run:

```bash
go test ./agents/memory -run 'TestMemoryQuantitativeEvaluationBaseline|TestQuantitativeGatesRejectCurrentV1' -v
```

Hot-path benchmark run:

```bash
go test ./agents/memory -run '^$' -bench 'BenchmarkMemoryEngine(BeforeTurn|AfterTurn)$' -benchmem -count=1
```

When V2 exists, the same evaluation harness should be run against the V2 engine factory and compared against the V1 result set using the comparison helper already implemented in `eval_harness_test.go`.

## 10. Recommended Final Direction

The correct V2 direction is not to replace the current design with the Google reference design.

The correct direction is:

- keep the current system's boundedness, storage abstraction, and idempotent persistence,
- add Google's strongest principle, which is asynchronous semantic consolidation,
- make caller-provided history a first-class, duplicate-free input source rather than an external workaround,
- keep that consolidation minimal at first,
- avoid Google's weakest tradeoff, which is loose boundedness and oversimplified retrieval,
- defer platform-scale expansion until the new loop proves its value with metrics.

In one sentence:

V2 should become a hybrid of a safe turn-time memory adapter and a minimal always-on consolidation engine.

## 11. Bottom Line

The current `agents/memory` implementation is already a strong foundation for production-grade agent memory because it gets the hard systems problems mostly right: bounded context assembly, pluggable storage, append-first durability, lifecycle tiers, and migration compatibility.

What it lacks is not engineering discipline. It lacks a second loop.

Google's always-on memory agent contributes exactly that second loop: a background process that keeps turning raw observations into higher-order memory.

That is the right next step for this repository, but only in a narrowed form.

The highest-confidence V2 is:

- exact memory-correctness hardening first,
- explicit caller-context reconciliation inside the same correctness pass,
- minimal offline consolidation second,
- everything else only after measurement proves it is necessary.
