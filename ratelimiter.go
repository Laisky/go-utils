package utils

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// ThrottleCfg Throttle's configuration
//
// Deprecated: use `RateLimiterArgs` instead
type ThrottleCfg RateLimiterArgs

// Throttle rate limitor
//
// Deprecated: use `RateLimiter` instead
type Throttle RateLimiter

// NewThrottleWithCtx create new Throttle
//
// Deprecated: use `NewRateLimiter` instead
var NewThrottleWithCtx = NewRateLimiter

// RateLimiterArgs Throttle's configuration
type RateLimiterArgs struct {
	Max, NPerSec int
}

// RateLimiterState tracks the exported state of a RateLimiter instance.
type RateLimiterState struct {
	Args            RateLimiterArgs
	AvailableTokens int
}

// RateLimiterStateManager coordinates rate limiter state across owners.
type RateLimiterStateManager interface {
	// Setup prepares the manager for the provided limiter args and returns true
	// if the caller should run the refill loop.
	Setup(ctx context.Context, args RateLimiterArgs, initialTokens int) (shouldRefill bool, err error)
	// TryConsume attempts to deduct the requested number of tokens.
	TryConsume(ctx context.Context, n int) (bool, error)
	// AddTokens increases the available tokens, returning the number actually added.
	AddTokens(ctx context.Context, n int) (int, error)
	// AvailableTokens returns the current token count.
	AvailableTokens(ctx context.Context) (int, error)
	// SetAvailableTokens overwrites the current token count.
	SetAvailableTokens(ctx context.Context, tokens int) error
}

// MemoryRateLimiterStateManager provides an in-memory implementation of the
// state manager interface. It is safe for concurrent use across goroutines and
// serves as the default state backend.
type MemoryRateLimiterStateManager struct {
	state       RateLimiterState
	initialized bool
	mu          rateLimiterSyncer
}

// rateLimiterSyncer abstracts the synchronization primitive used by the in-memory manager.
type rateLimiterSyncer struct{ ch chan struct{} }

// newRateLimiterSyncer creates a rateLimiterSyncer backed by a channel with a buffer of one, so that it behaves
// like a mutex. It takes no parameters and returns the ready-to-use syncer; a zero-value syncer has a nil channel
// and would block forever on lock.
func newRateLimiterSyncer() rateLimiterSyncer {
	return rateLimiterSyncer{ch: make(chan struct{}, 1)}
}

// lock acquires the syncer by sending into its single-slot channel, blocking until the current holder calls
// unlock. It takes no parameters and returns nothing; copies of a syncer share the same channel and thus the
// same lock.
func (s rateLimiterSyncer) lock() { s.ch <- struct{}{} }

// unlock releases the syncer by draining its single-slot channel. It takes no parameters and returns nothing;
// calling it without a matching lock blocks forever instead of panicking.
func (s rateLimiterSyncer) unlock() { <-s.ch }

// NewMemoryRateLimiterStateManager returns an in-memory state manager.
func NewMemoryRateLimiterStateManager() *MemoryRateLimiterStateManager {
	return &MemoryRateLimiterStateManager{mu: newRateLimiterSyncer()}
}

// RateLimiterOption allows customizing RateLimiter creation.
type RateLimiterOption func(*RateLimiter) error

// RateLimiter current limitor
type RateLimiter struct {
	RateLimiterArgs

	ctx         context.Context
	stateCtx    context.Context
	stateCancel context.CancelFunc

	stateManager  RateLimiterStateManager
	stopChan      chan struct{}
	initialTokens int
}

// NewRateLimiter create new Throttle
//
// 90x faster than `rate.NewLimiter`
func NewRateLimiter(
	ctx context.Context,
	args RateLimiterArgs,
	opts ...RateLimiterOption,
) (ratelimiter *RateLimiter, err error) {
	if ctx == nil {
		return nil, errors.Errorf("ctx should not be nil")
	}
	if args.NPerSec <= 0 {
		return nil, errors.Errorf("npersec should greater than 0")
	}
	if args.Max < args.NPerSec {
		return nil, errors.Errorf("max should greater than npersec")
	}

	stateCtx, stateCancel := context.WithCancel(ctx)
	ratelimiter = &RateLimiter{
		RateLimiterArgs: args,
		ctx:             ctx,
		stateCtx:        stateCtx,
		stateCancel:     stateCancel,
		stateManager:    NewMemoryRateLimiterStateManager(),
		stopChan:        make(chan struct{}),
		initialTokens:   args.NPerSec,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		if err = opt(ratelimiter); err != nil {
			return nil, errors.Wrap(err, "apply ratelimiter option")
		}
	}

	shouldRefill, err := ratelimiter.setupStateManager()
	if err != nil {
		return nil, errors.Wrap(err, "setup ratelimiter state manager")
	}
	if shouldRefill {
		go ratelimiter.runWithCtx(ctx)
	}
	return ratelimiter, nil
}

// Allow check whether is allowed
func (t *RateLimiter) Allow() bool {
	return t.AllowN(1)
}

