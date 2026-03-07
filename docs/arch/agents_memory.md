# Agents Memory Technical Architecture

## Menu

- [Agents Memory Technical Architecture](#agents-memory-technical-architecture)
  - [Menu](#menu)
  - [1. Background and Purpose](#1-background-and-purpose)
  - [2. System Scope and Non-Goals](#2-system-scope-and-non-goals)
    - [2.1 In-Scope](#21-in-scope)
    - [2.2 Out-of-Scope](#22-out-of-scope)
  - [3. Architecture Overview](#3-architecture-overview)
    - [3.1 Layered Design](#31-layered-design)
    - [3.2 Runtime Data Flow (High-Level)](#32-runtime-data-flow-high-level)
    - [3.3 Canonical Storage Topology](#33-canonical-storage-topology)
  - [4. Core Design Principles](#4-core-design-principles)
    - [4.1 Bounded Prompt Assembly](#41-bounded-prompt-assembly)
    - [4.2 Incremental, Append-First Persistence](#42-incremental-append-first-persistence)
    - [4.3 Tiered Fact Semantics](#43-tiered-fact-semantics)
    - [4.4 Compatibility During Migration](#44-compatibility-during-migration)
  - [5. Turn Lifecycle Internals](#5-turn-lifecycle-internals)
    - [5.1 `BeforeTurn` Pipeline](#51-beforeturn-pipeline)
    - [5.2 `AfterTurn` Pipeline](#52-afterturn-pipeline)
  - [6. Memory Fact Model and Recall Algorithms](#6-memory-fact-model-and-recall-algorithms)
    - [6.1 Fact Shape](#61-fact-shape)
    - [6.2 Rule-Based Extraction](#62-rule-based-extraction)
    - [6.3 Tier and Expiry Semantics](#63-tier-and-expiry-semantics)
    - [6.4 Recall Scoring](#64-recall-scoring)
    - [6.5 Delta Upsert Rule](#65-delta-upsert-rule)
  - [7. Memory Reference Injection and Prompt Safety](#7-memory-reference-injection-and-prompt-safety)
  - [8. Storage Contract and Backend Behavior](#8-storage-contract-and-backend-behavior)
    - [8.1 Storage Interface Contract](#81-storage-interface-contract)
    - [8.2 Local Backend (`storage/local`)](#82-local-backend-storagelocal)
    - [8.3 MCP Backend (`storage/mcp`)](#83-mcp-backend-storagemcp)
  - [9. Session Scaffold and Metadata Model](#9-session-scaffold-and-metadata-model)
  - [10. Maintenance Pipeline Details](#10-maintenance-pipeline-details)
  - [11. Heuristic LLM Integration](#11-heuristic-llm-integration)
    - [11.1 Activation](#111-activation)
    - [11.2 Request/Response Pattern](#112-requestresponse-pattern)
  - [12. Error Handling and Reliability Semantics](#12-error-handling-and-reliability-semantics)

## 1. Background and Purpose

`agents/memory` is the long-horizon memory subsystem for agent workflows in this repository.
It targets one core problem: **how to preserve useful context across turns and sessions while keeping runtime prompts bounded, queryable, and operationally maintainable**.

From the implementation, this subsystem is designed to provide four guarantees:

1. **Continuity**: carry recent runtime context and recallable facts into each new turn.
2. **Control**: keep prompt size bounded through context compaction and selective recall.
3. **Durability**: persist immutable interaction traces and structured facts in storage backends.
4. **Operability**: expose maintenance and directory-summary capabilities for lifecycle management.

The architecture intentionally separates:

- Turn lifecycle orchestration (`BeforeTurn`, `AfterTurn`)
- Storage abstraction (`agents/memory/storage.Engine`)
- Runtime maintenance (`RunMaintenance`, directory summaries)
- Optional heuristic enhancement (OpenAI-compatible Responses tool extraction)

This separation allows the same memory behavior to run on both local filesystem storage and MCP-backed remote storage, while preserving a stable lifecycle API.

Two API fields already exist for future expansion but are currently unused by the engine implementation:

- `BeforeTurnInput.BaseInstructions`
- `BeforeTurnInput.UserID` / `AfterTurnInput.UserID`

## 2. System Scope and Non-Goals

### 2.1 In-Scope

- Preparing model input with:
    - injected memory reference block,
    - recent context items,
    - current turn input.
- Persisting turn deltas and model outputs as append-only events.
- Extracting and recalling memory facts through rule-based logic and optional heuristic extraction.
- Tier-based retention (`L0`, `L1`, `L2`) with expiration/sweep.
- Compaction and archival routines via management APIs.

### 2.2 Out-of-Scope

- Strong transactional guarantees across multiple files.
- Conflict-free multi-writer coordination across independent processes.
- Semantic/vector retrieval quality guarantees from storage `Search` backends.
- Privacy policy or domain-specific PII classification beyond current extraction rules/prompts.

## 3. Architecture Overview

### 3.1 Layered Design

The system is organized in four layers:

1. **Agent lifecycle API layer** (`memory.Engine`, `memory.Management`)
   Defines how callers integrate memory into one turn and maintenance flow.
2. **Orchestration layer** (`StandardEngine`)
   Implements turn-time read/recall/assemble and post-turn persistence/fact updates.
3. **Storage contract layer** (`agents/memory/storage.Engine`)
   Normalizes file operations (`Read/Write/Stat/List/Search/Delete`) across backends.
4. **Storage backend layer**
   Local filesystem plugin and MCP-backed plugin.

### 3.2 Runtime Data Flow (High-Level)

For each turn:

1. `BeforeTurn` loads runtime context and recall facts (tiered with legacy fallback).
2. It optionally performs storage search using current input text as query.
3. It constructs one developer memory reference block and concatenates:
    - memory block,
    - recent context,
    - current input.
4. If estimated token usage crosses a threshold relative to `MaxInputTok`, runtime context compaction is attempted.
5. `AfterTurn` persists turn events, extracts facts, applies tier/retention policy, writes tier shards, and updates metadata idempotently via `processed_turn_ids`.

### 3.3 Canonical Storage Topology

Per session (`/memory/{session_id}`), the canonical topology is:

- `/meta/` (state, policy, watermarks)
- `/runtime/context/current.jsonl`
- `/events/raw/...` (append-only event shards)
- `/events/compact/...` (compaction/archive summary events)
- `/events/archive/...` (compressed historical shards)
- `/memory_tiers/L0|L1|L2/...` (tiered fact shards)

The engine currently maintains **compatibility reads/writes** for legacy files during migration (`log.jsonl`, `context.jsonl`, `memory_facts.jsonl`, `meta.json`).

## 4. Core Design Principles

### 4.1 Bounded Prompt Assembly

Prompt construction is bounded by:

- hard limits on recalled fact count and search chunk count,
- clipping/truncation for recalled chunk text,
- runtime-context compaction when token estimate approaches threshold.

### 4.2 Incremental, Append-First Persistence

- Turn events and facts are primarily appended to shard files.
- Rewrite operations are reserved for maintenance (compaction and expiry sweeping).
- Metadata updates capture lifecycle watermarks and idempotency state.

### 4.3 Tiered Fact Semantics

- `L0`: durable identity/preference style facts.
- `L1`: short-lived facts (daily horizon by default).
- `L2`: medium-lived facts (weekly horizon by default).

### 4.4 Compatibility During Migration

Read paths and write paths preserve legacy compatibility to avoid abrupt cutover risk while canonical layout becomes primary.

## 5. Turn Lifecycle Internals

### 5.1 `BeforeTurn` Pipeline

`BeforeTurn` executes the following deterministic sequence:

1. Validate required fields (`project`, `session_id`, `turn_id`, non-empty `current_input`).
2. Load context events from canonical runtime context; if empty, fallback to legacy context.
3. Build recall query from `CurrentInput` text and output fields.
4. Load active memory facts from tier shards (`L0`, `L2`, `L1` scan order) with expiry filtering.
5. If no tier facts exist, fallback to legacy `memory_facts.jsonl`.
6. Rank and deduplicate facts, then cap by `RecallFactsLimit`.
7. Optionally run storage `Search` (best effort; errors are swallowed and do not fail `BeforeTurn`).
8. Build one developer memory-reference item containing facts and search chunk snippets.
9. Append recent context items (bounded by `RecentContextItems`) and then current input.
10. Estimate token count; if above `CompactThreshold * MaxInputTok`, attempt runtime compaction and rebuild assembled input.

Important implementation note: storage search is limited to the current session subtree (`/memory/{session_id}`), so recall search is session-local rather than global across a project namespace.

Output includes:

- `InputItems`: `[memory_block?] + recent_context + current_input`
- `RecallFactIDs`: IDs of recalled facts included in memory block
- `ContextTokenCount`: conservative estimate (`chars/4`)

### 5.2 `AfterTurn` Pipeline

`AfterTurn` is idempotent by `TurnID` and follows this sequence:

1. Validate required fields (`project`, `session_id`, `turn_id`).
2. Ensure session scaffold exists (policy, summaries, watermarks).
3. Load metadata (canonical first, then legacy fallback).
4. If `turn_id` already exists in `processed_turn_ids`, return success with no writes.
5. Normalize persisted turn input via `prepareTurnInputForPersist`:
    - remove injected `<memory_reference>` items,
    - strip leading recalled context prefix when caller passed `BeforeTurn` output wholesale.
6. Build immutable `input_item`/`output_item` log events.
7. Append events to canonical raw shard and runtime context; also dual-write legacy log/context.
8. Extract candidate facts via rule-based parser and optional heuristic client.
9. Apply tier policy (`expires_at`) and `source_turn_id`, then delta-upsert filter against existing facts.
10. Append tiered fact shards and dual-write legacy facts when there are effective deltas.
11. Update metadata (`latest_turn_id`, bounded `processed_turn_ids`, `updated_at`) and persist canonical + legacy meta.

Important implementation note: the delta-upsert comparison currently uses `loadRecallFacts`, which returns a ranked and capped active-fact subset rather than the full active fact corpus. This keeps turn-time cost bounded, but it also means duplicate-prevention is approximate once stored fact volume grows beyond `RecallFactsLimit`.

## 6. Memory Fact Model and Recall Algorithms

### 6.1 Fact Shape

A fact record is modeled as:

- Identity fields: `fact_id`, `key`
- Value fields: `value`, `confidence`
- Lifecycle fields: `tier`, `expires_at`, `ts`, `source_turn_id`
- Write metadata: `id`, `type=fact_upsert`

The effective dedup identity is normalized `fact_id::key`.

### 6.2 Rule-Based Extraction

Built-in extractor currently recognizes simple English patterns in lowercase flattened text:

- `my name is ...` -> `user_name` (`L0`, confidence `0.95`)
- `i prefer ...` -> `user_preference` (`L0`, confidence `0.92`)
- `i like ...` -> `user_like` (`L2`, confidence `0.85`)
- `today i need ...` -> `today_task` (`L1`, confidence `0.80`)
- `this week i need ...` -> `weekly_task` (`L2`, confidence `0.82`)

### 6.3 Tier and Expiry Semantics

- `L0`: no `expires_at` by default.
- `L1`: `expires_at` at UTC day boundary + `L1RetentionDays`.
- `L2`: `expires_at` at UTC day boundary + `L2RetentionDays`.
- Unknown tiers are normalized to `L2`.

### 6.4 Recall Scoring

Recall ranking combines confidence, tier bias, recency, and query relevance:

- Base: `confidence`
- Tier bias: `L0 +0.35`, `L2 +0.20`, `L1 +0.10`
- Recency decay: `max(0, 0.25 - hours_since_ts/240)`
- Relevance: if query terms match fact text, add `0.25 + 0.15 * matched_terms`

Query tokenizer lowercases, removes punctuation, filters short tokens and stop words, and deduplicates terms.

### 6.5 Delta Upsert Rule

Candidate facts are persisted only when one of the following is true:

- no existing active fact for same identity,
- existing fact is expired,
- normalized value changed,
- tier changed.

Otherwise the write is skipped to prevent repetitive fact growth.

This rule is exact only within the fact set loaded for comparison during `AfterTurn`; because that lookup is capped by recall limits, it is best described as bounded delta-upsert rather than full-corpus deduplication.

## 7. Memory Reference Injection and Prompt Safety

The memory block is injected as one `developer` message with explicit boundary tags:

- opening tag: `<memory_reference>`
- mandatory disclaimer warning that memory may be outdated
- recalled lines (`Fact[...]`, `Recall[path:offset-range] ...`)
- closing tag: `</memory_reference>`

Prompt-safety controls for search chunks:

- clip around hit offsets to bounded window (`maxRecallChunkChars`),
- try extracting JSON `text` fields from JSON-like payloads,
- truncate by rune length, preserving UTF-8 safety,
- avoid injecting entire large blobs.

The compacted runtime context itself is not semantically summarized. Compaction replaces older runtime events with a single synthetic `compact_summary` event that records only the fact that compaction happened and how many events were removed.

## 8. Storage Contract and Backend Behavior

### 8.1 Storage Interface Contract

The memory engine depends on six storage methods:

- `Read`
- `Write`
- `Stat`
- `List`
- `Search`
- `Delete`

Write modes are normalized as `APPEND`, `OVERWRITE`, `TRUNCATE`.

### 8.2 Local Backend (`storage/local`)

The local backend uses an `os.Root` sandbox under per-project namespaces:

- project must match `^[A-Za-z0-9_.-]{1,128}$`
- storage path must be canonical absolute path, max length `512`
- non-root operations reject empty/root path when not allowed
- symlinks are skipped during traversal/search
- list/search enforce bounded defaults and hard caps

Search characteristics (local backend):

- substring match, case-insensitive
- skips files larger than `4MB`
- returns full file content in chunk payload with first-match byte range

### 8.3 MCP Backend (`storage/mcp`)

MCP backend is an adapter over `agents/files` MCP storage:

- can use prebuilt `Caller` or endpoint/api-key bootstrap
- if `Caller` is provided, endpoint/api-key are optional
- if `Caller` is absent, both `Endpoint` and `APIKey` are mandatory

## 9. Session Scaffold and Metadata Model

`ensureSessionScaffold` ensures baseline files for each session:

- `meta/policy.json` from runtime config defaults
- `meta/watermarks.json`
- `.abstract` and `.overview` for known managed directories

Current implementation note: `meta/watermarks.json` is scaffolded with an `updated_at` field but is not otherwise advanced by maintenance or turn-time flows yet.

Metadata (`meta/state.json`, dual-written to legacy `meta.json`) tracks:

- `version`
- `latest_turn_id`
- `processed_turn_ids` (bounded by `MaxProcessedTurns`)
- `last_compact_at`
- `last_maintenance_at`
- `updated_at`

Backward compatibility includes support for legacy `processed_turn` field name during meta load.

## 10. Maintenance Pipeline Details

`RunMaintenance` orchestrates the operational lifecycle:

1. Validate project/session and ensure scaffold.
2. Compact runtime context if context event count exceeds `2 * RecentContextItems`.
3. Archive old raw shards:
    - parse date from raw shard path,
    - skip shards newer than `CompactionMinAge`,
    - compress old shard body via zstd,
    - write to archive path and delete raw shard,
    - append `compact_summary` event describing archive action.
4. Sweep expired facts in `L1` then `L2` tiers by rewriting or deleting affected shard files.
5. Refresh `.abstract` and `.overview` for known directories.
6. Update `last_maintenance_at` in metadata.

`ListDirWithAbstract` provides management-facing discovery by listing directories and guaranteeing `.abstract` availability for each returned directory.

Current implementation note: directory summaries are deterministic filesystem summaries, not LLM-generated narratives. `.abstract` is a static template per directory, while `.overview` is a JSON-like snapshot rendered from entry counts, byte totals, and sample paths.

## 11. Heuristic LLM Integration

### 11.1 Activation

Heuristic extraction is enabled when either:

- `Config.HeuristicClient` is provided directly, or
- `LLMAPIBase` and `LLMAPIKey` are provided so engine builds an OpenAI-compatible client.

### 11.2 Request/Response Pattern

The built-in client:

- normalizes API base into `/v1/responses`,
- posts a Responses request with tool schema `extract_and_merge_memories`,
- enforces response body size cap,
- recursively extracts tool arguments from response payload,
- normalizes produced facts and deduplicates by `fact_id + key`.

Heuristic and rule-based facts are merged, with heuristic candidates taking precedence on identity collisions.

Current implementation note: the tool schema includes `deleted_fact_ids`, but the engine currently ignores deletion instructions and only persists normalized upsert facts.

## 12. Error Handling and Reliability Semantics

Reliability model in current implementation:

- Validation failures return typed `ValidationError` with stable codes.
- Most storage/read/write failures are wrapped and returned immediately.
- Search failures inside `BeforeTurn` are non-fatal by design (best-effort retrieval).
- Compaction failure during `BeforeTurn` does not fail the turn; request proceeds with current assembly.
- Meta update inside compaction path is best effort (`loadMeta` failure is tolerated).

Operational implications:

- The design favors turn availability over strict maintenance consistency.
- Idempotency at `AfterTurn` protects against duplicate writes under retries for same `TurnID`.
- Without external transaction support, partial multi-file persistence remains possible during abrupt failures.
- The system already has the structure for richer lifecycle management, but today it does not perform autonomous semantic consolidation across multiple memories; maintenance is operational, and semantic extraction remains turn-local.
