# General-purpose Golang Memory Library Technical Manual (Tiered Memory + Pluggable Storage Engines)

## 1. Document Purpose

This document defines the target architecture and implementation plan for the Go memory SDK used by chat agents.
The design is optimized for these requirements:

1. Tiered memory retention (L0/L1/L2 and beyond)
2. Full interaction logging (input/output/timestamp)
3. Background compaction and archival to control file growth
4. Hierarchical storage with per-folder `.abstract` and `.overview`

The design is based on:

1. Golang
2. OneAPI Responses API (`https://oneapi.laisky.com/v1/responses`) as the model interface
3. Pluggable storage engines (`agents/memory/storage`) with MCP and local plugins

## 2. Executive Summary

A feasible and low-risk implementation path is:

1. Keep the current `BeforeTurn` and `AfterTurn` lifecycle, and extend it incrementally
2. Treat interaction logs as immutable source of truth, never edited in place
3. Add a tier classifier to write memory facts into `L0`, `L1`, `L2` shards
4. Move compaction and folder-summary generation to asynchronous maintenance workers
5. Add a memory-specific directory listing API that returns folder path plus `.abstract`

No blocking technical unknowns were found. The main tradeoff is eventual consistency for `.overview` and compaction outputs, which is acceptable for memory systems.

## 3. Feasibility Assessment Against Current Implementation

Current implementation in `agents/memory/engine.go` already provides:

1. Append-only `log.jsonl` and `context.jsonl`
2. Structured memory facts in `memory_facts.jsonl`
3. Idempotent turn writes through `meta.processed_turn_ids`
4. Synchronous context compaction when token threshold is exceeded

Gaps relative to the new requirements:

1. No tiered memory files (`L0/L1/L2`)
2. Logs are present but not partitioned for long-term operations
3. Compaction is synchronous and only rewrites active context
4. No directory-level `.abstract` and `.overview`
5. `List` does not return folder intent summary

Conclusion: the requested capabilities are feasible without breaking the existing SDK integration model.

## 4. Goals and Non-goals

### 4.1 Goals

1. Maintain compatibility with `BeforeTurn` and `AfterTurn`
2. Introduce tiered memory storage with configurable retention
3. Store complete interaction logs for all turns
4. Add asynchronous compact/archive pipeline
5. Support hierarchical folder summaries (`.abstract` and `.overview`)
6. Provide `list_dir` results enriched with `.abstract`

### 4.2 Non-goals

1. Changing business tool orchestration logic in the agent
2. Building a distributed transaction layer
3. Guaranteeing strict real-time consistency for generated summaries

## 5. Target Architecture

```mermaid
flowchart TD
    A[Agent Workflow] --> B[Memory Engine\nBeforeTurn/AfterTurn]
    B --> C[Storage Adapter\nMCP Plugin / Local Plugin]

    B --> D[Sync Write Path]
    D --> E1[/events/raw/.../log-*.jsonl]
    D --> E2[/memory_tiers/L0|L1|L2/*.jsonl]
    D --> E3[/runtime/context/current.jsonl]
    D --> E4[/meta/state.json]

    B --> F[Async Maintenance Workers]
    F --> G1[Compactor]
    F --> G2[Retention Sweeper]
    F --> G3[Folder Summary Builder]

    G1 --> H1[/events/compact/...]
    G1 --> H2[/events/archive/.../*.zst]
    G2 --> H3[Delete expired L1/L2 records]
    G3 --> H4[Write .abstract and .overview]
```

### 5.1 Design Principles

1. Keep write path minimal and deterministic
2. Push heavy tasks (compaction, long summaries) to background workers
3. Preserve full traceability through immutable logs
4. Use UTC timestamps only
5. Keep retention policy configurable via metadata file

## 6. Directory and Layered Storage Specification

### 6.1 Session Root Layout

```text
/memory/{session_id}/
  /.abstract
  /.overview
  /meta/
    /.abstract
    /.overview
    /state.json
    /policy.json
    /watermarks.json
  /events/
    /.abstract
    /.overview
    /raw/
      /YYYY/
        /MM/
          /DD/
            log-0001.jsonl
            log-0002.jsonl
    /compact/
      /YYYY/
        /MM/
          /DD/
            compact-<ts>.jsonl
    /archive/
      /YYYY/
        /MM/
          /DD/
            log-<range>.jsonl.zst
  /memory_tiers/
    /.abstract
    /.overview
    /L0/
      /.abstract
      /.overview
      /YYYY/
        /MM/
          facts-<date>.jsonl
    /L1/
      /.abstract
      /.overview
      /YYYY/
        /MM/
          facts-<date>.jsonl
    /L2/
      /.abstract
      /.overview
      /YYYY/
        /WW/
          facts-<week>.jsonl
  /runtime/
    /.abstract
    /.overview
    /context/
      current.jsonl
      latest_compact_pointer.json
```

