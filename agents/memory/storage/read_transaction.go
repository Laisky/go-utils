package storage

import "context"

// ReadTransactionalEngine optionally supplies one stable read-only snapshot.
// The callback's Engine is valid synchronously for the selected project only;
// writes and deletes must fail rather than publish changes. Implementations may
// admit concurrent readers, while writers wait until every snapshot is released.
// Callers that may lazily refresh persisted summaries must use Transaction.
type ReadTransactionalEngine interface {
	TransactionalEngine
	ReadTransaction(ctx context.Context, project, sessionID string, fn func(Engine) error) error
}
