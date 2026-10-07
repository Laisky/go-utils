package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============================================================
// Unit Tests – MemoryRateLimiterStateManager
// ============================================================

func TestMemoryRateLimiterStateManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	shouldRefill, err := manager.Setup(ctx, args, 3)
	require.NoError(t, err)
	require.True(t, shouldRefill)

	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, tokens)

	added, err := manager.AddTokens(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 7, added)

	ok, err := manager.TryConsume(ctx, 4)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = manager.TryConsume(ctx, 20)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, manager.SetAvailableTokens(ctx, 2))

	shouldRefill, err = manager.Setup(ctx, args, 6)
	require.NoError(t, err)
	require.False(t, shouldRefill)

	added, err = manager.AddTokens(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, 5, added)

	tokens, err = manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, tokens)
}

func TestMemoryStateManagerUninitialized(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()

	// All operations on an uninitialized manager should fail
	_, err := manager.TryConsume(ctx, 1)
	require.Error(t, err)

	_, err = manager.AddTokens(ctx, 1)
	require.Error(t, err)

	_, err = manager.AvailableTokens(ctx)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 1)
	require.Error(t, err)
}

func TestMemoryStateManagerCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	manager := NewMemoryRateLimiterStateManager()

	_, err := manager.Setup(ctx, RateLimiterArgs{NPerSec: 1, Max: 1}, 1)
	require.Error(t, err)

	// Initialize with a good context first
	goodCtx := context.Background()
	_, err = manager.Setup(goodCtx, RateLimiterArgs{NPerSec: 1, Max: 1}, 1)
	require.NoError(t, err)

	_, err = manager.TryConsume(ctx, 1)
	require.Error(t, err)

	_, err = manager.AddTokens(ctx, 1)
	require.Error(t, err)

	_, err = manager.AvailableTokens(ctx)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 1)
	require.Error(t, err)
}

func TestMemoryStateManagerAddTokensEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 10)
	require.NoError(t, err)

	// Already at max, adding should return 0
	added, err := manager.AddTokens(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, 0, added)

	// Adding zero or negative returns 0
	added, err = manager.AddTokens(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, 0, added)

	added, err = manager.AddTokens(ctx, -1)
	require.NoError(t, err)
	require.Equal(t, 0, added)
}

func TestMemoryStateManagerTryConsumeEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	// Consuming 0 always succeeds
	ok, err := manager.TryConsume(ctx, 0)
	require.NoError(t, err)
	require.True(t, ok)

	// Consuming negative always succeeds
	ok, err = manager.TryConsume(ctx, -1)
	require.NoError(t, err)
	require.True(t, ok)

	// Tokens unchanged after zero/negative consume
	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 5, tokens)
}

func TestMemoryStateManagerSetupMismatchedArgs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	// Second setup with different args should fail
	_, err = manager.Setup(ctx, RateLimiterArgs{NPerSec: 3, Max: 10}, 3)
	require.Error(t, err)
}

func TestMemoryStateManagerSetAvailableTokensBounds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 5, Max: 10}

	_, err := manager.Setup(ctx, args, 5)
	require.NoError(t, err)

	err = manager.SetAvailableTokens(ctx, -1)
	require.Error(t, err)

	err = manager.SetAvailableTokens(ctx, 11)
	require.Error(t, err)

	// Valid boundary values
	require.NoError(t, manager.SetAvailableTokens(ctx, 0))
	tokens, err := manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, tokens)

	require.NoError(t, manager.SetAvailableTokens(ctx, 10))
	tokens, err = manager.AvailableTokens(ctx)
	require.NoError(t, err)
	require.Equal(t, 10, tokens)
}

// ============================================================
// Unit Tests – Shared state manager
// ============================================================

func TestRateLimiterWithSharedStateManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := NewMemoryRateLimiterStateManager()
	args := RateLimiterArgs{NPerSec: 2, Max: 4}

	limiterA, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterA.Close)

	limiterB, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterB.Close)

	require.True(t, limiterA.Allow())
	require.True(t, limiterB.Allow())

	require.False(t, limiterA.Allow())
	require.False(t, limiterB.Allow())

	time.Sleep(1100 * time.Millisecond)

	require.True(t, limiterA.Allow())
	require.True(t, limiterB.Allow())
	require.False(t, limiterA.Allow())
}

func TestWithRateLimiterStateManagerNilRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 1, Max: 1},
		WithRateLimiterStateManager(nil))
	require.Error(t, err)
}
