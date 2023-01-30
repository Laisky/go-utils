package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAsyncTask(t *testing.T) {
	ctx := context.Background()

	store := NewAsyncTaskStoreMemory()

	at1, err := NewAsyncTask(ctx, store)
	require.NoError(t, err)

	at2, err := NewAsyncTask(ctx, store)
	require.NoError(t, err)

	require.NotEqual(t, at1.ID(), at2.ID())
	require.Equal(t, at1.Status(), at2.Status())
	require.Equal(t, AsyncTaskStatusPending, at2.Status())

	err = at1.SetDone(ctx, "done")
	require.NoError(t, err)
	require.Equal(t, AsyncTaskStatusDone, at1.Status())
	atr1, err := store.Get(ctx, at1.ID())
	require.NoError(t, err)
	require.Equal(t, "done", atr1.Data)

	err = at2.SetError(ctx, "oho")
	require.NoError(t, err)
	require.Equal(t, AsyncTaskStatusFailed, at2.Status())
	atr2, err := store.Get(ctx, at2.ID())
	require.NoError(t, err)
	require.Equal(t, "oho", atr2.Err)
}
