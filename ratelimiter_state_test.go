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

// TestMemoryRateLimiterStateManager verifies the MemoryRateLimiterStateManager lifecycle: the first Setup asks
// the caller to refill, AddTokens is capped at Max, TryConsume rejects a request larger than Max,
// SetAvailableTokens overwrites the count, and a second Setup with the same args keeps the existing count
// (ignoring its initial tokens) and reports that no refill loop is needed.
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

// TestMemoryStateManagerUninitialized verifies that TryConsume, AddTokens, AvailableTokens, and
// SetAvailableTokens all return errors when called on a manager that has not been set up.
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

// TestMemoryStateManagerCancelledContext verifies that Setup fails with an already canceled context, and that
// after a successful Setup every other manager method also returns an error when given a canceled context.
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

// TestMemoryStateManagerAddTokensEdgeCases verifies that AddTokens adds nothing and returns no error when the
// manager is already at Max or when the requested amount is zero or negative.
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

// TestMemoryStateManagerTryConsumeEdgeCases verifies that TryConsume succeeds for zero and negative amounts
// without changing the available token count.
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

// TestMemoryStateManagerSetupMismatchedArgs verifies that calling Setup again with args that differ from those
// of the first Setup returns an error.
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

// TestMemoryStateManagerSetAvailableTokensBounds verifies that SetAvailableTokens rejects -1 and Max+1 while
// accepting and storing the boundary values 0 and Max.
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

// TestRateLimiterWithSharedStateManager verifies that two limiters sharing one MemoryRateLimiterStateManager
// draw from a single token pool and that only the first one runs a refill loop, so the shared pool refills at
// NPerSec rather than at twice that rate. Every bound is checked against counted refills or measured time, so a
// stalled refill goroutine on a loaded host cannot fail the test.
func TestRateLimiterWithSharedStateManager(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := &countingStateManager{MemoryRateLimiterStateManager: NewMemoryRateLimiterStateManager()}
	args := RateLimiterArgs{NPerSec: 10, Max: 20}

	start := time.Now()
	limiterA, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterA.Close)

	limiterB, err := NewRateLimiter(ctx, args, WithRateLimiterStateManager(manager))
	require.NoError(t, err)
	t.Cleanup(limiterB.Close)

	refillers, _ := manager.counts()
	require.Equal(t, 1, refillers, "only the first limiter sharing a manager may run a refill loop")

	// One pool: both limiters together get the initial NPerSec tokens plus whatever was refilled, never more.
	consumed := 0
	for limiterA.Allow() {
		consumed++
	}
	for limiterB.Allow() {
		consumed++
	}
	_, requested := manager.counts()
	require.GreaterOrEqual(t, consumed, args.NPerSec)
	require.LessOrEqual(t, consumed, args.NPerSec+requested, "the limiters must not have separate pools")

	// limiterB runs no refill loop of its own, so tokens it sees again come from limiterA's loop.
	awaitTokens(t, limiterB, 2)
	require.True(t, limiterB.AllowN(2))

	// A single refill loop requests at most NPerSec tokens per measured second; a doubled one would not.
	require.Eventually(t, func() bool {
		_, requested := manager.counts()
		return requested >= 2*args.NPerSec
	}, 30*time.Second, 10*time.Millisecond, "the shared pool must keep refilling")
	_, requested = manager.counts()
	require.LessOrEqual(t, float64(requested), float64(args.NPerSec)*time.Since(start).Seconds()+1,
		"refill of the shared pool must not exceed NPerSec")
}

// TestWithRateLimiterStateManagerNilRejected verifies that NewRateLimiter returns an error when the
// WithRateLimiterStateManager option is given a nil manager.
func TestWithRateLimiterStateManagerNilRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := NewRateLimiter(ctx, RateLimiterArgs{NPerSec: 1, Max: 1},
		WithRateLimiterStateManager(nil))
	require.Error(t, err)
}
