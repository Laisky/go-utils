# MCP FileIO User Manual

This manual is for end users and integrators who want to call `file_stat`, `file_read`, `file_write`, `file_delete`, `file_list`, and `file_search` through MCP.

## Menu

- [MCP FileIO User Manual](#mcp-fileio-user-manual)
  - [Menu](#menu)
  - [1. What FileIO is for](#1-what-fileio-is-for)
  - [2. Quick start (5 minutes)](#2-quick-start-5-minutes)
    - [2.1 Prepare environment variables](#21-prepare-environment-variables)
    - [2.2 Initialize MCP session (required)](#22-initialize-mcp-session-required)
    - [2.3 (Optional) list available tools](#23-optional-list-available-tools)
  - [3. Unified call format](#3-unified-call-format)
  - [4. Input rules (read this first)](#4-input-rules-read-this-first)
    - [4.1 `project`](#41-project)
    - [4.2 `path`](#42-path)
    - [4.3 Content and encoding](#43-content-and-encoding)
  - [5. Tool-by-tool guide with cURL examples](#5-tool-by-tool-guide-with-curl-examples)
    - [5.1 `file_write`](#51-file_write)
      - [Example A: create and write](#example-a-create-and-write)
      - [Example B: overwrite from byte offset](#example-b-overwrite-from-byte-offset)
    - [5.2 `file_read`](#52-file_read)
    - [5.3 `file_stat`](#53-file_stat)
    - [5.4 `file_list`](#54-file_list)
    - [5.5 `file_search`](#55-file_search)
    - [5.6 `file_delete`](#56-file_delete)
      - [Example A: delete one file](#example-a-delete-one-file)
      - [Example B: recursively delete a directory](#example-b-recursively-delete-a-directory)
  - [6. Common error codes](#6-common-error-codes)
  - [7. Minimal end-to-end flow](#7-minimal-end-to-end-flow)
  - [8. Integration best practices](#8-integration-best-practices)

## 1. What FileIO is for

MCP FileIO provides a remote, project-scoped workspace for text files. It is useful for AI agents and automation workflows that need durable shared files.

You can:

- Read and write files under a specific `project` (text content, `utf-8`).
- Browse directories (directories are logical/implicit).
- Delete a file or a directory subtree.
- Search indexed file content with `file_search`.

## 2. Quick start (5 minutes)

### 2.1 Prepare environment variables

```bash
export MCP_ENDPOINT="https://mcp.laisky.com"
export MCP_API_KEY="<YOUR_API_KEY>"
export MCP_AUTH="Authorization: Bearer ${MCP_API_KEY}"
```

### 2.2 Initialize MCP session (required)

MCP uses a session. Call `initialize` first, then read `Mcp-Session-Id` from response headers and include it in all following requests.

```bash
curl -i -sS "$MCP_ENDPOINT" \
	-H 'Content-Type: application/json' \
	-H "$MCP_AUTH" \
	--data-raw '{
		"jsonrpc":"2.0",
		"id":1,
		"method":"initialize",
		"params":{
			"protocolVersion":"2025-06-18",
			"capabilities":{},
			"clientInfo":{"name":"curl-client","version":"1.0.0"}
		}
	}'
```

Save the response header value:

```bash
export MCP_SESSION_ID="<SESSION_ID_FROM_RESPONSE_HEADER>"
```

### 2.3 (Optional) list available tools

```bash
curl -sS "$MCP_ENDPOINT" \
	-H 'Content-Type: application/json' \
	-H "$MCP_AUTH" \
	-H "Mcp-Session-Id: ${MCP_SESSION_ID}" \
	--data-raw '{
		"jsonrpc":"2.0",
		"id":2,
		"method":"tools/list",
		"params":{}
	}'
```

## 3. Unified call format

All FileIO calls use `tools/call`:

```json
{
  "jsonrpc": "2.0",
  "id": 100,
  "method": "tools/call",
  "params": {
    "name": "file_write",
    "arguments": {
      "project": "demo",
      "path": "/notes/todo.txt",
      "content": "hello"
    }
  }
}
```

Tool output is typically in `result.content[0].text` (as JSON string). Error payloads include:

```json
{ "code": "INVALID_PATH", "message": "...", "retryable": false }
```

## 4. Input rules (read this first)

### 4.1 `project`

- Required, length `1..128`
- Allowed characters: `a-z A-Z 0-9 _ - .`

### 4.2 `path`

- Root path is `""`
- Non-root paths must start with `/`, for example `/docs/a.txt`
- Must not end with `/`
- Must not contain `//`, `.` or `..` segments
- Must not contain whitespace or control characters
- Max length is `512`

### 4.3 Content and encoding

- Text only
- `content_encoding` must be `utf-8` (default is `utf-8`)

## 5. Tool-by-tool guide with cURL examples

All examples below are directly runnable.

To reduce repetition, define a helper first:

```bash
mcp_call() {
	local payload="$1"
	curl -sS "$MCP_ENDPOINT" \
		-H 'Content-Type: application/json' \
		-H "$MCP_AUTH" \
		-H "Mcp-Session-Id: ${MCP_SESSION_ID}" \
		--data-raw "$payload"
}
```

### 5.1 `file_write`

Write or update a file.

- Required: `project`, `path`, `content`
- Optional: `content_encoding`, `offset`, `mode`
- `mode`:
  - `APPEND` (default): always writes at EOF
  - `OVERWRITE`: writes from `offset` without truncating tail bytes
  - `TRUNCATE`: clears file then writes (`offset` must be `0`)

#### Example A: create and write

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":101,
	"method":"tools/call",
	"params":{
		"name":"file_write",
		"arguments":{
			"project":"demo",
			"path":"/docs/readme.txt",
			"content":"Hello FileIO\n",
			"mode":"APPEND"
		}
	}
}'
```

#### Example B: overwrite from byte offset

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":102,
	"method":"tools/call",
	"params":{
		"name":"file_write",
		"arguments":{
			"project":"demo",
			"path":"/docs/readme.txt",
			"content":"MCP",
			"mode":"OVERWRITE",
			"offset":6
		}
	}
}'
```

Success response example:

```json
{ "bytes_written": 4 }
```

### 5.2 `file_read`

Read file content.

- Required: `project`, `path`
- Optional: `offset` (default `0`), `length` (default `-1`, read to EOF)

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":103,
	"method":"tools/call",
	"params":{
		"name":"file_read",
		"arguments":{
			"project":"demo",
			"path":"/docs/readme.txt",
			"offset":0,
			"length":100
		}
	}
}'
```

Success response example:

```json
{ "content": "Hello MCPIO...", "content_encoding": "utf-8" }
```

### 5.3 `file_stat`

Check whether a path is a file, directory, or missing.

- Required: `project`
- Optional: `path` (empty or omitted means root)

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":104,
	"method":"tools/call",
	"params":{
		"name":"file_stat",
		"arguments":{
			"project":"demo",
			"path":"/docs"
		}
	}
}'
```

Response example:

```json
{
  "exists": true,
  "type": "DIRECTORY",
  "size": 0,
  "created_at": "0001-01-01T00:00:00Z",
  "updated_at": "2026-02-13T08:00:00Z"
}
```

### 5.4 `file_list`

List files/directories under a path.

- Required: `project`
- Optional:
  - `path`: directory path; root is `""` (tool also accepts `"/"`)
  - `depth`: default `1`
    - `0`: only the `path` entry itself
    - `1`: immediate children
    - `>1`: recursive traversal
  - `limit`: max entries to return

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":105,
	"method":"tools/call",
	"params":{
		"name":"file_list",
		"arguments":{
			"project":"demo",
			"path":"/docs",
			"depth":2,
			"limit":50
		}
	}
}'
```

Response example:

```json
{
  "entries": [
    { "name": "readme.txt", "path": "/docs/readme.txt", "type": "FILE", "size": 18, "created_at": "...", "updated_at": "..." },
    { "name": "sub", "path": "/docs/sub", "type": "DIRECTORY", "size": 0, "created_at": "0001-01-01T00:00:00Z", "updated_at": "..." }
  ],
  "has_more": false
}
```

### 5.5 `file_search`

Search indexed chunks inside a project.

- Required: `project`, `query`
- Optional:
  - `path_prefix`: prefix filter, for example `/docs`
  - `limit`: default `5`, max `20`

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":106,
	"method":"tools/call",
	"params":{
		"name":"file_search",
		"arguments":{
			"project":"demo",
			"query":"readme",
			"path_prefix":"/docs",
			"limit":5
		}
	}
}'
```

Response example:

```json
{
  "chunks": [
    {
      "file_path": "/docs/readme.txt",
      "file_seek_start_bytes": 0,
      "file_seek_end_bytes": 120,
      "chunk_content": "...",
      "score": 0.93
    }
  ]
}
```

> Note: `file_search` can be eventually consistent. Recent writes may take a short time to appear.

### 5.6 `file_delete`

Delete a file or directory subtree.

- Required: `project`
- Optional:
  - `path`: target path
  - `recursive`: required for non-empty directory deletion

#### Example A: delete one file

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":107,
	"method":"tools/call",
	"params":{
		"name":"file_delete",
		"arguments":{
			"project":"demo",
			"path":"/docs/readme.txt",
			"recursive":false
		}
	}
}'
```

#### Example B: recursively delete a directory

```bash
mcp_call '{
	"jsonrpc":"2.0",
	"id":108,
	"method":"tools/call",
	"params":{
		"name":"file_delete",
		"arguments":{
			"project":"demo",
			"path":"/docs",
			"recursive":true
		}
	}
}'
```

Success response example:

```json
{ "deleted_count": 3 }
```

> Root deletion is not allowed (`path=""` returns `PERMISSION_DENIED`).

## 6. Common error codes

- `INVALID_PATH`: invalid `project` or `path`
- `INVALID_OFFSET`: invalid offset/length/depth/mode position
- `NOT_FOUND`: target path does not exist
- `IS_DIRECTORY`: file operation used on directory path
- `NOT_DIRECTORY`: a parent segment is an existing file
- `NOT_EMPTY`: directory is not empty and `recursive=false`
- `PERMISSION_DENIED`: missing permission or forbidden operation (for example root delete)
- `PAYLOAD_TOO_LARGE`: write payload exceeds limit
- `QUOTA_EXCEEDED`: project storage quota exceeded
- `RESOURCE_BUSY`: lock timeout under concurrent mutations; retry later
- `SEARCH_BACKEND_ERROR`: search backend unavailable or disabled

## 7. Minimal end-to-end flow

```bash
# 1) Write
mcp_call '{"jsonrpc":"2.0","id":201,"method":"tools/call","params":{"name":"file_write","arguments":{"project":"demo","path":"/quick/start.txt","content":"hello"}}}'

# 2) Read
mcp_call '{"jsonrpc":"2.0","id":202,"method":"tools/call","params":{"name":"file_read","arguments":{"project":"demo","path":"/quick/start.txt"}}}'

# 3) List
mcp_call '{"jsonrpc":"2.0","id":203,"method":"tools/call","params":{"name":"file_list","arguments":{"project":"demo","path":"/quick","depth":1,"limit":20}}}'

# 4) Search
mcp_call '{"jsonrpc":"2.0","id":204,"method":"tools/call","params":{"name":"file_search","arguments":{"project":"demo","query":"hello","limit":5}}}'

# 5) Delete
mcp_call '{"jsonrpc":"2.0","id":205,"method":"tools/call","params":{"name":"file_delete","arguments":{"project":"demo","path":"/quick/start.txt","recursive":false}}}'
```

## 8. Integration best practices

- Always initialize a session and cache `Mcp-Session-Id`.
- Parse error JSON by `code` and branch your retry/fallback logic.
- For write retries, use an idempotent pattern (for example `file_stat` + conditional write).
- For `file_search`, allow a short delay after writes for indexing visibility.

If you want, I can also add a compact ~30-line Bash wrapper that handles session bootstrap and unified error parsing automatically.
