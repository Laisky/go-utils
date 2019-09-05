package utils

import (
	"sync/atomic"
	"time"
)

// Mutex mutex that support unblocking lock
type Mutex struct {
	l uint32
}

// NewMutex create new mutex
func NewMutex() *Mutex {
	return &Mutex{
		l: 0,
	}
}

// TryLock return true if succeed locked
func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.l, 0, 1)
}

// IsLocked return true if is locked
func (m *Mutex) IsLocked() bool {
	return atomic.LoadUint32(&m.l) == 1
}

// TryRelease return true if succeed release
func (m *Mutex) TryRelease() bool {
	return atomic.CompareAndSwapUint32(&m.l, 1, 0)
}

// ForceRelease force release lock
func (m *Mutex) ForceRelease() {
	atomic.StoreUint32(&m.l, 0)
}

// SpinLock block until succee acquired lock
func (m *Mutex) SpinLock(step, timeout time.Duration) {
	start := Clock.GetUTCNow()
	for {
		if m.TryLock() || Clock.GetUTCNow().Sub(start) > timeout {
			return
		}
		time.Sleep(step)
	}
}
