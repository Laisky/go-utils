package utils

import (
	"context"
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

// NewThrottle create new Throttle
func NewThrottle(cfg *ThrottleCfg) *Throttle {
	t := &Throttle{
		ThrottleCfg: cfg,
		token:       struct{}{},
		stopChan:    make(chan struct{}),
	}
	t.tokensChan = make(chan struct{}, t.Max)
	return t
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

// Run (Deprecated) start throttle
func (t *Throttle) Run() {
	go func() {
		defer Logger.Info("throttle exit")
		for {
			for i := 0; i < t.NPerSec; i++ {
				select {
				case <-t.stopChan:
					return
				case t.tokensChan <- t.token:
				default:
				}
			}

			time.Sleep(1 * time.Second)
		}
	}()
}

// RunWithCtx start throttle with context
func (t *Throttle) RunWithCtx(ctx context.Context) {
	go func() {
		defer Logger.Info("throttle exit")
		for {
			for i := 0; i < t.NPerSec; i++ {
				select {
				case <-ctx.Done():
					return
				case <-t.stopChan:
					return
				case t.tokensChan <- t.token:
				default:
				}
			}

			time.Sleep(1 * time.Second)
		}
	}()
}

// Stop stop throttle
func (t *Throttle) Stop() {
	t.stopChan <- struct{}{}
}
