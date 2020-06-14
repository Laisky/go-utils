package utils

import (
	"context"
	"fmt"
	"time"
)

// ThrottleCfg Throttle's configuration
type ThrottleCfg struct {
	Max, NPerSec int
}

// Throttle current limitor
type Throttle struct {
	*ThrottleCfg
	token      struct{}
	tokensChan chan struct{}
	stopChan   chan struct{}
}

// NewThrottleWithCtx create new Throttle
func NewThrottleWithCtx(ctx context.Context, cfg *ThrottleCfg) (t *Throttle, err error) {
	if cfg.NPerSec <= 0 {
		return nil, fmt.Errorf("NPerSec should greater than 0")
	}
	if cfg.Max < cfg.NPerSec {
		return nil, fmt.Errorf("Max should greater than NPerSec")
	}

	t = &Throttle{
		ThrottleCfg: cfg,
		token:       struct{}{},
		stopChan:    make(chan struct{}),
	}
	t.tokensChan = make(chan struct{}, t.Max)
	t.runWithCtx(ctx)
	return t, nil
}

// Allow check whether is allowed
func (t *Throttle) Allow() bool {
	select {
	case <-t.tokensChan:
		return true
	default:
		return false
	}
}

// runWithCtx start throttle with context
func (t *Throttle) runWithCtx(ctx context.Context) {
	go func() {
		defer Logger.Info("throttle exit")

		for i := 0; i < t.NPerSec; i++ {
			t.tokensChan <- t.token
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
	TOKEN_LOOP:
		for {
			select {
			case <-ticker.C:
				for i := 0; i < t.NPerSec; i++ {
					select {
					case <-ctx.Done():
						return
					case <-t.stopChan:
						return
					case t.tokensChan <- t.token:
					default:
						continue TOKEN_LOOP
					}
				}
			case <-ctx.Done():
				return
			case <-t.stopChan:
				return
			}
		}
	}()
}

// Close stop throttle
func (t *Throttle) Close() {
	close(t.stopChan)
}

// Stop stop throttle
//
// Deprecated: replaced by Close
func (t *Throttle) Stop() {
	t.Close()
}
