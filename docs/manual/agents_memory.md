# Agents Storage & Memory SDK Manual

This manual explains how to use:

- `agents/memory/storage`: standardized storage interface plus plugins
- `agents/memory`: tiered memory SDK on top of storage

The content matches the current implementation in `agents/memory`.

## Menu

- [Agents Storage \& Memory SDK Manual](#agents-storage--memory-sdk-manual)
  - [Menu](#menu)
  - [1) Why This Exists](#1-why-this-exists)
  - [2) Architecture Overview](#2-architecture-overview)
  - [3) Package APIs](#3-package-apis)
    - [3.1 Core engine](#31-core-engine)
    - [3.2 Management API](#32-management-api)
  - [4) Install and Import](#4-install-and-import)
  - [5) Storage Plugin Quick Start](#5-storage-plugin-quick-start)
    - [5.1 Storage parameter conventions](#51-storage-parameter-conventions)
    - [5.2 `storage.Engine` method parameters](#52-storageengine-method-parameters)
      - [`Read(ctx, project, path, offset, length) (string, error)`](#readctx-project-path-offset-length-string-error)
      - [`Write(ctx, project, path, content, mode, offset) error`](#writectx-project-path-content-mode-offset-error)
      - [`Stat(ctx, project, path) (FileInfo, error)`](#statctx-project-path-fileinfo-error)
      - [`List(ctx, project, path, depth, limit) ([]FileInfo, bool, error)`](#listctx-project-path-depth-limit-fileinfo-bool-error)
      - [`Search(ctx, project, query, pathPrefix, limit) ([]FileChunk, error)`](#searchctx-project-query-pathprefix-limit-filechunk-error)
      - [`Delete(ctx, project, path, recursive) error`](#deletectx-project-path-recursive-error)
    - [5.3 Local plugin config: `local.Config`](#53-local-plugin-config-localconfig)
    - [5.4 MCP plugin config: `mcpstorage.Config`](#54-mcp-plugin-config-mcpstorageconfig)
    - [5.5 Storage return model fields](#55-storage-return-model-fields)
      - [`FileInfo`](#fileinfo)
      - [`FileChunk`](#filechunk)
  - [6) Memory Quick Start](#6-memory-quick-start)
      - [`memory.NewEngine(storage, conf) (*memory.StandardEngine, error)`](#memorynewenginestorage-conf-memorystandardengine-error)
    - [6.1 Engine config reference: `memory.Config`](#61-engine-config-reference-memoryconfig)
    - [6.2 Turn lifecycle input/output parameters](#62-turn-lifecycle-inputoutput-parameters)
      - [`BeforeTurnInput`](#beforeturninput)
      - [`BeforeTurnOutput`](#beforeturnoutput)
      - [`AfterTurnInput`](#afterturninput)
    - [6.3 Message schema parameters](#63-message-schema-parameters)
      - [`ResponseItem`](#responseitem)
      - [`ResponseContentPart`](#responsecontentpart)
    - [6.4 Custom heuristic integration parameters](#64-custom-heuristic-integration-parameters)
      - [`ExtractAndMergeFacts(ctx, in) (HeuristicFactResult, error)`](#extractandmergefactsctx-in-heuristicfactresult-error)
      - [`HeuristicFactInput`](#heuristicfactinput)
      - [`HeuristicFactResult`](#heuristicfactresult)
  - [7) Maintenance and Directory Summary](#7-maintenance-and-directory-summary)
    - [7.1 Management API parameter reference](#71-management-api-parameter-reference)
      - [`RunMaintenance(ctx, project, sessionID) error`](#runmaintenancectx-project-sessionid-error)
      - [`RunConsolidation(ctx, project, sessionID) error`](#runconsolidationctx-project-sessionid-error)
      - [`ListDirWithAbstract(ctx, project, sessionID, path, depth, limit) ([]DirectorySummary, error)`](#listdirwithabstractctx-project-sessionid-path-depth-limit-directorysummary-error)
      - [`DirectorySummary`](#directorysummary)
  - [8) Data Layout (Canonical)](#8-data-layout-canonical)
  - [9) Compatibility Layout](#9-compatibility-layout)
  - [10) Runtime Behavior Details](#10-runtime-behavior-details)
    - [10.1 `BeforeTurn`](#101-beforeturn)
    - [10.1.1 Local storage backend operational limits](#1011-local-storage-backend-operational-limits)
    - [10.2 `AfterTurn`](#102-afterturn)
    - [10.3 Fact extraction rules](#103-fact-extraction-rules)
  - [11) Retention and Time Rules](#11-retention-and-time-rules)
  - [12) Testing and Quality](#12-testing-and-quality)
  - [13) Troubleshooting](#13-troubleshooting)
  - [14) Minimal Adoption Checklist](#14-minimal-adoption-checklist)
  - [15) V1 to V2 Migration](#15-v1-to-v2-migration)
    - [15.1 What changed](#151-what-changed)
    - [15.2 Recommended migration steps](#152-recommended-migration-steps)
    - [15.3 BeforeTurn migration example](#153-beforeturn-migration-example)
    - [15.4 AfterTurn migration example](#154-afterturn-migration-example)
    - [15.5 Mixed-mode rollout](#155-mixed-mode-rollout)


## 1) Why This Exists

Long-running agents need stable memory across turns and sessions.
This SDK provides:

1. Durable interaction history
2. Tiered long-term facts with retention
3. Exact active-fact indexing for write-side correctness
4. Runtime context compaction
5. Offline insight consolidation
6. Operational maintenance APIs and metrics

## 2) Architecture Overview

```mermaid
flowchart TD
    A[Agent Workflow] --> B[Memory Engine\nBeforeTurn / AfterTurn]
    B --> C[Storage Interface]
    C --> D1[MCP Plugin]
    C --> D2[Local Plugin]

    B --> E1[/events/raw/.../log-*.jsonl]
    B --> E2[/runtime/context/current.jsonl]
    B --> E3[/memory_tiers/L0|L1|L2/...]
    B --> E4[/indexes/active_facts.json]
    B --> E5[/insights/.../insights-*.jsonl]
    B --> E6[/meta/state.json + metrics.json + watermarks.json]

    F[RunMaintenance] --> G1[Compaction]
    F --> G2[Archive old raw shards]
    F --> G3[Sweep expired L1/L2 facts]
    F --> G4[Run consolidation]
    F --> G5[Refresh .abstract/.overview]
```

## 3) Package APIs

### 3.1 Core engine

`memory.Engine`:

1. `BeforeTurn(ctx, input)`
2. `AfterTurn(ctx, input)`

### 3.2 Management API

`*memory.StandardEngine` also implements `memory.Management`:

1. `RunMaintenance(ctx, project, sessionID)`
2. `RunConsolidation(ctx, project, sessionID)`
3. `ListDirWithAbstract(ctx, project, sessionID, path, depth, limit)`

Use it by interface assertion:

```go
var mgmt memory.Management = engine
```

## 4) Install and Import

If used from another module:

```bash
go get github.com/Laisky/go-utils/v6
```

Imports:

```go
import (
    "github.com/Laisky/go-utils/v6/agents/memory"
    "github.com/Laisky/go-utils/v6/agents/memory/storage/local"
    mcpstorage "github.com/Laisky/go-utils/v6/agents/memory/storage/mcp"
)
```

## 5) Storage Plugin Quick Start

Create local storage plugin (default for dev/debug):

```go
storage, err := local.NewEngine(local.Config{
    RootDir: "/tmp/agent-memory-storage",
})
if err != nil {
    panic(err)
}
defer storage.Close()
```

Create MCP storage plugin:

```go
ctx := context.Background()

storage, err := mcpstorage.NewEngine(ctx, mcpstorage.Config{
    Endpoint: "https://mcp.laisky.com",
    APIKey:   os.Getenv("MEMORY_MCP_API_KEY"),
})
if err != nil {
    panic(err)
}
```

`agents/memory/storage.Engine` methods:

1. `Read`
2. `Write`
3. `Stat`
4. `List`
5. `Search`
6. `Delete`

### 5.1 Storage parameter conventions

These conventions are shared by local and MCP storage implementations:

| Parameter                             | Meaning                                 | Required        | Constraints                                                                                                                    |
| ------------------------------------- | --------------------------------------- | --------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `project`                             | Tenant/project namespace for isolation. | Yes             | Must match `^[A-Za-z0-9_.-]{1,128}$`.                                                                                          |
| `path` / `storagePath` / `pathPrefix` | Absolute storage path.                  | Usually yes     | Must start with `/`, must be canonical (no `//`, `/./`, `/../`), no trailing `/`, no whitespace/control chars, max length 512. |
| `offset`                              | Byte offset for read/overwrite.         | Method-specific | Must be `>= 0`.                                                                                                                |
| `length`                              | Read length in bytes.                   | Optional        | `-1` means read to EOF.                                                                                                        |
| `depth`                               | Traversal depth for `List`.             | Optional        | Backend-specific defaulting rules apply (see note below).                                                                      |
| `limit`                               | Result cap for `List`/`Search`.         | Optional        | Non-positive values use backend defaults.                                                                                      |

Depth/limit backend nuance:

1. Local backend: `depth <= 0` and `limit <= 0` use defaults.
2. MCP backend: `limit <= 0` uses default; `depth < 0` uses default.

Root path usage:

1. `List` and `Stat` allow root (`""` or `"/"` depending on backend rules).
2. `Read`, `Write`, and `Delete` require a concrete non-root file/directory path.

### 5.2 `storage.Engine` method parameters

#### `Read(ctx, project, path, offset, length) (string, error)`

| Parameter | Required | Description                                            |
| --------- | -------- | ------------------------------------------------------ |
| `ctx`     | Yes      | Request context for cancellation/deadline propagation. |
| `project` | Yes      | Project namespace.                                     |
| `path`    | Yes      | Absolute file path to read.                            |
| `offset`  | No       | Start byte offset.                                     |
| `length`  | No       | Max bytes to read; `-1` for full remaining content.    |

#### `Write(ctx, project, path, content, mode, offset) error`

| Parameter | Required        | Description                                  |
| --------- | --------------- | -------------------------------------------- |
| `ctx`     | Yes             | Request context.                             |
| `project` | Yes             | Project namespace.                           |
| `path`    | Yes             | Absolute file path to write.                 |
| `content` | Yes             | UTF-8 content body.                          |
| `mode`    | Yes             | `APPEND`, `OVERWRITE`, or `TRUNCATE`.        |
| `offset`  | Method-specific | Used by `OVERWRITE`; ignored by other modes. |

#### `Stat(ctx, project, path) (FileInfo, error)`

| Parameter | Required | Description                            |
| --------- | -------- | -------------------------------------- |
| `ctx`     | Yes      | Request context.                       |
| `project` | Yes      | Project namespace.                     |
| `path`    | Yes      | Absolute target path; root is allowed. |

#### `List(ctx, project, path, depth, limit) ([]FileInfo, bool, error)`

| Parameter | Required | Description                                    |
| --------- | -------- | ---------------------------------------------- |
| `ctx`     | Yes      | Request context.                               |
| `project` | Yes      | Project namespace.                             |
| `path`    | Yes      | Absolute traversal root path; root is allowed. |
| `depth`   | No       | Max descendant depth.                          |
| `limit`   | No       | Max entries returned.                          |

`bool` return value is `hasMore`.

#### `Search(ctx, project, query, pathPrefix, limit) ([]FileChunk, error)`

| Parameter    | Required | Description                                 |
| ------------ | -------- | ------------------------------------------- |
| `ctx`        | Yes      | Request context.                            |
| `project`    | Yes      | Project namespace.                          |
| `query`      | Yes      | Search query string.                        |
| `pathPrefix` | Yes      | Absolute search scope prefix; root allowed. |
| `limit`      | No       | Max chunks returned.                        |

#### `Delete(ctx, project, path, recursive) error`

| Parameter   | Required | Description                        |
| ----------- | -------- | ---------------------------------- |
| `ctx`       | Yes      | Request context.                   |
| `project`   | Yes      | Project namespace.                 |
| `path`      | Yes      | Absolute target path.              |
| `recursive` | No       | If true, remove directory subtree. |

### 5.3 Local plugin config: `local.Config`

| Field      | Required | Default | Description                                       |
| ---------- | -------- | ------- | ------------------------------------------------- |
| `RootDir`  | Yes      | None    | Local filesystem root used by the storage engine. |
| `FilePerm` | No       | `0644`  | File permission for newly created files.          |
| `DirPerm`  | No       | `0755`  | Directory permission for created directories.     |

### 5.4 MCP plugin config: `mcpstorage.Config`

| Field          | Required    | Default                  | Description                                                                 |
| -------------- | ----------- | ------------------------ | --------------------------------------------------------------------------- |
| `Caller`       | Conditional | `nil`                    | Prebuilt MCP tool caller. If set, `Endpoint` and `APIKey` are not required. |
| `Endpoint`     | Conditional | None                     | MCP endpoint URL. Required when `Caller` is `nil`.                          |
| `APIKey`       | Conditional | None                     | MCP API key. Required when `Caller` is `nil`.                               |
| `RetryDelays`  | No          | `[200ms, 500ms, 1s, 2s]` | Retry backoff sequence for retryable MCP errors (caller mode).              |
| `DefaultDepth` | No          | `1`                      | Default `List` depth for MCP file operations (caller mode).                 |
| `DefaultLimit` | No          | `50`                     | Default `List` limit for MCP file operations (caller mode).                 |

### 5.5 Storage return model fields

#### `FileInfo`

| Field       | Description                                                        |
| ----------- | ------------------------------------------------------------------ |
| `Path`      | Absolute path of the entry.                                        |
| `Exists`    | Whether the target exists.                                         |
| `Type`      | `FILE`, `DIRECTORY`, or `UNKNOWN`.                                 |
| `SizeBytes` | Size in bytes (`0` for unknown/non-file cases as backend-defined). |
| `UpdatedAt` | Last update time (RFC3339 when backend provides it).               |

#### `FileChunk`

| Field        | Description                                   |
| ------------ | --------------------------------------------- |
| `FilePath`   | Absolute path containing the hit.             |
| `StartBytes` | Inclusive start byte offset for hit range.    |
| `EndBytes`   | Exclusive end byte offset for hit range.      |
| `Content`    | Returned chunk/document content from backend. |
| `Score`      | Backend relevance score.                      |

## 6) Memory Quick Start

Create engine:

```go
engine, err := memory.NewEngine(storage, memory.Config{
    RecentContextItems:     30,
    RecallFactsLimit:       20,
    InsightRecallLimit:     5,
    SearchLimit:            5,
    CompactThreshold:       0.8,
    L1RetentionDays:        1,
    L2RetentionDays:        7,
    ConsolidationMinEvents: 6,
    CompactionMinAge:       24 * time.Hour,
    SummaryRefreshInterval: time.Hour,
    MaxProcessedTurns:      1024,
})
if err != nil {
    panic(err)
}
```

Constructor parameters:

#### `memory.NewEngine(storage, conf) (*memory.StandardEngine, error)`

| Parameter | Required | Description                                                                               |
| --------- | -------- | ----------------------------------------------------------------------------------------- |
| `storage` | Yes      | Storage implementation conforming to `storage.Engine`; must be non-nil.                   |
| `conf`    | No       | Runtime behavior config; zero/invalid values are normalized to defaults where applicable. |

Standard turn lifecycle:

```go
conversation := []memory.ResponseItem{
    {
        Type: "message",
        Role: "assistant",
        Content: []memory.ResponseContentPart{
            {Type: "output_text", Text: "Previous answer already known to caller."},
        },
    },
    {
        Type: "message",
        Role: "user",
        Content: []memory.ResponseContentPart{
            {Type: "input_text", Text: "I prefer concise answers"},
        },
    },
}

prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
    Project:           "your-tenant",
    SessionID:         "session-001",
    UserID:            "user-001",
    TurnID:            "turn-001",
    ConversationItems: conversation,
    CurrentInputStart: 1,
    CurrentInputCount: 1,
    MaxInputTok:       120000,
})
if err != nil {
    panic(err)
}

modelOutput := []memory.ResponseItem{
    {
        Type: "message",
        Role: "assistant",
        Content: []memory.ResponseContentPart{
            {Type: "output_text", Text: "Understood."},
        },
    },
}

err = engine.AfterTurn(ctx, memory.AfterTurnInput{
    Project:           "your-tenant",
    SessionID:         "session-001",
    UserID:            "user-001",
    TurnID:            "turn-001",
    ConversationItems: prepared.InputItems,
    CurrentInputStart: len(prepared.InputItems) - 1,
    CurrentInputCount: 1,
    OutputItems:       modelOutput,
})
if err != nil {
    panic(err)
}
```

Legacy compatibility path:

```go
prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
    Project:   "your-tenant",
    SessionID: "session-001",
    UserID:    "user-001",
    TurnID:    "turn-001",
    CurrentInput: []memory.ResponseItem{
        {
            Type: "message",
            Role: "user",
            Content: []memory.ResponseContentPart{
                {Type: "input_text", Text: "I prefer concise answers"},
            },
        },
    },
    MaxInputTok: 120000,
})
if err != nil {
    panic(err)
}

err = engine.AfterTurn(ctx, memory.AfterTurnInput{
    Project:     "your-tenant",
    SessionID:   "session-001",
    UserID:      "user-001",
    TurnID:      "turn-001",
    InputItems:  prepared.InputItems,
    OutputItems: modelOutput,
})
if err != nil {
    panic(err)
}
```

### 6.1 Engine config reference: `memory.Config`

Non-positive numeric values are normalized to defaults.

| Field                    | Required | Default               | Description                                                           |
| ------------------------ | -------- | --------------------- | --------------------------------------------------------------------- |
| `RecentContextItems`     | No       | `30`                  | Number of recent context items included in `BeforeTurn` output.       |
| `RecallFactsLimit`       | No       | `20`                  | Max recalled memory facts injected per turn.                          |
| `InsightRecallLimit`     | No       | `5`                   | Max recalled consolidated insights injected per turn.                 |
| `SearchLimit`            | No       | `5`                   | Max storage search chunks used to enrich memory block.                |
| `CompactThreshold`       | No       | `0.8`                 | Compaction trigger ratio against `MaxInputTok`; valid range `(0, 1)`. |
| `L1RetentionDays`        | No       | `1`                   | L1 fact retention in days.                                            |
| `L2RetentionDays`        | No       | `7`                   | L2 fact retention in days.                                            |
| `ConsolidationMinEvents` | No       | `6`                   | Minimum raw events before consolidation writes new insight records.   |
| `CompactionMinAge`       | No       | `24h`                 | Minimum shard age before archive compaction.                          |
| `SummaryRefreshInterval` | No       | `1h`                  | Summary refresh interval used in policy metadata.                     |
| `MaxProcessedTurns`      | No       | `1024`                | Max remembered turn IDs for idempotency dedup.                        |
| `LLMAPIBase`             | No       | Empty                 | OpenAI-compatible API base/endpoint for heuristic fact extraction.    |
| `LLMAPIKey`              | No       | Empty                 | API key for LLM heuristic extractor.                                  |
| `LLMModel`               | No       | `openai/gpt-oss-120b` | Model name for heuristic extraction.                                  |
| `LLMTimeout`             | No       | `12s`                 | Timeout for heuristic LLM requests.                                   |
| `LLMMaxOutputTokens`     | No       | `800`                 | Max output tokens for heuristic LLM response.                         |
| `HeuristicClient`        | No       | `nil`                 | Custom heuristic client. If set, it is used directly.                 |
| `TimeNow`                | No       | `time.Now`            | Time provider hook for deterministic testing/custom clocks.           |

Heuristic client activation rules:

1. If `HeuristicClient` is provided, engine uses it directly.
2. If `HeuristicClient` is nil and both `LLMAPIBase` and `LLMAPIKey` are non-empty, engine auto-builds an OpenAI-compatible client.
3. If neither condition is met, only rule-based fact extraction is used.

### 6.2 Turn lifecycle input/output parameters

#### `BeforeTurnInput`

| Field               | Required    | Description                                                                |
| ------------------- | ----------- | -------------------------------------------------------------------------- |
| `Project`           | Yes         | Tenant/project namespace.                                                  |
| `SessionID`         | Yes         | Session identifier; controls storage root `/memory/{session_id}`.          |
| `UserID`            | No          | Optional user identifier for caller-level bookkeeping.                     |
| `TurnID`            | Yes         | Unique turn identifier used in event/fact IDs and dedup logic.             |
| `ConversationItems` | No          | Full caller-provided conversation slice, oldest to newest.                 |
| `CurrentInputStart` | No          | Start index of current-turn items inside `ConversationItems`.              |
| `CurrentInputCount` | No          | Number of current-turn items inside `ConversationItems`.                   |
| `CurrentInput`      | Conditional | Legacy latest-only input path. Required when `ConversationItems` is empty. |
| `BaseInstructions`  | No          | Reserved field (currently not consumed by engine internals).               |
| `MaxInputTok`       | No          | Token budget hint for compaction trigger. `<=0` disables threshold check.  |

#### `BeforeTurnOutput`

| Field               | Description                                                                                       |
| ------------------- | ------------------------------------------------------------------------------------------------- |
| `InputItems`        | Prepared model input (memory block + caller history + gap-filled recent context + current input). |
| `RecallFactIDs`     | Fact IDs included in the memory block for traceability.                                           |
| `RecallInsightIDs`  | Insight IDs included in the memory block for traceability.                                        |
| `ContextTokenCount` | Estimated token count for prepared input.                                                         |

#### `AfterTurnInput`

| Field               | Required | Description                                                                                                                                                                                      |
| ------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Project`           | Yes      | Tenant/project namespace.                                                                                                                                                                        |
| `SessionID`         | Yes      | Session identifier.                                                                                                                                                                              |
| `UserID`            | No       | Optional user identifier.                                                                                                                                                                        |
| `TurnID`            | Yes      | Turn identifier used for idempotent write dedup (`processed_turn_ids`).                                                                                                                          |
| `ConversationItems` | No       | Full caller-visible conversation slice used by explicit V2 reconciliation.                                                                                                                       |
| `CurrentInputStart` | No       | Start index of current-turn items inside `ConversationItems`.                                                                                                                                    |
| `CurrentInputCount` | No       | Count of current-turn items inside `ConversationItems`.                                                                                                                                          |
| `InputItems`        | No       | Legacy input path. When callers pass `BeforeTurnOutput.InputItems`, engine strips generated `<memory_reference>` blocks and trims a replayed recent-context prefix before persisting turn delta. |
| `OutputItems`       | No       | Model output items to persist as turn events.                                                                                                                                                    |

Idempotency note: if `TurnID` already exists in `processed_turn_ids`, `AfterTurn` returns success without duplicating writes.

V2 recommendation:

1. Prefer `ConversationItems + CurrentInputStart + CurrentInputCount` for both `BeforeTurn` and `AfterTurn`.
2. Keep caller items ordered oldest to newest.
3. Use the legacy `CurrentInput` and `InputItems` fields only for backward compatibility.

### 6.3 Message schema parameters

#### `ResponseItem`

| Field      | Required | Description                                                           |
| ---------- | -------- | --------------------------------------------------------------------- |
| `Type`     | Yes      | Item type such as `message`, `function_call`, or tool-specific types. |
| `Role`     | No       | Role, typically `system`, `user`, or `assistant` for message items.   |
| `Content`  | No       | Structured content parts for message payload.                         |
| `CallID`   | No       | Tool/function call identifier when relevant.                          |
| `Output`   | No       | Raw textual output for non-message items when needed.                 |
| `Metadata` | No       | Arbitrary string key-value metadata.                                  |

#### `ResponseContentPart`

| Field      | Required | Description                                                            |
| ---------- | -------- | ---------------------------------------------------------------------- |
| `Type`     | Yes      | Part type such as `input_text`, `output_text`, `image_url`, or `file`. |
| `Text`     | No       | Text body for text part types.                                         |
| `ImageURL` | No       | Image URL for image parts.                                             |
| `FileID`   | No       | Referenced file ID.                                                    |
| `Filename` | No       | Human-readable file name.                                              |

### 6.4 Custom heuristic integration parameters

If you provide `Config.HeuristicClient`, your implementation must satisfy:

#### `ExtractAndMergeFacts(ctx, in) (HeuristicFactResult, error)`

| Parameter | Required | Description                         |
| --------- | -------- | ----------------------------------- |
| `ctx`     | Yes      | Request context.                    |
| `in`      | Yes      | Heuristic extraction input payload. |

#### `HeuristicFactInput`

| Field           | Required | Description                                          |
| --------------- | -------- | ---------------------------------------------------- |
| `TurnID`        | Yes      | Current turn ID.                                     |
| `NowRFC3339`    | Yes      | Current timestamp in RFC3339 UTC string.             |
| `UserID`        | No       | Optional user identifier for provenance-aware logic. |
| `InputItems`    | Yes      | Current turn input items to analyze.                 |
| `ExistingFacts` | Yes      | Exact currently active facts for merge decisions.    |

#### `HeuristicFactResult`

| Field            | Description                                                  |
| ---------------- | ------------------------------------------------------------ |
| `UpdatedFacts`   | New or changed facts proposed for upsert.                    |
| `DeletedFactIDs` | Fact IDs that should be deleted from the exact active index. |

## 7) Maintenance and Directory Summary

Run maintenance:

```go
var mgmt memory.Management = engine
if err := mgmt.RunMaintenance(ctx, "your-tenant", "session-001"); err != nil {
    panic(err)
}

if err := mgmt.RunConsolidation(ctx, "your-tenant", "session-001"); err != nil {
    panic(err)
}
```

List folders with `.abstract` content:

```go
summaries, err := mgmt.ListDirWithAbstract(
    ctx,
    "your-tenant",
    "session-001",
    "",   // root path fallback to /memory/{session_id}
    8,
    200,
)
if err != nil {
    panic(err)
}

for _, s := range summaries {
    fmt.Println(s.Path, s.HasOverview)
    fmt.Println(s.Abstract)
}
```

### 7.1 Management API parameter reference

#### `RunMaintenance(ctx, project, sessionID) error`

| Parameter   | Required | Description               |
| ----------- | -------- | ------------------------- |
| `ctx`       | Yes      | Request context.          |
| `project`   | Yes      | Tenant/project namespace. |
| `sessionID` | Yes      | Target session.           |

Behavior summary:

1. Compacts runtime context when needed.
2. Archives old raw shards.
3. Sweeps expired L1/L2 facts.
4. Runs bounded offline consolidation into insight records.
5. Refreshes `.abstract` and `.overview` files.
6. Updates metadata, watermarks, and metrics.

#### `RunConsolidation(ctx, project, sessionID) error`

| Parameter   | Required | Description               |
| ----------- | -------- | ------------------------- |
| `ctx`       | Yes      | Request context.          |
| `project`   | Yes      | Tenant/project namespace. |
| `sessionID` | Yes      | Target session.           |

Behavior summary:

1. Reads raw events for the session.
2. Derives bounded observations from event and fact history.
3. Writes new insight records under `/insights/...`.
4. Updates consolidation watermarks and metrics.

#### `ListDirWithAbstract(ctx, project, sessionID, path, depth, limit) ([]DirectorySummary, error)`

| Parameter   | Required | Default                 | Description                                                 |
| ----------- | -------- | ----------------------- | ----------------------------------------------------------- |
| `ctx`       | Yes      | None                    | Request context.                                            |
| `project`   | Yes      | None                    | Tenant/project namespace.                                   |
| `sessionID` | Yes      | None                    | Target session.                                             |
| `path`      | No       | `"/memory/{sessionID}"` | Root listing path. Empty string falls back to session root. |
| `depth`     | No       | `8`                     | Directory traversal depth.                                  |
| `limit`     | No       | `200`                   | Max entries returned by underlying storage list call.       |

#### `DirectorySummary`

| Field         | Description                                                          |
| ------------- | -------------------------------------------------------------------- |
| `Path`        | Directory path.                                                      |
| `Abstract`    | Directory abstract content (`.abstract`), auto-generated if missing. |
| `UpdatedAt`   | Last update timestamp of `.abstract` when available.                 |
| `HasOverview` | Whether `.overview` exists for the directory.                        |

## 8) Data Layout (Canonical)

Per session canonical layout:

```text
/memory/{session_id}/
  /meta/
    state.json
    policy.json
    watermarks.json
        metrics.json
  /events/
    /raw/YYYY/MM/DD/log-YYYYMMDD.jsonl
    /compact/YYYY/MM/DD/compact-YYYYMMDD.jsonl
    /archive/YYYY/MM/DD/log-*.jsonl.zst
    /indexes/
        active_facts.json
    /insights/
        /YYYY/MM/DD/insights-YYYYMMDD.jsonl
  /memory_tiers/
    /L0/YYYY/MM/facts-YYYYMM.jsonl
    /L1/YYYY/MM/facts-YYYYMMDD.jsonl
        /L2/YYYY/WW/facts-YYYY-WWW.jsonl  # actual pattern: facts-{ISOYear}-W{ISOWeek(2-digit)}.jsonl
  /runtime/context/
    current.jsonl
    latest_compact_pointer.json
```

Managed folders also have:

1. `.abstract` (target 100-200 words)
2. `.overview` (up to 2000 words)

## 9) Compatibility Layout

During migration, the engine still reads/writes legacy files:

1. `/memory/{session}/log.jsonl`
2. `/memory/{session}/context.jsonl`
3. `/memory/{session}/memory_facts.jsonl`
4. `/memory/{session}/meta.json`

Compatibility behavior:

1. `BeforeTurn` reads canonical context first, then legacy context fallback
2. Fact recall prefers `indexes/active_facts.json`, then falls back to tier files and finally legacy facts
3. Meta reads canonical state first; if missing, legacy meta fallback
4. `AfterTurn` dual-writes canonical and legacy files

## 10) Runtime Behavior Details

### 10.1 `BeforeTurn`

1. Normalize caller input into history items and current-turn items
2. Load runtime context and recall facts
    - Fact recall is ranked by query relevance + confidence + recency + tier priority.
    - Exact active facts are loaded from `indexes/active_facts.json` when present.
3. Load bounded insight recall from `/insights/...`
4. Search related chunks via storage `Search`
    - Search retrieval is best-effort; search errors are ignored and request assembly continues.
5. Remove duplicate engine-recalled recent items already present in caller history/current items
6. Build memory block + caller history + recent context + current input
7. Estimate token load
8. Compact runtime context if threshold exceeded

### 10.1.1 Local storage backend operational limits

From `agents/memory/storage/local` implementation:

1. `List` depth defaults to `8` and is capped at `32`.
2. `List` limit defaults to `1000` and is capped at `5000`.
3. `Search` limit defaults to `5` and is capped at `50`.
4. `Search` skips files larger than `4 MiB`.
5. Symlinks are skipped in traversal/search to avoid root escape and loop recursion.

### 10.2 `AfterTurn`

1. Ensure scaffold files (`policy`, summaries, watermarks)
2. Enforce idempotency by `processed_turn_ids`
3. Normalize current-turn inputs from the explicit conversation boundary when provided
4. Persist only turn-delta inputs (skip injected memory reference and trim replayed recent-context prefix in legacy mode)
5. Append turn events into raw shard and runtime context
6. Load exact active facts from `indexes/active_facts.json`
7. Extract facts and classify into `L0`, `L1`, `L2`
8. Apply exact mutations:
    - Identity key: `fact_id + key`
    - Skip unchanged active values
    - Write supersede records when value/tier changes
    - Write delete records for heuristic `DeletedFactIDs`
    - Rewrite `indexes/active_facts.json` to reflect the new active state
9. Update metadata, watermarks, and metrics

### 10.3 Fact extraction rules

Current built-in rules:

1. `my name is ...` -> `L0`
2. `i prefer ...` -> `L0`
3. `i like ...` -> `L2`
4. `today i need ...` -> `L1`
5. `this week i need ...` -> `L2`

## 11) Retention and Time Rules

1. `L0`: no auto-expiry
2. `L1`: expires by `L1RetentionDays` (default `1`)
3. `L2`: expires by `L2RetentionDays` (default `7`)
4. All timestamps and expiry boundaries use UTC

## 12) Testing and Quality

Unit/behavior tests (default):

```bash
go test ./agents/memory -count=1
go test -race ./agents/memory -count=1
go test ./agents/memory -covermode=atomic -coverprofile=/tmp/memory.cover.out
go test ./agents/memory -run 'TestMemoryQuantitativeEvaluationBaseline|TestQuantitativeGatesAcceptCurrentV2' -count=1
go test ./agents/memory -run '^$' -bench 'BenchmarkMemoryEngine(BeforeTurn|AfterTurn)$' -benchmem -count=1
```

Coverage report:

```bash
go tool cover -func=/tmp/memory.cover.out
```

E2E test with MCP (build-tagged):

```bash
go test -tags e2e ./agents/memory -run TestMemorySDKEndToEndWithMCP -count=1
```

Required env for E2E:

1. `MEMORY_MCP_ENDPOINT` or `MCP_ENDPOINT`
2. `MEMORY_MCP_API_KEY` or `MCP_API_KEY`
3. `MEMORY_PROJECT`

## 13) Troubleshooting

1. `INVALID_PATH`: verify absolute paths and valid `project`
2. Frequent `RESOURCE_BUSY`: reduce concurrent writers per session
3. Empty recall: verify extracted facts exist in tier shards
4. Unexpected duplicate prompts: prefer explicit `ConversationItems` boundaries instead of the legacy fallback path
5. Missing folder abstracts: run `RunMaintenance` or call `ListDirWithAbstract`
6. No E2E execution: confirm `-tags e2e` and required env variables

## 14) Minimal Adoption Checklist

1. Create `files.MCPClient` and `files.MCPStorage`
2. Create `memory.NewEngine(storageEngine, config)`
3. Pass explicit `ConversationItems` with current-turn boundaries to `BeforeTurn`
4. Pass explicit `ConversationItems` with the same current-turn boundary to `AfterTurn`
5. Periodically run `RunMaintenance`
6. Optionally run `RunConsolidation` on its own scheduler when you want separate consolidation cadence
7. Use `ListDirWithAbstract` for memory directory introspection

## 15) V1 to V2 Migration

V2 keeps the old fields for compatibility, but new integrations should move to the explicit conversation contract.

### 15.1 What changed

V1 style:

1. `BeforeTurn` usually received only `CurrentInput`.
2. `AfterTurn` often received `BeforeTurnOutput.InputItems` via `InputItems`.
3. The engine inferred replayed history by stripping the injected memory block and trimming a recent-context prefix.

V2 style:

1. Callers pass the full caller-visible conversation in `ConversationItems`.
2. Callers mark the current-turn slice with `CurrentInputStart` and `CurrentInputCount`.
3. `BeforeTurn` deduplicates caller history against engine recall exactly.
4. `AfterTurn` persists only the current-turn delta directly instead of relying on prefix inference.

### 15.2 Recommended migration steps

1. Keep your existing `TurnID`, `Project`, `SessionID`, and `UserID` flow unchanged.
2. Build one oldest-to-newest `ConversationItems` slice for every model request.
3. Set `CurrentInputStart` to the index of the first current-turn item.
4. Set `CurrentInputCount` to the number of current-turn input items.
5. Pass the same explicit boundary model to `AfterTurn` after the model returns.
6. Stop depending on `CurrentInput` and `InputItems` once your caller has fully migrated.

### 15.3 BeforeTurn migration example

V1:

```go
prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
    Project:   project,
    SessionID: sessionID,
    TurnID:    turnID,
    CurrentInput: []memory.ResponseItem{
        userItem,
    },
    MaxInputTok: 120000,
})
```

V2:

```go
conversation := []memory.ResponseItem{
    historyAssistantItem,
    userItem,
}

prepared, err := engine.BeforeTurn(ctx, memory.BeforeTurnInput{
    Project:           project,
    SessionID:         sessionID,
    TurnID:            turnID,
    ConversationItems: conversation,
    CurrentInputStart: 1,
    CurrentInputCount: 1,
    MaxInputTok:       120000,
})
```

### 15.4 AfterTurn migration example

V1:

```go
err = engine.AfterTurn(ctx, memory.AfterTurnInput{
    Project:     project,
    SessionID:   sessionID,
    TurnID:      turnID,
    InputItems:  prepared.InputItems,
    OutputItems: modelOutput,
})
```

V2:

```go
err = engine.AfterTurn(ctx, memory.AfterTurnInput{
    Project:           project,
    SessionID:         sessionID,
    TurnID:            turnID,
    ConversationItems: prepared.InputItems,
    CurrentInputStart: len(prepared.InputItems) - 1,
    CurrentInputCount: 1,
    OutputItems:       modelOutput,
})
```

### 15.5 Mixed-mode rollout

If you cannot migrate every caller immediately:

1. You can continue using `CurrentInput` and `InputItems` temporarily.
2. The engine will still support the legacy prefix-trimming path.
3. Exact duplicate suppression for caller-provided history is strongest when you adopt `ConversationItems` boundaries.
4. Migrate high-traffic or multi-turn callers first, because they benefit most from the duplicate-free V2 path.
