# Agents Storage & Memory SDK Manual

This manual explains how to use:

- `agents/files`: MCP FileIO storage SDK
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
    C --> D[MCP FileIO Adapter]

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
    "github.com/Laisky/go-utils/v6/agents/files"
    "github.com/Laisky/go-utils/v6/agents/memory"
)
```

## 5) MCP Storage Quick Start

Create MCP client and storage adapter:

```go
ctx := context.Background()

client, err := files.NewMCPClient(files.MCPClientConfig{
    Endpoint: "https://mcp.laisky.com",
    APIKey:   os.Getenv("MEMORY_MCP_API_KEY"),
})
if err != nil {
    panic(err)
}

storage, err := files.NewMCPStorage(files.MCPStorageConfig{Caller: client})
if err != nil {
    panic(err)
}

_ = storage
```

`files.Storage` methods:

1. `Read`
2. `Write`
3. `Stat`
4. `List`
5. `Search`
6. `Delete`

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
    /L2/YYYY/WW/facts-YYYY-Www.jsonl
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
2. Search related chunks via storage `Search`
3. Build memory block + recent context + current input
4. Estimate token load
5. Compact runtime context if threshold exceeded

### 10.2 `AfterTurn`

1. Ensure scaffold files (`policy`, summaries, watermarks)
2. Enforce idempotency by `processed_turn_ids`
3. Append turn events into raw shard and runtime context
4. Extract facts and classify into `L0`, `L1`, `L2`
5. Write facts to tiered shards
6. Update metadata

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
2. Create `memory.NewEngine(storage, config)`
3. Add `BeforeTurn` before model invocation
4. Add `AfterTurn` after model response
5. Periodically run `RunMaintenance`
6. Use `ListDirWithAbstract` for memory directory introspection