### 6.2 Folder Summary Files

Each directory managed by memory must contain:

1. `.abstract`: 100-200 words, concise purpose and what data exists in that folder
2. `.overview`: up to 2000 words, rolling summary of all major records under that folder

Rules:

1. Summaries are generated in English
2. Summaries are eventually consistent (not updated on every single write)
3. On missing files, create placeholders first and schedule async refresh
4. Never include secrets in summaries

### 6.3 Tier Semantics

1. `L0`: permanent memory, never auto-deleted (except explicit compliance deletion)
2. `L1`: short-lived memory, daily cleanup
3. `L2`: medium-lived memory, weekly cleanup

Recommended default mapping:

1. Identity, stable preferences, durable constraints -> `L0`
2. Daily plans, temporary intent, short tasks -> `L1`
3. Ongoing weekly tasks, medium-lived context -> `L2`

## 7. Data Model

### 7.1 Common Event Envelope

```json
{"id":"evt_001","ts":"2026-02-19T12:00:00Z","session_id":"s1","turn_id":"t1","kind":"input_item","payload":{}}
```

Required fields:

1. `id`: unique event id
2. `ts`: RFC3339 UTC timestamp
3. `session_id`: session key
4. `turn_id`: turn id for idempotency and trace
5. `kind`: event type
6. `payload`: event payload

### 7.2 Interaction Log Event Types

1. `input_item`
2. `output_item`
3. `tool_call`
4. `tool_result`
5. `compact_summary`
6. `fact_upsert`
7. `fact_delete`

### 7.3 Tiered Fact Record

```json
{
  "id":"fact_evt_01",
  "ts":"2026-02-19T12:05:00Z",
  "type":"fact_upsert",
  "fact_id":"user_style",
  "key":"reply_style",
  "value":"concise",
  "confidence":0.93,
  "tier":"L0",
  "expires_at":"",
  "source_turn_id":"t1"
}
```

Tier rules:

1. `tier` must be one of `L0`, `L1`, `L2`
2. `expires_at` is required for `L1` and `L2`, empty for `L0`
3. `expires_at` must be computed in UTC

## 8. API and Interface Evolution

### 8.1 Engine Interface

Keep existing interface unchanged for compatibility:

```go
type Engine interface {
    BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error)
    AfterTurn(ctx context.Context, in AfterTurnInput) error
}
```

### 8.2 Optional Management Interface

Add a memory-specific management interface for maintenance and directory discovery:

```go
type DirectorySummary struct {
    Path       string
    Abstract   string
    UpdatedAt  string
    HasOverview bool
}

type Management interface {
    RunMaintenance(ctx context.Context, project, sessionID string) error
    ListDirWithAbstract(ctx context.Context, project, sessionID, path string, depth, limit int) ([]DirectorySummary, error)
}
```

Rationale:

1. Avoid breaking generic storage interfaces
2. Satisfy `list_dir` requirement by enriching directory entries with `.abstract`

## 9. Runtime Flows

### 9.1 BeforeTurn (Read Path)

Execution order:

1. Read `meta/state.json` and `runtime/context/current.jsonl`
2. Load non-expired facts from `L0`, `L1`, `L2`
3. Search recent compact summaries and relevant raw logs
4. Build memory block in priority order: `L0` -> `L2` -> `L1` -> retrieved log chunks
5. Build final model input items: memory block + recent context + current input
6. If projected tokens exceed threshold, prefer using latest compact snapshot pointer

### 9.2 AfterTurn (Write Path)

Execution order:

1. Append all turn interaction records to `/events/raw/YYYY/MM/DD/log-*.jsonl`
2. Append rehydratable items to `/runtime/context/current.jsonl`
3. Extract candidate facts from current turn
4. Classify each fact into `L0/L1/L2` and write to the corresponding tier shard
5. Update `meta/state.json` watermarks and idempotency fields
6. Enqueue async jobs for compaction and summary refresh

### 9.3 Tier Classification

Tier classification is a heuristic task. It will be handled by the standardized LLM request client (see Section 10) to evaluate the importance of the memory and assign it to the appropriate tier (`L0`, `L1`, or `L2`).

