package utils

import (
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
	isStop     bool
}

// NewThrottle create new Throttle
func NewThrottle(cfg *ThrottleCfg) *Throttle {
	t := &Throttle{
		ThrottleCfg: cfg,
		token:       struct{}{},
		isStop:      false,
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

// Run start throttle
func (t *Throttle) Run() {
	t.isStop = false
	go func() {
		for {
			for i := 0; i < t.NPerSec; i++ {
				select {
				case t.tokensChan <- t.token:
				default:
				}
			}

			if t.isStop {
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()
}

// Stop stop throttle
func (t *Throttle) Stop() {
	t.isStop = true
}
