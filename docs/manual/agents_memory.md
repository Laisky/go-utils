# Agents Storage & Memory SDK Manual

This manual explains how to use:

- `agents/memory/storage`: standardized storage interface plus plugins
- `agents/memory`: tiered memory SDK on top of storage

The content matches the current implementation in `agents/memory`.

## 1) Why This Exists

Long-running agents need stable memory across turns and sessions.
This SDK provides:

1. Durable interaction history
2. Tiered long-term facts with retention
3. Runtime context compaction
4. Operational maintenance APIs

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
    B --> E4[/meta/state.json]

    F[RunMaintenance] --> G1[Compaction]
    F --> G2[Archive old raw shards]
    F --> G3[Sweep expired L1/L2 facts]
    F --> G4[Refresh .abstract/.overview]
```

## 3) Package APIs

### 3.1 Core engine

`memory.Engine`:

1. `BeforeTurn(ctx, input)`
2. `AfterTurn(ctx, input)`

### 3.2 Management API

`*memory.StandardEngine` also implements `memory.Management`:

1. `RunMaintenance(ctx, project, sessionID)`
2. `ListDirWithAbstract(ctx, project, sessionID, path, depth, limit)`

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
    SearchLimit:            5,
    CompactThreshold:       0.8,
    L1RetentionDays:        1,
    L2RetentionDays:        7,
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
| `SearchLimit`            | No       | `5`                   | Max storage search chunks used to enrich memory block.                |
| `CompactThreshold`       | No       | `0.8`                 | Compaction trigger ratio against `MaxInputTok`; valid range `(0, 1)`. |
| `L1RetentionDays`        | No       | `1`                   | L1 fact retention in days.                                            |
| `L2RetentionDays`        | No       | `7`                   | L2 fact retention in days.                                            |
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

| Field              | Required | Description                                                               |
| ------------------ | -------- | ------------------------------------------------------------------------- |
| `Project`          | Yes      | Tenant/project namespace.                                                 |
| `SessionID`        | Yes      | Session identifier; controls storage root `/memory/{session_id}`.         |
| `UserID`           | No       | Optional user identifier for caller-level bookkeeping.                    |
| `TurnID`           | Yes      | Unique turn identifier used in event/fact IDs and dedup logic.            |
| `CurrentInput`     | Yes      | Current user/tool input items for this turn.                              |
| `BaseInstructions` | No       | Reserved field (currently not consumed by engine internals).              |
| `MaxInputTok`      | No       | Token budget hint for compaction trigger. `<=0` disables threshold check. |

#### `BeforeTurnOutput`

| Field               | Description                                                           |
| ------------------- | --------------------------------------------------------------------- |
| `InputItems`        | Prepared model input (memory block + recent context + current input). |
| `RecallFactIDs`     | Fact IDs included in the memory block for traceability.               |
| `ContextTokenCount` | Estimated token count for prepared input.                             |

#### `AfterTurnInput`

| Field         | Required | Description                                                             |
| ------------- | -------- | ----------------------------------------------------------------------- |
| `Project`     | Yes      | Tenant/project namespace.                                               |
| `SessionID`   | Yes      | Session identifier.                                                     |
| `UserID`      | No       | Optional user identifier.                                               |
| `TurnID`      | Yes      | Turn identifier used for idempotent write dedup (`processed_turn_ids`). |
| `InputItems`  | No       | Input items to persist as turn events. When callers pass `BeforeTurnOutput.InputItems`, engine automatically strips generated `<memory_reference>` blocks and leading recalled-history prefix, then persists only turn-delta input. |
| `OutputItems` | No       | Model output items to persist as turn events.                           |

Idempotency note: if `TurnID` already exists in `processed_turn_ids`, `AfterTurn` returns success without duplicating writes.

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

#### `ExtractAndMergeFacts(ctx, in) ([]MemoryFact, error)`

| Parameter | Required | Description                         |
| --------- | -------- | ----------------------------------- |
| `ctx`     | Yes      | Request context.                    |
| `in`      | Yes      | Heuristic extraction input payload. |

#### `HeuristicFactInput`

| Field           | Required | Description                                          |
| --------------- | -------- | ---------------------------------------------------- |
| `TurnID`        | Yes      | Current turn ID.                                     |
| `NowRFC3339`    | Yes      | Current timestamp in RFC3339 UTC string.             |
| `InputItems`    | Yes      | Current turn input items to analyze.                 |
| `ExistingFacts` | Yes      | Currently recalled facts for merge/upsert decisions. |

## 7) Maintenance and Directory Summary

Run maintenance:

```go
var mgmt memory.Management = engine
if err := mgmt.RunMaintenance(ctx, "your-tenant", "session-001"); err != nil {
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
4. Refreshes `.abstract` and `.overview` files.
5. Updates metadata timestamps.

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
  /events/
    /raw/YYYY/MM/DD/log-YYYYMMDD.jsonl
    /compact/YYYY/MM/DD/compact-YYYYMMDD.jsonl
    /archive/YYYY/MM/DD/log-*.jsonl.zst
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
2. Fact recall reads tier files first, then legacy facts fallback
3. Meta reads canonical state first; if missing, legacy meta fallback
4. `AfterTurn` dual-writes canonical and legacy files

## 10) Runtime Behavior Details

### 10.1 `BeforeTurn`

1. Load runtime context and recall facts
    - Fact recall is ranked by query relevance + confidence + recency + tier priority.
    - Dedup identity is `fact_id + key` before applying `RecallFactsLimit`.
2. Search related chunks via storage `Search`
    - Search retrieval is best-effort; search errors are ignored and request assembly continues.
3. Build memory block + recent context + current input
4. Estimate token load
5. Compact runtime context if threshold exceeded

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
3. Normalize and persist only turn-delta inputs (skip injected memory reference and recalled-history prefix)
4. Append turn events into raw shard and runtime context
5. Extract facts and classify into `L0`, `L1`, `L2`
6. Perform delta upsert filter:
    - Identity key: `fact_id + key`
    - Skip writes when latest active fact has same normalized value and same tier
    - Write when value/tier changes or previous fact has expired
7. Write remaining fact deltas to tiered shards
8. Update metadata

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
4. Missing folder abstracts: run `RunMaintenance` or call `ListDirWithAbstract`
5. No E2E execution: confirm `-tags e2e` and required env variables

## 14) Minimal Adoption Checklist

1. Create `files.MCPClient` and `files.MCPStorage`
2. Create `memory.NewEngine(storageEngine, config)`
3. Add `BeforeTurn` before model invocation
4. Add `AfterTurn` after model response
5. Periodically run `RunMaintenance`
6. Use `ListDirWithAbstract` for memory directory introspection
