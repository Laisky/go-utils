# Transactional local memory storage (Linux)

`transactionlocal.NewEngine(local.Config{RootDir: dedicatedRoot})` is an opt-in
backend for `memory.NewTransactionalEngine`. Existing `local.NewEngine`, MCP
adapters, `memory.NewEngine`, paths inside each project, and JSON/JSONL formats
are unchanged. The new backend deliberately uses a separate root layout.

The adapter holds a kernel `flock` across the entire operation, including calls
to heuristic clients. Independent adapter instances and processes using the same
root serialize through that lock. This is a whole-root lock, including reads;
separate sessions are serialized too. A transaction copies the published
snapshot, applies its callback to the private copy, synchronizes every file and
child directory, and atomically publishes a synchronized generation pointer.
Callback errors, any failed write/delete (even when caught by the callback), and
cancellation observed before publication discard the private snapshot. The view
is valid only synchronously inside the callback, for its specified project. Do
not retain it, use it in goroutines, or reenter the adapter from the callback.

A killed process before pointer publication leaves an unreachable snapshot.
A killed process after publication leaves the complete new snapshot. Reopening
uses only the pointer; it never guesses from modification times or falls back to
an uncommitted snapshot. The next successful commit removes abandoned and
superseded generations. An error after publication, including a final directory
sync or obsolete-generation cleanup error, may mean the transaction committed.
Retry by the same turn ID to reconcile that outcome. Process termination testing
covers pre-publication discard and successful publication followed by restart;
it does not simulate storage controller failure or prove physical power-loss
behavior.

The root must be private to cooperating users on one local filesystem that
supports advisory `flock`, atomic rename, and file/directory `fsync`. Network
filesystems, other processes modifying generations directly, device corruption,
and filesystems that do not honor these primitives are outside the guarantee.
The backend is Linux-only. Its files and directories default to owner-only
permissions through the existing local backend. It refuses a root containing
legacy project directories. Symlinks and other nonregular entries inside a
snapshot are rejected rather than copied. Invalid or missing published
generations fail closed.

Each successful operation copies the whole current tree and syncs the result.
This trades throughput and temporary disk capacity for a small, inspectable
transaction protocol. Provision enough space for two complete snapshots and
staging left behind by interrupted attempts. For large workloads, implement
`storage.TransactionalEngine` using a backend-native serializable transaction
instead of adopting this adapter.

## Migration and rollback

1. Stop **all** memory readers/writers and maintenance using the old destination.
   Preserve a backup, and reconcile any already-partial historical turns before
   migration; transactions cannot reconstruct previously lost facts or markers.
2. Create a new, empty, private root. Copy the existing local storage root's
   project directories into a `generation-import` directory beneath it. Preserve
   contents and permissions. Set the `current` file to exactly
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