1. Invoke the LLM with the `classify_memory` tool.
2. Use the LLM's structured output to determine the tier and expiration.
3. Keep a deterministic rule-based fallback (e.g., default to `L2`) in case the LLM call fails or times out.

## 10. Heuristic Memory Processing via LLM (OpenAI API)

### 10.1 The Need for Heuristic Processing

Tasks such as determining the importance of a memory, extracting key information from raw interactions, and merging new key information into existing memory are inherently heuristic. They cannot be effectively implemented using fixed, rule-based code. Instead, they require the cognitive capabilities of an LLM.

### 10.2 Standardized OpenAI Request Client

To facilitate heuristic tasks, the SDK will implement a standardized internal client for the OpenAI Responses API (`https://oneapi.laisky.com/v1/responses`).
This client will:

1. Handle authentication, retries, and rate limiting.
2. Support structured outputs via Function Calling / Tools.
3. Accept dynamic `system_prompt` and `tools` configurations based on the specific heuristic task.

### 10.3 Defined Heuristic Tasks

For each heuristic task, we define a specific system prompt and tool structure:

#### 10.3.1 Memory Importance and Tier Classification

1. **Trigger**: During `AfterTurn` or background processing when new facts are extracted.
2. **System Prompt**: "You are a memory classification assistant. Analyze the provided interaction and determine its long-term value. Classify it into L0 (permanent, identity/preferences), L1 (short-term, daily tasks), or L2 (medium-term, weekly projects)."
3. **Tool Structure**: A function `classify_memory` that accepts `tier` (enum: L0, L1, L2), `confidence_score` (float), and `expires_at` (string, RFC3339).

#### 10.3.2 Key Information Extraction

1. **Trigger**: Background compaction or periodic review of raw logs.
2. **System Prompt**: "You are an information extraction assistant. Review the following raw interaction logs and extract key facts, user preferences, and project states. Ignore transient chatter."
3. **Tool Structure**: A function `extract_facts` that accepts an array of `facts`, each containing `fact_id`, `key`, `value`, and `context`.

#### 10.3.3 Memory Merging and Consolidation

1. **Trigger**: When new facts conflict with or update existing facts in L0/L1/L2.
2. **System Prompt**: "You are a memory consolidation assistant. Compare the new extracted facts with the existing memory facts. Resolve conflicts, update outdated information, and merge them into a cohesive state."
3. **Tool Structure**: A function `merge_memories` that accepts an array of `updated_facts` and `deleted_fact_ids`.

#### 10.3.4 Folder Summary Generation (`.abstract` and `.overview`)

1. **Trigger**: Asynchronous maintenance worker for folder summary builder.
2. **System Prompt**: "You are a directory summarization assistant. Based on the provided file contents and metadata, generate a concise abstract (100-200 words) and a detailed overview (up to 2000 words) describing the purpose and contents of this directory."
3. **Tool Structure**: A function `generate_summaries` that accepts `abstract` (string) and `overview` (string).

### 10.4 Execution Timing

1. **Synchronous (Blocking)**: Lightweight tasks like initial tier classification of a single fact during `AfterTurn` (if latency permits, otherwise fallback to rule-based and refine async).
2. **Asynchronous (Background)**: Heavy tasks like bulk information extraction from raw logs, memory merging, and folder summary generation will be handled by background maintenance workers to avoid blocking the agent's main workflow.

## 11. Background Compact and Archive Mechanism

### 11.1 Objectives

1. Prevent active files from unbounded growth
2. Preserve full traceability
3. Keep retrieval latency stable

### 11.2 Compaction Pipeline

1. Select sealed raw shards older than configured age
2. Deduplicate repeated payload blocks
3. Generate compact summary records and write to `/events/compact/...`
4. Compress old raw shards to `.zst` and move to `/events/archive/...`
5. Keep manifest in `meta/watermarks.json` for traceability

### 11.3 Retention Sweep

Run on daily schedule in UTC:

1. Delete `L1` records where `expires_at < now`
2. Delete `L2` records where `expires_at < now`
3. Never delete `L0` via sweeper
4. Process date boundaries as `[start, next_day_start)` to include entire last day

### 11.4 Failure Handling

1. Compaction is retryable and idempotent by shard key
2. On failure, keep raw shards untouched and retry later
3. Read path must continue using raw logs and existing context if compact outputs are stale

## 12. `list_dir` with `.abstract` Behavior

### 12.1 Functional Requirement

When listing memory directories, response must include each directory path and the corresponding `.abstract` content.

### 12.2 Execution Strategy

