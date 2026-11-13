package memory

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// TransactionalEngine applies each lifecycle or management operation in one
// backend transaction. Use NewTransactionalEngine to require this stronger
// capability explicitly. Persisted records retain the StandardEngine format.
type TransactionalEngine struct {
	backend  storageengine.TransactionalEngine
	template *StandardEngine
}

var _ Engine = (*TransactionalEngine)(nil)
var _ Management = (*TransactionalEngine)(nil)

// NewTransactionalEngine creates an engine whose turns, compaction, and
// maintenance share durable storage-backed transactions. It returns an error for
// storage that does not implement the transaction contract. All readers/writers
// of its destination must participate; see docs/memory-transactions.md for
// migration, retry, and rollback requirements.
func NewTransactionalEngine(backend storageengine.Engine, conf Config) (*TransactionalEngine, error) {
	tx, ok := backend.(storageengine.TransactionalEngine)
	if !ok {
		return nil, errors.New("storage does not support durable memory transactions")
	}
	template, err := newStandardEngine(backend, conf)
	if err != nil {
		return nil, errors.Wrap(err, "configure transactional memory engine")
	}
	return &TransactionalEngine{backend: tx, template: template}, nil
}

// transaction runs fn with an isolated engine and returns callback, mutation, or
// commit errors. It never changes shared engine fields during concurrent calls.
func (engine *TransactionalEngine) transaction(
	ctx context.Context, project, sessionID string, fn func(*StandardEngine) error,
) error {
	if strings.TrimSpace(project) == "" {
		return errors.New("project is required")
	}
	if err := validateSessionID(sessionID); err != nil {
		return errors.Wrap(err, "validate memory transaction session")
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "start memory transaction")
	}
	if err := engine.backend.Transaction(ctx, project, sessionID, func(view storageengine.Engine) error {
		storage := &transactionMutations{view: view}
		bound := *engine.template
		bound.storage = storage
		bound.strictPersistence = true
		if err := fn(&bound); err != nil {
			return err
		}
		if storage.err != nil {
			return errors.Wrap(storage.err, "abort failed memory storage operation")
		}
		return errors.Wrap(ctx.Err(), "finish memory transaction")
	}); err != nil {
		return errors.Wrap(err, "memory transaction")
	}
	return nil
}

// BeforeTurn prepares model input and commits any compaction and metrics in one
// transaction. A classified post-publication error returns the prepared output
// together with that error, so callers need not replay recall solely to retrieve
// its result. Other failures return no output; see storage.PublishedCommit.
func (engine *TransactionalEngine) BeforeTurn(ctx context.Context, in BeforeTurnInput) (BeforeTurnOutput, error) {
	if err := validateBeforeTurnInput(in); err != nil {
		return BeforeTurnOutput{}, errors.Wrap(err, "validate before-turn input")
	}
	var out BeforeTurnOutput
	err := engine.transaction(ctx, in.Project, in.SessionID, func(bound *StandardEngine) error {
		var err error
		out, err = bound.BeforeTurn(ctx, in)
		return err
	})
	if err != nil {
		if _, published := storageengine.PublishedCommit(err); published {
			return out, err
		}
		return BeforeTurnOutput{}, err
	}
	return out, nil
}

// AfterTurn commits events, facts, indexes, watermarks, and the processed marker
// together, returning an error when any persistent mutation or commit fails.
func (engine *TransactionalEngine) AfterTurn(ctx context.Context, in AfterTurnInput) error {
	if err := validateAfterTurnInput(in); err != nil {
		return errors.Wrap(err, "validate after-turn input")
	}
	return engine.transaction(ctx, in.Project, in.SessionID, func(bound *StandardEngine) error {
		return bound.AfterTurn(ctx, in)
	})
}

// RunMaintenance commits session maintenance under the same transaction as turns.
func (engine *TransactionalEngine) RunMaintenance(ctx context.Context, project, sessionID string) error {
	return engine.transaction(ctx, project, sessionID, func(bound *StandardEngine) error {
		return bound.RunMaintenance(ctx, project, sessionID)
	})
}

// RunConsolidation commits consolidation under the same transaction as turns.
func (engine *TransactionalEngine) RunConsolidation(ctx context.Context, project, sessionID string) error {
	return engine.transaction(ctx, project, sessionID, func(bound *StandardEngine) error {
		return bound.RunConsolidation(ctx, project, sessionID)
	})
}

// ListDirWithAbstract reads directory summaries from one consistent transaction.
// Classified post-publication errors retain the prepared summaries with the
// diagnostic; callers must inspect the outcome before considering a retry.
func (engine *TransactionalEngine) ListDirWithAbstract(
	ctx context.Context, project, sessionID, path string, depth, limit int,
) ([]DirectorySummary, error) {
	root, err := validateSummaryRequest(project, sessionID, path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	var out []DirectorySummary
	err = engine.transaction(ctx, project, sessionID, func(bound *StandardEngine) error {
		var err error
		out, err = bound.ListDirWithAbstract(ctx, project, sessionID, root, depth, limit)
		return err
	})
	if err != nil {
		if _, published := storageengine.PublishedCommit(err); published {
			return out, err
		}
		return nil, err
	}
	return out, nil
}

// transactionMutations remembers failed storage operations, including best-effort
// reads and writes whose errors the legacy implementation intentionally suppresses.
type transactionMutations struct {
	view storageengine.Engine
	err  error
}

var _ storageengine.Engine = (*transactionMutations)(nil)

// Write forwards a mutation and remembers its first failure for transaction abort.
func (storage *transactionMutations) Write(
	ctx context.Context, project, path, content string, mode storageengine.WriteMode, offset int64,
) error {
	err := storage.view.Write(ctx, project, path, content, mode, offset)
	storage.remember(err)
	return err
}

// Delete forwards a deletion and remembers its first failure for transaction abort.
func (storage *transactionMutations) Delete(ctx context.Context, project, path string, recursive bool) error {
	err := storage.view.Delete(ctx, project, path, recursive)
	storage.remember(err)
	return err
}

// Read forwards a read and remembers its first failure for transaction abort.
func (storage *transactionMutations) Read(
	ctx context.Context, project, path string, offset, length int64,
) (string, error) {
	content, err := storage.view.Read(ctx, project, path, offset, length)
	storage.remember(err)
	return content, err
}

// Stat forwards metadata lookup and remembers its first failure for transaction abort.
func (storage *transactionMutations) Stat(ctx context.Context, project, path string) (storageengine.FileInfo, error) {
	info, err := storage.view.Stat(ctx, project, path)
	storage.remember(err)
	return info, err
}

// List forwards directory listing and remembers its first failure for transaction abort.
func (storage *transactionMutations) List(
	ctx context.Context, project, path string, depth, limit int,
) ([]storageengine.FileInfo, bool, error) {
	entries, more, err := storage.view.List(ctx, project, path, depth, limit)
	storage.remember(err)
	return entries, more, err
}

// Search forwards search and remembers its first failure for transaction abort.
func (storage *transactionMutations) Search(
	ctx context.Context, project, query, prefix string, limit int,
) ([]storageengine.FileChunk, error) {
	chunks, err := storage.view.Search(ctx, project, query, prefix, limit)
	storage.remember(err)
	return chunks, err
}

// remember retains the first failed storage operation for transaction abort.
func (storage *transactionMutations) remember(err error) {
	if err != nil && storage.err == nil {
		storage.err = err
	}
}