// Len return current tokens length
func (t *RateLimiter) Len() int {
	tokens, err := t.stateManager.AvailableTokens(t.stateCtx)
	if err != nil {
		log.Shared.Warn("ratelimiter len", zap.Error(err))
		return 0
	}
	return tokens
}

// ExportState returns a snapshot of the current limiter state.
func (t *RateLimiter) ExportState() RateLimiterState {
	tokens, err := t.stateManager.AvailableTokens(t.stateCtx)
	if err != nil {
		log.Shared.Warn("ratelimiter export state", zap.Error(err))
		tokens = 0
	}
	return RateLimiterState{
		Args:            t.RateLimiterArgs,
		AvailableTokens: tokens,
	}
}

// RestoreState overwrites the current limiter state with the provided snapshot.
func (t *RateLimiter) RestoreState(state RateLimiterState) error {
	if state.Args.NPerSec != t.NPerSec || state.Args.Max != t.Max {
		return errors.Errorf("state args mismatch: want %+v got %+v", t.RateLimiterArgs, state.Args)
	}

	return errors.Wrap(t.setAvailableTokens(state.AvailableTokens), "set available tokens")
}

// Clone creates a new limiter with the same configuration and token availability.
func (t *RateLimiter) Clone(ctx context.Context) (*RateLimiter, error) {
	state := t.ExportState()
	clone, err := NewRateLimiter(ctx, state.Args, WithAvailableTokens(state.AvailableTokens))
	if err != nil {
		return nil, errors.Wrap(err, "clone ratelimiter")
	}

	return clone, nil
}

// AllowN check whether is allowed,
// default ratelimiter only allow 1 request per second at least,
// so if you want to allow less than 1 request per second,
// you should use `AllowN` to consume more tokens each time.
func (t *RateLimiter) AllowN(n int) bool {
	if n <= 0 {
		return true
	}

	ok, err := t.stateManager.TryConsume(t.stateCtx, n)
	if err != nil {
		log.Shared.Warn("ratelimiter allow", zap.Int("tokens", n), zap.Error(err))
		return false
	}

	return ok
}

// WithAvailableTokens sets the initial token count for a newly created limiter.
func WithAvailableTokens(tokens int) RateLimiterOption {
	return func(t *RateLimiter) error {
		if tokens < 0 {
			return errors.Errorf("available tokens should not be negative: %d", tokens)
		}
		if tokens > t.Max {
			return errors.Errorf("available tokens %d exceeds max %d", tokens, t.Max)
		}

		t.initialTokens = tokens
		return nil
	}
}

// WithRateLimiterState restores the provided state on a new limiter instance.
func WithRateLimiterState(state RateLimiterState) RateLimiterOption {
	return func(t *RateLimiter) error {
		if state.Args.NPerSec != t.NPerSec || state.Args.Max != t.Max {
			return errors.Errorf("state args mismatch: want %+v got %+v", t.RateLimiterArgs, state.Args)
		}
		if state.AvailableTokens < 0 {
			return errors.Errorf("available tokens should not be negative: %d", state.AvailableTokens)
		}
		if state.AvailableTokens > t.Max {
			return errors.Errorf("available tokens %d exceeds max %d", state.AvailableTokens, t.Max)
		}

		t.initialTokens = state.AvailableTokens
		return nil
	}
}

// WithRateLimiterStateManager overrides the default in-memory state manager.
func WithRateLimiterStateManager(manager RateLimiterStateManager) RateLimiterOption {
	return func(t *RateLimiter) error {
		if manager == nil {
			return errors.Errorf("state manager should not be nil")
		}

		t.stateManager = manager
		return nil
	}
}

