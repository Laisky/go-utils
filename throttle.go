package utils

import (
	"time"
)

type ThrottleCfg struct {
	Max, NPerSec int
}

type Throttle struct {
	*ThrottleCfg
	token      struct{}
	tokensChan chan struct{}
	isStop     bool
}

func NewThrottle(cfg *ThrottleCfg) *Throttle {
	t := &Throttle{
		ThrottleCfg: cfg,
		token:       struct{}{},
		isStop:      false,
	}
	t.tokensChan = make(chan struct{}, t.Max)
	return t
}

func (t *Throttle) Allow() bool {
	select {
	case <-t.tokensChan:
		return true
	default:
		return false
	}
}

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

func (t *Throttle) Stop() {
	t.isStop = true
}
