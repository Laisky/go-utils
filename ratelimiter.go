package utils

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v5/log"
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

// RateLimiterOption allows customizing RateLimiter creation.
type RateLimiterOption func(*RateLimiter) error

// RateLimiter current limitor
type RateLimiter struct {
	RateLimiterArgs

	token      struct{}
	tokensChan chan struct{}
	stopChan   chan struct{}
}

// NewRateLimiter create new Throttle
//
// 90x faster than `rate.NewLimiter`
func NewRateLimiter(ctx context.Context, args RateLimiterArgs, opts ...RateLimiterOption) (ratelimiter *RateLimiter, err error) {
	if args.NPerSec <= 0 {
		return nil, errors.Errorf("npersec should greater than 0")
	}
	if args.Max < args.NPerSec {
		return nil, errors.Errorf("max should greater than npersec")
	}

	ratelimiter = &RateLimiter{
		RateLimiterArgs: args,
		token:           struct{}{},
		stopChan:        make(chan struct{}),
	}
	ratelimiter.tokensChan = make(chan struct{}, ratelimiter.Max)

	for i := 0; i < ratelimiter.NPerSec; i++ {
		ratelimiter.tokensChan <- ratelimiter.token
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		if err = opt(ratelimiter); err != nil {
			return nil, errors.Wrap(err, "apply ratelimiter option")
		}
	}

	go ratelimiter.runWithCtx(ctx)
	return ratelimiter, nil
}

// Allow check whether is allowed
func (t *RateLimiter) Allow() bool {
	select {
	case <-t.tokensChan:
		return true
	default:
		return false
	}
}

// Len return current tokens length
func (t *RateLimiter) Len() int {
	return len(t.tokensChan)
}

// ExportState returns a snapshot of the current limiter state.
func (t *RateLimiter) ExportState() RateLimiterState {
	return RateLimiterState{
		Args:            t.RateLimiterArgs,
		AvailableTokens: len(t.tokensChan),
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
	var cost int
	for i := 0; i < n; i++ {
		if !t.Allow() {
		RESTORE_LOOP:
			for j := 0; j < cost; j++ {
				select {
				case t.tokensChan <- t.token:
				default:
					break RESTORE_LOOP
				}
			}

			return false
		}

		cost++
	}

	return true
}

// WithAvailableTokens sets the initial token count for a newly created limiter.
func WithAvailableTokens(tokens int) RateLimiterOption {
	return func(t *RateLimiter) error {
		return errors.Wrap(t.setAvailableTokens(tokens), "apply available tokens option")
	}
}

// WithRateLimiterState restores the provided state on a new limiter instance.
func WithRateLimiterState(state RateLimiterState) RateLimiterOption {
	return func(t *RateLimiter) error {
		if state.Args.NPerSec != t.NPerSec || state.Args.Max != t.Max {
			return errors.Errorf("state args mismatch: want %+v got %+v", t.RateLimiterArgs, state.Args)
		}

		return errors.Wrap(t.setAvailableTokens(state.AvailableTokens), "apply ratelimiter state option")
	}
}

// runWithCtx start throttle with context
func (t *RateLimiter) runWithCtx(ctx context.Context) {
	defer log.Shared.Debug("throttle exit")

	var (
		nPerBatch float64
		interval  time.Duration
	)
	switch {
	case t.NPerSec <= 10:
		nPerBatch = float64(t.NPerSec)
		interval = time.Second
	case t.NPerSec <= 10000:
		nPerBatch = float64(t.NPerSec) / 10
		interval = 100 * time.Millisecond
	default:
		nPerBatch = float64(t.NPerSec) / 100
		interval = 10 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
TOKEN_LOOP:
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		}

		for i := 0; i < int(nPerBatch); i++ {
			select {
			case t.tokensChan <- t.token:
			default:
				continue TOKEN_LOOP
			}
		}
	}
}

// Close stop throttle
func (t *RateLimiter) Close() {
	close(t.stopChan)
}

func (t *RateLimiter) setAvailableTokens(tokens int) error {
	if tokens < 0 {
		return errors.Errorf("available tokens should not be negative: %d", tokens)
	}
	if tokens > t.Max {
		return errors.Errorf("available tokens %d exceeds max %d", tokens, t.Max)
	}

	for {
		current := len(t.tokensChan)
		if current == tokens {
			return nil
		}

		switch {
		case current < tokens:
			select {
			case t.tokensChan <- t.token:
				continue
			default:
				// Channel already full; re-check in the next iteration.
			}
		default: // current > tokens
			select {
			case <-t.tokensChan:
				continue
			default:
				// Channel already drained to target; re-check.
			}
		}

		if len(t.tokensChan) == tokens {
			return nil
		}
	}
}
