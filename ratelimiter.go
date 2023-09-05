package utils

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v4/log"
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
func NewRateLimiter(ctx context.Context, args RateLimiterArgs) (ratelimiter *RateLimiter, err error) {
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

// AllowN check whether is allowed,
// default ratelimiter only allow 1 request per second at least,
// so if you want to allow less than 1 request per second,
// you should use `AllowN` to consume more tokens each time.
func (t *RateLimiter) AllowN(n int) bool {
	var cost int
	for i := 0; i < n; i++ {
		cost++
		if !t.Allow() {
		RESTORE_LOOP:
			for j := 0; j < cost-1; j++ {
				select {
				case t.tokensChan <- t.token:
				default:
					break RESTORE_LOOP
				}
			}

			return false
		}
	}

	return true
}

// runWithCtx start throttle with context
func (t *RateLimiter) runWithCtx(ctx context.Context) {
	defer log.Shared.Debug("throttle exit")

	var nBatch float64 = 10
	nPerBatch := float64(t.NPerSec) / nBatch
	interval := time.Duration(1/nBatch*1000) * time.Millisecond

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

		for i := float64(0); i < nPerBatch; i++ {
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
