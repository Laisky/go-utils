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
  - [6. Recommended V2 Principles](#6-recommended-v2-principles)
    - [6.1 Principle 1: Separate online recall from offline consolidation](#61-principle-1-separate-online-recall-from-offline-consolidation)
    - [6.2 Principle 2: Introduce an observation layer](#62-principle-2-introduce-an-observation-layer)
    - [6.3 Principle 3: Make deduplication exact where it must be exact](#63-principle-3-make-deduplication-exact-where-it-must-be-exact)
    - [6.4 Principle 4: Add memory state transitions](#64-principle-4-add-memory-state-transitions)
    - [6.5 Principle 5: Keep bounded prompt assembly as a hard invariant](#65-principle-5-keep-bounded-prompt-assembly-as-a-hard-invariant)
    - [6.6 Principle 6: Make consolidation policy-driven](#66-principle-6-make-consolidation-policy-driven)
  - [7. Recommended V2 Architecture](#7-recommended-v2-architecture)
    - [7.1 Proposed Layers](#71-proposed-layers)
    - [7.2 Proposed Record Families](#72-proposed-record-families)
    - [7.3 Proposed Storage Topology](#73-proposed-storage-topology)
  - [8. Upgrade Roadmap](#8-upgrade-roadmap)
    - [8.1 Phase 1: Correctness Hardening](#81-phase-1-correctness-hardening)
    - [8.2 Phase 2: Observation Layer](#82-phase-2-observation-layer)
    - [8.3 Phase 3: Background Consolidation](#83-phase-3-background-consolidation)
    - [8.4 Phase 4: Recall Policy Engine](#84-phase-4-recall-policy-engine)
    - [8.5 Phase 5: Multimodal and External Memory Ingestion](#85-phase-5-multimodal-and-external-memory-ingestion)
  - [9. Recommended Final Direction](#9-recommended-final-direction)
  - [10. Bottom Line](#10-bottom-line)

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

### 6.2 Principle 2: Introduce an observation layer

Add a first-class record type between raw events and facts.

Suggested shape:

- `observation_id`
- `source_turn_id`
- `summary`
- `raw_ref`
- `entities`
- `topics`
- `importance`
- `status` such as `new`, `consolidated`, `superseded`

Facts should become one output of memory processing, not the only output.

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

## 7. Recommended V2 Architecture

### 7.1 Proposed Layers

V2 should evolve into five layers:

1. Turn lifecycle layer.
2. Online recall assembly layer.
3. Offline consolidation layer.
4. Exact active-memory index layer.
5. Storage backend layer.

### 7.2 Proposed Record Families

V2 should store at least five record families:

1. Raw events
2. Runtime context snapshots
3. Observations
4. Facts
5. Insights and links

Optional later additions:

1. Memory deletion tombstones
2. Contradiction records
3. Recall feedback metrics

### 7.3 Proposed Storage Topology

The current topology can be extended rather than replaced:

- `/events/raw/...`
- `/runtime/context/...`
- `/memory_tiers/...`
- `/observations/raw/...`
- `/observations/compact/...`
- `/insights/...`
- `/indexes/active_facts.json` or equivalent shard set
- `/indexes/active_observations.json` or equivalent shard set

The pluggable storage contract can remain intact.

## 8. Upgrade Roadmap

### 8.1 Phase 1: Correctness Hardening

Objective: remove the most important architectural debt without changing the public model too much.

Deliverables:

- Add an exact active-fact index for `AfterTurn` deduplication.
- Implement first-class delete and supersede semantics.
- Advance `watermarks.json` meaningfully.
- Add metrics for compaction, recall count, dedupe decisions, and consolidation lag.
- Make search scope configurable: session, user, project.

Expected outcome:

- preserve current ergonomics,
- eliminate approximate dedup as the main correctness risk.

### 8.2 Phase 2: Observation Layer

Objective: stop collapsing raw turn text directly into facts.

Deliverables:

- Introduce observation records.
- Extract entities, topics, and importance per observation.
- Keep current fact extraction, but derive it from observations.
- Let `BeforeTurn` optionally recall observations and facts differently.

Expected outcome:

- richer memory products,
- better support for later consolidation,
- cleaner separation between episodic and semantic memory.

### 8.3 Phase 3: Background Consolidation

Objective: import the strongest principle from the Google design.

Deliverables:

- Add a background consolidation API or scheduler.
- Consolidate unconsolidated observations into insights.
- Create explicit links between related observations and facts.
- Mark observation state transitions after consolidation.

Expected outcome:

- memory quality can improve without waiting for another user turn,
- the system gains reconstructive memory behavior.

### 8.4 Phase 4: Recall Policy Engine

Objective: move recall from simple ranking to policy-driven assembly.

Deliverables:

- Different recall budgets for facts, observations, and insights.
- Scope rules by session, user, or project.
- Freshness and trust weighting.
- Optional contradiction suppression or warning behavior.

Expected outcome:

- recall stays bounded while becoming more expressive.

### 8.5 Phase 5: Multimodal and External Memory Ingestion

Objective: adopt the broader ingestion capabilities demonstrated by Google's reference.

Deliverables:

- external ingestion endpoints,
- file and document adapters,
- observation extraction from non-text artifacts,
- source-aware retention and recall.

Expected outcome:

- the subsystem becomes an actual memory platform rather than only a chat-memory helper.

## 9. Recommended Final Direction

The correct V2 direction is not to replace the current design with the Google reference design.

The correct direction is:

- keep the current system's boundedness, storage abstraction, and idempotent persistence,
- add Google's strongest principle, which is asynchronous semantic consolidation,
- avoid Google's weakest tradeoff, which is loose boundedness and oversimplified retrieval.

In one sentence:

V2 should become a hybrid of a safe turn-time memory adapter and an always-on consolidation engine.

## 10. Bottom Line

The current `agents/memory` implementation is already a strong foundation for production-grade agent memory because it gets the hard systems problems mostly right: bounded context assembly, pluggable storage, append-first durability, lifecycle tiers, and migration compatibility.

What it lacks is not engineering discipline. It lacks a second loop.

Google's always-on memory agent contributes exactly that second loop: a background process that keeps turning raw observations into higher-order memory.

That is the right next step for this repository.
