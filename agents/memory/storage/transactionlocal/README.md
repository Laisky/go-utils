# Transactional local memory storage (Linux)

`transactionlocal.NewEngine(local.Config{RootDir: dedicatedRoot})` is an opt-in
backend for `memory.NewTransactionalEngine`. Existing `local.NewEngine`, MCP
adapters, `memory.NewEngine`, paths inside each project, and JSON/JSONL formats
are unchanged. The new backend deliberately uses a separate root layout.

Mutable transactions hold an exclusive kernel `flock` across the whole callback,
including calls to heuristic clients. Independent instances and processes using
one root serialize mutations through that lock. Reads initially use the current
immutable generation directly. The first write or delete creates a private full
copy; subsequent operations use that copy. A callback that never mutates does
not copy the tree, sync snapshot data, publish a pointer, or collect generations.
For a previously empty root, actual reads may create and remove an empty local
namespace to retain the local adapter's validation and missing-file semantics;
a callback doing no storage operations creates no namespace.

`ReadTransaction` supplies a stable snapshot under a shared lock, allowing
concurrent readers while excluding publication and generation collection.
Direct `Read`, `Stat`, `List`, and `Search` use this path. Read-only callbacks
reject writes/deletes, even if the callback catches the error. Operations that
can lazily generate a summary must use ordinary `Transaction`: they acquire an
exclusive lock but stage only if a write is actually needed.

A mutating transaction synchronizes every staged file and child directory and
atomically publishes a synchronized generation pointer. Callback errors, any
failed write/delete (even when caught by the callback), and cancellation observed
before publication discard the private snapshot. The view is valid only
synchronously inside the callback, for its specified project. Do not retain it,
use it in goroutines, or reenter the adapter from the callback. Copied files and
directories retain access and modification times; actual mutations update their
normal filesystem timestamps. Copies use separate file inodes, never hardlinks,
so a failed mutation cannot alter the currently published generation.

A killed process before pointer publication leaves an unreachable snapshot.
A killed process after publication leaves the complete new snapshot. Reopening
uses only the pointer; it never guesses from modification times or falls back to
an uncommitted snapshot. The next successful commit removes abandoned and
superseded generations. Post-publication failures carry `storage.CommitError`:
`CommitOutcomeUncertain` means publication occurred but final directory sync
could not confirm durability; `CommitOutcomeDurable` means only later cleanup
or acknowledgement failed after durable commit. Inspect `storage.PublishedCommit`
through wrapped errors. These classifications do not make arbitrary callbacks
replay-safe. AfterTurn can retry the same retained TurnID; BeforeTurn metrics
count explicit calls and may increment again on deliberate replay. Prepared
BeforeTurn/summary output remains available with a classified diagnostic, so
retrieving results does not force replay. Other providers may return unclassified
errors, which never prove rollback. Process termination testing
covers pre-publication discard and successful publication followed by restart;
it does not simulate storage controller failure or prove physical power-loss
behavior.

The root must be private to cooperating users on one local filesystem that
supports advisory `flock`, atomic rename, and file/directory `fsync`. The
constructor requires root ownership by the effective user and rejects group/world
write permissions without silently chmodding the directory. Ancestors must be
owned by that user or root and must not grant group/world writes, except trusted
sticky temporary directories such as `/tmp`. A canonical symlink alias to a safe
root is resolved once; changing the alias later cannot redirect an existing
engine. Each lock acquisition revalidates the root's identity and protection.
Lock files must be single-link regular files owned by that user without
untrusted write permission. The pointer and live generation receive equivalent
ownership/type/write-permission checks; pointer symlinks, hardlinks and special
files are rejected before reading.

Read/traverse bits and explicitly chosen payload `FilePerm`/`DirPerm` remain
caller policy. New generations are created owner-only. Imports must preserve
trusted ownership and ensure no payload file or directory grants untrusted
mutation access; these control-entry checks are not a recursive import audit.
Same-user/root attackers, owners concurrently changing namespace permissions,
network filesystems, out-of-protocol generation mutation, device corruption and
filesystems that ignore the required primitives are outside the guarantee.
Namespace checks do not claim protection from a malicious trusted owner.

The backend is Linux-only. Files and directories default to owner-only
permissions through the existing local backend. It refuses a root containing
legacy project directories. Symlinks and other nonregular entries inside a
snapshot are rejected rather than copied. Invalid or missing published
generations fail closed.

Each mutating transaction still copies the whole current tree and syncs the
result; unrelated session history therefore affects write cost. Existing
snapshot reads and no-op transactions avoid this cost. This trades write
throughput and temporary disk capacity for an inspectable transaction protocol.
Provision enough space for two complete snapshots and staging left behind by interrupted attempts. For large workloads, implement
`storage.TransactionalEngine` using a backend-native serializable transaction
instead of adopting this adapter.

## Migration and rollback

1. Stop **all** memory readers/writers and maintenance using the old destination.
   Preserve a backup, and reconcile any already-partial historical turns before
   migration; transactions cannot reconstruct previously lost facts or markers.
2. Create a new, empty, private root. Copy the existing local storage root's
   project directories into a `generation-import` directory beneath it. Preserve
   contents and permissions, retaining effective-user ownership and no untrusted
   write access to imported data. The generation directory and `current` pointer
   must pass the ownership and permission checks above. Set `current` to exactly
   `generation-import` (no newline), synchronize the files and directories, and
   keep the old root unchanged. A provider-native migration can instead copy
   files through one transaction view.
3. Switch every participant to this adapter and `memory.NewTransactionalEngine`.
   Do not point legacy local/MCP writers at an internal generation. Mixed
   transactional and legacy writers cannot share a destination safely.
4. For rollback, stop all participants, read the current pointer, and copy that
   complete generation's project directories to a new legacy root. Verify the
   copy before switching back. Old code reads the same JSON/JSONL bytes, but its
   former partial-write/concurrency limitations return after rollback.

Rolling binary upgrades are safe when both binaries use the same transaction
contract. A migration involving a legacy writer requires the coordinated stop
above; there is no claim of safe mixed-mode rolling writes. The compatibility
suite reads committed content through the unchanged local backend and retains
historical memory fixtures.
