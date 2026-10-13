# Session-scoped, bounded directory summaries (#114, #116)

`ListDirWithAbstract` is scoped to `/memory/{sessionID}`. Empty path selects that
root. Explicit roots and backend-returned paths must be canonical absolute paths
inside the requested subtree after outer whitespace trimming; traversal aliases,
sibling-prefix matches, backslashes, ASCII whitespace/control characters and
overlong paths are rejected. Backend paths are all validated before metadata
reads or writes. Project/session actor authorization is still the caller's duty.

Depth is capped at 32 and the requested entry/directory limit at 200; nonpositive
values retain defaults 8/200. The directory budget includes the root and derived
ancestors. A backend over-return or derived-directory overflow fails closed before
metadata effects. Each directory needs at most three metadata/content calls.
Abstracts are capped at 64 KiB each and 1 MiB in total, counting untrimmed bytes;
reads use a bounded range with a one-byte overflow probe. A ten-second deadline
covers the operation. Errors return no partial summaries, but already-created
in-scope abstracts are not rolled back transactionally.

For MCP, the entire operation shares 4 MiB of raw response-body capacity and at
most `2 * (1 + 3 * effectiveLimit)` HTTP attempts, including initialization/retries.
`files.WithMCPBudget` can also set a caller's stricter aggregate limits. Nested
budgets cannot erase parent limits; actual bytes and attempts on failures count.
In-flight responses reserve byte capacity, so concurrent requests may fail
admission rather than overspend. One overflow-probe byte can be consumed per
bounded response. Custom ToolCaller/storage implementations must honor context
and bounded reads; the client cannot undo memory allocated by a hostile plugin.

`MCPStorage.List` independently caps depth/entries at 32/5000 and rejects backend
over-return. Per-response limits remain even without an aggregate context budget.
Exhaustion is distinguishable as `files.ErrMCPBudgetExceeded` for HTTP quotas.

Baseline `042437b6015f5ba8f185ef91b05b2bd3a5a420f3` accepted sibling session paths,
forwarded maximal positive limits, and read overlarge summaries. Tests now verify
no cross-scope side effects, pre-call clamping, bounded fanout/aggregate content,
quota accounting on partial failures, nested limits and cancellation. Run
`go test -race ./agents/memory ./agents/files`.

Compatibility: project-wide administrative listing through a mismatched session
is no longer accepted. Use an explicitly authorized lower-level storage operation
for project-wide administration. Oversized summary requests must be partitioned
or reduced. There is no stored-data migration; rollback reintroduces the gap.

## Compatibility: the root is additional to the child limit

`limit` continues to bound backend entries, not root-plus-child summaries. One
child directory with `limit=1` returns both the root and that child. The derived
directory budget and MCP attempt budget reserve one extra root slot; backend
over-return, session confinement, depth, elapsed-time and byte limits remain
enforced. Root/child boundary cases are retained as behavior regressions.
