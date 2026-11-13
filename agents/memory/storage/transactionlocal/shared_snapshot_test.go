//go:build linux

package transactionlocal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestReadTransactionSharedSnapshots checks concurrent admission and exclusion of writers.
func TestReadTransactionSharedSnapshots(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/value", "old", storage.WriteModeTruncate, 0))
	other, err := NewEngine(local.Config{RootDir: engine.root})
	require.NoError(t, err)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	reader := func(backend *Engine) {
		results <- backend.ReadTransaction(ctx, "p", "s", func(view storage.Engine) error {
			entered <- struct{}{}
			<-release
			value, readErr := view.Read(ctx, "p", "/value", 0, -1)
			if readErr != nil {
				return readErr
			}
			if value != "old" {
				return fmt.Errorf("unexpected snapshot value %q", value)
			}
			return nil
		})
	}
	go reader(engine)
	go reader(other)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for range 2 {
		select {
		case <-entered:
		case <-deadline.C:
			close(release)
			t.Fatal("readers must share a snapshot concurrently")
		}
	}
	waiting, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err = other.Write(waiting, "p", "/value", "blocked", storage.WriteModeTruncate, 0)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.ErrorContains(t, engine.ReadTransaction(ctx, "p", "s", func(view storage.Engine) error {
		// A caught mutation error still invalidates the read-only operation.
		_ = view.Write(ctx, "p", "/value", "bad", storage.WriteModeTruncate, 0)
		return nil
	}), "cannot mutate")
}

// TestReadTransactionMutationFailure requires caught mutations to fail without publication.
func TestReadTransactionMutationFailure(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/value", "old", storage.WriteModeTruncate, 0))
	before, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	var retained storage.Engine
	err = engine.ReadTransaction(ctx, "p", "s", func(view storage.Engine) error {
		retained = view
		_ = view.Delete(ctx, "p", "/value", false)
		return nil
	})
	require.ErrorContains(t, err, "cannot mutate")
	_, err = retained.Read(ctx, "p", "/value", 0, -1)
	require.ErrorContains(t, err, "closed")
	after, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestTransactionLazyCopyIsolation checks staging begins at mutation and never shares file inodes.
func TestTransactionLazyCopyIsolation(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	require.NoError(t, engine.Write(ctx, "p", "/value", "old", storage.WriteModeTruncate, 0))
	pointer, err := os.ReadFile(filepath.Join(engine.root, "current"))
	require.NoError(t, err)
	original := filepath.Join(engine.root, string(pointer), "p/value")
	originalInfo, err := os.Stat(original)
	require.NoError(t, err)
	require.NoError(t, engine.Transaction(ctx, "p", "s", func(view storage.Engine) error {
		value, readErr := view.Read(ctx, "p", "/value", 0, -1)
		require.NoError(t, readErr)
		require.Equal(t, "old", value)
		require.NoError(t, view.Write(ctx, "p", "/value", "new", storage.WriteModeTruncate, 0))
		lazy := view.(*transactionView)
		require.NotEmpty(t, lazy.staged)
		stagedInfo, statErr := os.Stat(filepath.Join(lazy.staged, "p/value"))
		require.NoError(t, statErr)
		require.False(t, os.SameFile(originalInfo, stagedInfo), "hardlinks would break rollback isolation")
		unchanged, readErr := os.ReadFile(original)
		require.NoError(t, readErr)
		require.Equal(t, "old", string(unchanged))
		return nil
	}))
}

// TestTransactionConcurrentSessionsAndReaders checks complete paired snapshots under bounded writes.
func TestTransactionConcurrentSessionsAndReaders(t *testing.T) {
	engine := newTestEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	results := make(chan error, 12)
	for session := range 4 {
		workers.Go(func() {
			results <- engine.Transaction(ctx, "p", fmt.Sprint(session), func(view storage.Engine) error {
				return view.Write(ctx, "p", fmt.Sprintf("/session-%d/value", session), "kept", storage.WriteModeTruncate, 0)
			})
		})
	}
	workers.Wait()
	for range 4 {
		require.NoError(t, <-results)
	}
	for session := range 4 {
		value, err := engine.Read(ctx, "p", fmt.Sprintf("/session-%d/value", session), 0, -1)
		require.NoError(t, err)
		require.Equal(t, "kept", value)
	}
	require.NoError(t, engine.Transaction(ctx, "p", "pair", func(view storage.Engine) error {
		if err := view.Write(ctx, "p", "/pair/a", "0", storage.WriteModeTruncate, 0); err != nil {
			return err
		}
		return view.Write(ctx, "p", "/pair/b", "0", storage.WriteModeTruncate, 0)
	}))
	for range 2 {
		workers.Go(func() {
			for range 8 {
				err := engine.ReadTransaction(ctx, "p", "pair", func(view storage.Engine) error {
					a, err := view.Read(ctx, "p", "/pair/a", 0, -1)
					if err != nil {
						return err
					}
					b, err := view.Read(ctx, "p", "/pair/b", 0, -1)
					if err != nil {
						return err
					}
					if a != b {
						return fmt.Errorf("torn pair %q/%q", a, b)
					}
					return nil
				})
				if err != nil {
					results <- err
					return
				}
			}
			results <- nil
		})
	}
	workers.Go(func() {
		for i := range 8 {
			err := engine.Transaction(ctx, "p", "pair", func(view storage.Engine) error {
				value := fmt.Sprint(i)
				if err := view.Write(ctx, "p", "/pair/a", value, storage.WriteModeTruncate, 0); err != nil {
					return err
				}
				return view.Write(ctx, "p", "/pair/b", value, storage.WriteModeTruncate, 0)
			})
			if err != nil {
				results <- err
				return
			}
		}
		results <- nil
	})
	workers.Wait()
	for range 3 {
		require.NoError(t, <-results)
	}
}
