package storage

import "context"

// TransactionalEngine provides durable serializable transactions for memory
// lifecycle operations. Transaction must serialize all operations on the same
// project/session across clients and processes, including reads and maintenance.
// A backend may serialize more broadly. Its callback receives an isolated Engine
// view with read-your-writes behavior; that view must not escape the callback.
//
// A nil callback result commits every mutation atomically and durably. An error,
// panic, or cancellation observed before commit discards every mutation. A lost commit
// acknowledgement may return an error after commit: the complete committed state
// must be visible to the next transaction, allowing AfterTurn's marker to suppress
// replay. This does not make BeforeTurn metrics or arbitrary management operations
// replay-safe. When known, backends should classify post-publication failures with
// CommitError; callers must not automatically repeat non-idempotent operations.
// Individual operations in the view must not transparently retry a
// non-idempotent mutation after an ambiguous acknowledgement.
//
// Implementations must coordinate every writer to the destination. Wrapping a
// legacy backend with a process-local mutex does not satisfy this contract.
// Existing Engine implementations do not acquire this capability implicitly.
type TransactionalEngine interface {
	Engine
	Transaction(ctx context.Context, project, sessionID string, fn func(Engine) error) error
}
