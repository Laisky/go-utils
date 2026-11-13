package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
)

// admissionTransactionBackend records transaction entry before running a test callback.
// It models the admission boundary, not durable transaction guarantees.
type admissionTransactionBackend struct {
	storageengine.Engine
	calls            int
	project, session string
	enter            func(context.Context) error
}

// Transaction records calls and can simulate staging or lock waits before callback entry.
func (backend *admissionTransactionBackend) Transaction(
	ctx context.Context, project, session string, fn func(storageengine.Engine) error,
) error {
	backend.calls++
	backend.project, backend.session = project, session
	if backend.enter != nil {
		if err := backend.enter(ctx); err != nil {
			return err
		}
	}
	return fn(backend.Engine)
}

// TestTransactionalManagementValidatesBeforeBackend rejects invalid namespace and
// summary paths before any backend admission, staging, or callback can execute.
func TestTransactionalManagementValidatesBeforeBackend(t *testing.T) {
	for _, invalid := range []struct{ name, project, session string }{
		{"empty_project", "", "alice"}, {"blank_project", " \t", "alice"},
		{"empty_session", "demo", ""}, {"traversal", "demo", ".."},
		{"nested_session", "demo", "alice/bob"}, {"long_session", "demo", strings.Repeat("x", 129)},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			for _, operation := range []string{"maintenance", "consolidation", "summary"} {
				t.Run(operation, func(t *testing.T) {
					backend := &admissionTransactionBackend{Engine: newMemoryStorageMock()}
					engine, err := NewTransactionalEngine(backend, recoveryConfig())
					require.NoError(t, err)
					switch operation {
					case "maintenance":
						err = engine.RunMaintenance(t.Context(), invalid.project, invalid.session)
					case "consolidation":
						err = engine.RunConsolidation(t.Context(), invalid.project, invalid.session)
					case "summary":
						_, err = engine.ListDirWithAbstract(t.Context(), invalid.project, invalid.session, "", 2, 5)
					}
					require.Error(t, err)
					require.Zero(t, backend.calls, "invalid input must not reach expensive backend admission")
				})
			}
		})
	}
	for _, path := range []string{"/", "/memory/bob", "/memory/alice-other", "/memory/alice/../bob", "memory/alice"} {
		t.Run(path, func(t *testing.T) {
			backend := &admissionTransactionBackend{Engine: newMemoryStorageMock()}
			engine, err := NewTransactionalEngine(backend, recoveryConfig())
			require.NoError(t, err)
			out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", path, 2, 5)
			require.Error(t, err)
			require.Nil(t, out)
			require.Zero(t, backend.calls)
		})
	}
}

// TestTransactionalManagementRetainsValidInputs controls against rejecting all
// requests or trimming valid caller namespace values at the transaction boundary.
func TestTransactionalManagementRetainsValidInputs(t *testing.T) {
	for _, operation := range []string{"maintenance", "consolidation", "summary"} {
		t.Run(operation, func(t *testing.T) {
			backend := &admissionTransactionBackend{Engine: newMemoryStorageMock()}
			engine, err := NewTransactionalEngine(backend, recoveryConfig())
			require.NoError(t, err)
			switch operation {
			case "maintenance":
				err = engine.RunMaintenance(t.Context(), " demo ", " alice ")
			case "consolidation":
				err = engine.RunConsolidation(t.Context(), " demo ", " alice ")
			case "summary":
				_, err = engine.ListDirWithAbstract(t.Context(), " demo ", "alice", " /memory/alice/sub ", -1, -1)
			}
			require.NoError(t, err)
			require.Equal(t, 1, backend.calls)
			require.Equal(t, " demo ", backend.project)
			if operation == "summary" {
				require.Equal(t, "alice", backend.session)
			} else {
				require.Equal(t, " alice ", backend.session)
			}
		})
	}
}

// TestTransactionalSummaryDeadlineAtAdmission proves the ten-second budget
// already applies when a backend begins locking or staging, before its callback.
func TestTransactionalSummaryDeadlineAtAdmission(t *testing.T) {
	backend := &admissionTransactionBackend{Engine: newMemoryStorageMock()}
	var deadline time.Time
	var bounded bool
	backend.enter = func(ctx context.Context) error {
		deadline, bounded = ctx.Deadline()
		return errors.New("admission observation complete")
	}
	engine, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	started := time.Now()
	out, err := engine.ListDirWithAbstract(t.Context(), "demo", "alice", "", 1, 1)
	require.ErrorContains(t, err, "admission observation complete")
	require.Nil(t, out)
	require.Equal(t, 1, backend.calls)
	require.True(t, bounded, "summary deadline must precede backend work")
	require.WithinDuration(t, started.Add(summaryTimeout), deadline, time.Second)
}

// TestTransactionalSummaryBackendWaitCancellation checks a blocked backend exits
// on the caller's shorter deadline without entering a storage callback.
func TestTransactionalSummaryBackendWaitCancellation(t *testing.T) {
	backend := &admissionTransactionBackend{Engine: newMemoryStorageMock()}
	entered := make(chan struct{})
	backend.enter = func(ctx context.Context) error { close(entered); <-ctx.Done(); return errors.WithStack(ctx.Err()) }
	engine, err := NewTransactionalEngine(backend, recoveryConfig())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	out, err := engine.ListDirWithAbstract(ctx, "demo", "alice", "", 1, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, out)
	require.Equal(t, 1, backend.calls)
	select {
	case <-entered:
	default:
		t.Fatal("backend admission was not exercised")
	}
}
