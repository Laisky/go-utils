//go:build linux

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTransactionalTurnUTCShardRetry checks retry before and after a lost commit
// acknowledgement across midnight without leaving a duplicate in another shard.
func TestTransactionalTurnUTCShardRetry(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "aborted"
		if committed {
			name = "committed"
		}
		t.Run(name, func(t *testing.T) {
			backend := newTurnBackend(t)
			fault := &turnFaultBackend{TransactionalEngine: backend, loseAck: committed}
			// The raw append is the first turn write; fail after it if not committed.
			if !committed {
				fault.failAt = 2
				fault.afterApply = true
			}
			first, err := NewTransactionalEngine(fault, recoveryConfig())
			require.NoError(t, err)
			require.Error(t, first.AfterTurn(context.Background(), recoveryInput("one")))
			conf := recoveryConfig()
			tomorrow := recoveryTime.Add(24 * time.Hour)
			conf.TimeNow = func() time.Time { return tomorrow }
			retry, err := NewTransactionalEngine(backend, conf)
			require.NoError(t, err)
			require.NoError(t, retry.AfterTurn(context.Background(), recoveryInput("one")))
			old, err := backend.Stat(context.Background(), recoveryProject, rawLogShardPath(recoverySession, recoveryTime))
			require.NoError(t, err)
			next, err := backend.Stat(context.Background(), recoveryProject, rawLogShardPath(recoverySession, tomorrow))
			require.NoError(t, err)
			require.Equal(t, committed, old.Exists)
			require.Equal(t, !committed, next.Exists)
			reader, err := NewEngine(backend, conf)
			require.NoError(t, err)
			events, err := reader.loadRawEvents(context.Background(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.Equal(t, "one-in-0", events[0].ID)
		})
	}
}

// TestTransactionalTurnRetainedReceiptCompatibility preserves the historical
// bounded receipt window and immutable-ID requirement without promising infinite
// deduplication or silently introducing input-conflict storage semantics.
func TestTransactionalTurnRetainedReceiptCompatibility(t *testing.T) {
	backend := newTurnBackend(t)
	conf := recoveryConfig()
	conf.MaxProcessedTurns = 1
	engine, err := NewTransactionalEngine(backend, conf)
	require.NoError(t, err)
	require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput("one")))
	before := turnSnapshot(t, backend)
	reused := recoveryInput("one")
	reused.InputItems[0].Content[0].Text = "different input with reused ID"
	require.NoError(t, engine.AfterTurn(context.Background(), reused))
	require.Equal(t, before, turnSnapshot(t, backend), "retained ID keeps historical suppression behavior")
	require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput("two")))
	require.NoError(t, engine.AfterTurn(context.Background(), recoveryInput("one")))
	reader, err := NewEngine(backend, conf)
	require.NoError(t, err)
	events, err := reader.loadRawEvents(context.Background(), recoveryProject, recoverySession)
	require.NoError(t, err)
	require.Len(t, events, 3, "evicted IDs do not have unlimited historical deduplication")
}
