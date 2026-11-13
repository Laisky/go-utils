//go:build linux

package transactionlocal

import (
	"context"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestTransactionPublicationOutcome classifies an actual post-rename durability
// failure while retaining a complete visible generation and the previous one.
func TestTransactionPublicationOutcome(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	mutate := func(value string) func(storage.Engine) error {
		return func(view storage.Engine) error {
			for _, path := range []string{"/first", "/second"} {
				if err := view.Write(ctx, "p", path, value, storage.WriteModeTruncate, 0); err != nil {
					return err
				}
			}
			return nil
		}
	}
	require.NoError(t, engine.Transaction(ctx, "p", "s", mutate("old")))
	prior, err := engine.current()
	require.NoError(t, err)
	injected := errors.New("synthetic published directory fsync failure")
	syncCalls := 0
	engine.commitSync = func(string) error { syncCalls++; return injected }
	err = engine.Transaction(ctx, "p", "s", mutate("new"))
	require.ErrorIs(t, err, injected)
	outcome, published := storage.PublishedCommit(err)
	require.True(t, published)
	require.Equal(t, storage.CommitOutcomeUncertain, outcome)
	require.Equal(t, 1, syncCalls)
	current, err := engine.current()
	require.NoError(t, err)
	require.NotEqual(t, prior, current)
	reopened, err := NewEngine(local.Config{RootDir: engine.root})
	require.NoError(t, err)
	for _, path := range []string{"/first", "/second"} {
		body, err := reopened.Read(ctx, "p", path, 0, -1)
		require.NoError(t, err)
		require.Equal(t, "new", body, "published state must be complete")
	}
	// A failed callback is an abort, never a post-publication outcome, and a
	// no-mutation callback never reaches the injected publication/fsync operation.
	err = engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		if err := view.Write(ctx, "p", "/first", "aborted", storage.WriteModeTruncate, 0); err != nil {
			return err
		}
		return errors.New("synthetic callback abort")
	})
	require.ErrorContains(t, err, "callback abort")
	_, published = storage.PublishedCommit(err)
	require.False(t, published)
	require.NoError(t, engine.Transaction(ctx, "p", "s", func(storage.Engine) error { return nil }))
	require.Equal(t, 1, syncCalls)
	body, err := reopened.Read(ctx, "p", "/first", 0, -1)
	require.NoError(t, err)
	require.Equal(t, "new", body)
}