1. Call storage `List` to collect directories
2. For each directory, read `{dir}/.abstract`
3. Return `DirectorySummary{Path, Abstract, UpdatedAt, HasOverview}`
4. If `.abstract` is missing, return an empty abstract and enqueue summary generation

### 12.3 Performance Controls

1. Cache `.abstract` by `path + updated_at`
2. Limit synchronous reads per request
3. Truncate oversized abstracts at read time to safe length

## 13. Migration Plan from Current Layout

Current files:

1. `/memory/{session_id}/log.jsonl`
2. `/memory/{session_id}/context.jsonl`
3. `/memory/{session_id}/memory_facts.jsonl`
4. `/memory/{session_id}/meta.json`

Target migration steps:

1. Copy `log.jsonl` into `/events/raw/.../log-legacy.jsonl`
2. Copy `context.jsonl` into `/runtime/context/current.jsonl`
3. Reclassify `memory_facts.jsonl` entries into `L0/L1/L2` (default `L2` when unknown)
4. Convert `meta.json` into `/meta/state.json`
5. Generate initial `.abstract` and `.overview` for all directories
6. Enable dual-read compatibility for one release cycle, then remove legacy read path

## 14. Observability and Debugging

Required metrics:

1. `memory_after_turn_write_latency_ms`
2. `memory_before_turn_read_latency_ms`
3. `memory_compact_job_latency_ms`
4. `memory_compact_backlog_size`
5. `memory_tier_records_total{tier}`
6. `memory_summary_refresh_failures_total`

Required debug logs:

1. Session id, turn id, shard path, record counts
2. Compaction input/output size statistics
3. Tier classification decisions and confidence

Do not log sensitive raw content or secrets.

## 15. Concurrency and Consistency

1. Use single-writer-per-session for synchronous writes
2. Allow concurrent reads with watermark-based snapshot consistency
3. Use optimistic version field in `meta/state.json` for maintenance updates
4. Keep log appends immutable and idempotent by `(session_id, turn_id, event_index)`

## 16. Security and Compliance

1. `project` is strict tenant boundary
2. All path joins must be normalized to prevent cross-tenant traversal
3. Support explicit fact deletion and legal erasure workflows
4. Keep keys in environment variables only
5. Redact sensitive fields in summaries and logs

## 17. Testing Strategy

### 17.1 Unit Tests

1. Tier classification and `expires_at` calculation
2. Daily and weekly cleanup boundaries in UTC
3. JSONL encode/decode for event and fact schemas
4. `ListDirWithAbstract` fallback behavior

### 17.2 Integration Tests

1. End-to-end turn lifecycle with tiered fact writes
2. Compaction and archive pipeline on synthetic long sessions
3. Recovery from compaction failure without read-path breakage
4. Migration from legacy file layout to new layout

### 17.3 Regression Tests

1. 1k+ turn sessions with stable latency
2. Retrieval quality before and after compaction
3. Idempotency under retried `AfterTurn`

## 18. Implementation Milestones

1. M1 (1 week): new directory schema, metadata policy, and dual-write scaffolding
2. M2 (1 week): tier classifier + writes to `L0/L1/L2`
3. M3 (1 week): background compactor, archive, and retention sweeper
4. M4 (1 week): `.abstract` and `.overview` generators + `ListDirWithAbstract`
5. M5 (1 week): migration tooling, compatibility window, and stress validation

## 19. Acceptance Criteria

The design is accepted when all conditions are met:

1. Every interaction is logged with input/output/timestamp and traceable by turn
2. Tiered memory files are written and cleaned by policy (`L0` permanent, `L1` daily, `L2` weekly)
3. Background compaction reduces hot storage footprint without history loss
4. Every managed directory has `.abstract` and `.overview`
5. `list_dir` responses include directory path and `.abstract`
6. Read/write flow remains stable under retries and long sessions

## 20. Recommended Default Policy (`meta/policy.json`)

```json
{
  "version": 1,
  "timezone": "UTC",
  "tiers": {
    "L0": {"retention_days": 0, "auto_delete": false},
    "L1": {"retention_days": 1, "auto_delete": true},
    "L2": {"retention_days": 7, "auto_delete": true}
  },
  "compaction": {
    "enabled": true,
    "max_hot_shard_bytes": 8388608,
    "min_shard_age_hours": 24,
    "compression": "zstd"
  },
  "summary": {
    "abstract_words_min": 100,
    "abstract_words_max": 200,
    "overview_words_max": 2000,
    "refresh_interval_minutes": 60
  }
}
```

This policy-driven approach is the recommended implementation because it avoids hard-coded retention logic and makes future tier expansion straightforward.