// runWithCtx starts the refill loop driven by a real ticker. The ctx argument
// stops the loop when canceled.
func (t *RateLimiter) runWithCtx(ctx context.Context) {
	defer log.Shared.Debug("throttle exit")

	interval := 100 * time.Millisecond
	if t.NPerSec > 10000 {
		interval = 10 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	t.refillLoop(ctx, ticker.C, time.Now)
}

// refillLoop adds tokens each time ticks fires until ctx is canceled or the
// limiter is closed. The now argument supplies the monotonic clock.
//
// The refill amount is proportional to the time elapsed since the previous
// refill rather than to the number of ticks received: time.Ticker drops ticks
// for a slow receiver, so counting ticks under-delivers the configured rate
// whenever this goroutine is delayed. Pending fractional tokens are carried
// over and capped at Max so a long stall cannot accumulate unbounded credit.
func (t *RateLimiter) refillLoop(ctx context.Context, ticks <-chan time.Time, now func() time.Time) {
	last := now()
	var pending float64
	for {
		select {
		case <-ticks:
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		}

		current := now()
		elapsed := current.Sub(last)
		if elapsed <= 0 {
			continue
		}
		last = current

		pending += elapsed.Seconds() * float64(t.NPerSec)
		if pending > float64(t.Max) {
			pending = float64(t.Max)
		}

		// A tiny epsilon absorbs float rounding so that, for example, ten
		// 100ms refills at 3/s yield exactly 3 tokens instead of 2.
		tokensToAdd := int(pending + 1e-9)
		if tokensToAdd <= 0 {
			continue
		}
		pending -= float64(tokensToAdd)
		if pending < 0 {
			pending = 0
		}

		if _, err := t.stateManager.AddTokens(ctx, tokensToAdd); err != nil {
			log.Shared.Warn("ratelimiter add tokens", zap.Int("tokens", tokensToAdd), zap.Error(err))
		}
	}
}

// Close stop throttle
func (t *RateLimiter) Close() {
	t.stateCancel()
	close(t.stopChan)
}

// setAvailableTokens overwrites the limiter's available token count with tokens through the state manager,
// using the limiter's state context. It returns an error if tokens is negative or greater than Max, or a wrapped
// state manager error, for example when the limiter has been closed and its state context is canceled.
func (t *RateLimiter) setAvailableTokens(tokens int) error {
	if tokens < 0 {
		return errors.Errorf("available tokens should not be negative: %d", tokens)
	}
	if tokens > t.Max {
		return errors.Errorf("available tokens %d exceeds max %d", tokens, t.Max)
	}

	return errors.Wrap(t.stateManager.SetAvailableTokens(t.stateCtx, tokens), "set tokens via state manager")
}

// setupStateManager calls the state manager's Setup with the limiter's state context, args, and initial token
// count. It returns true when this limiter should run the refill loop (the manager reports it was freshly
// initialized by this call), or false and a wrapped error when Setup fails, for example because the manager
// was already initialized with different args or the context is canceled.
func (t *RateLimiter) setupStateManager() (bool, error) {
	shouldRefill, err := t.stateManager.Setup(t.stateCtx, t.RateLimiterArgs, t.initialTokens)
	if err != nil {
		return false, errors.Wrap(err, "init state manager")
	}

	return shouldRefill, nil
}

// Setup implements synchronization for the in-memory state manager.
func (m *MemoryRateLimiterStateManager) Setup(
	ctx context.Context,
	args RateLimiterArgs,
	initialTokens int,
) (bool, error) {
	select {
	case <-ctx.Done():
		return false, errors.Wrap(ctx.Err(), "context done")
	default:
	}

	if err := ensureTokenRange(args, initialTokens); err != nil {
		return false, err
	}

	m.mu.lock()
	defer m.mu.unlock()

	if !m.initialized {
		m.state.Args = args
		m.state.AvailableTokens = initialTokens
		m.initialized = true
		return true, nil
	}

	if m.state.Args != args {
		return false, errors.Errorf("state manager already initialized with args %+v", m.state.Args)
	}

	return false, nil
}

// TryConsume deducts tokens if enough are available.
func (m *MemoryRateLimiterStateManager) TryConsume(ctx context.Context, n int) (bool, error) {
	if n <= 0 {
		return true, nil
	}

	select {
	case <-ctx.Done():
		return false, errors.Wrap(ctx.Err(), "context done")
	default:
	}

	m.mu.lock()
	defer m.mu.unlock()

	if !m.initialized {
		return false, errors.Errorf("state manager not initialized")
	}

	if n > m.state.Args.Max {
		return false, nil
	}

	if m.state.AvailableTokens < n {
		return false, nil
	}

	m.state.AvailableTokens -= n
	return true, nil
}

// AddTokens increases the available tokens without exceeding Max.
func (m *MemoryRateLimiterStateManager) AddTokens(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}

	select {
	case <-ctx.Done():
		return 0, errors.Wrap(ctx.Err(), "context done")
	default:
	}

	m.mu.lock()
	defer m.mu.unlock()

	if !m.initialized {
		return 0, errors.Errorf("state manager not initialized")
	}

	capacity := m.state.Args.Max - m.state.AvailableTokens
	if capacity <= 0 {
		return 0, nil
	}

	if n > capacity {
		n = capacity
	}

	m.state.AvailableTokens += n
	return n, nil
}

// AvailableTokens returns the current token count.
func (m *MemoryRateLimiterStateManager) AvailableTokens(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
		return 0, errors.Wrap(ctx.Err(), "context done")
	default:
	}

	m.mu.lock()
	defer m.mu.unlock()

	if !m.initialized {
		return 0, errors.Errorf("state manager not initialized")
	}

	return m.state.AvailableTokens, nil
}

// SetAvailableTokens overwrites the token count, respecting Max bounds.
func (m *MemoryRateLimiterStateManager) SetAvailableTokens(ctx context.Context, tokens int) error {
	select {
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "context done")
	default:
	}

	m.mu.lock()
	defer m.mu.unlock()

	if !m.initialized {
		return errors.Errorf("state manager not initialized")
	}

	if err := ensureTokenRange(m.state.Args, tokens); err != nil {
		return err
	}

	m.state.AvailableTokens = tokens
	return nil
}

// ensureTokenRange validates that tokens lies within the inclusive range [0, args.Max]. It returns nil for a
// valid count, and an error when tokens is negative or exceeds args.Max.
func ensureTokenRange(args RateLimiterArgs, tokens int) error {
	if tokens < 0 {
		return errors.Errorf("available tokens should not be negative: %d", tokens)
	}
	if tokens > args.Max {
		return errors.Errorf("available tokens %d exceeds max %d", tokens, args.Max)
	}

	return nil
}
