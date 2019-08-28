package utils

import (
	"sync"
	"time"
)

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

// ParseTs2String can parse unix timestamp(int64) to string
func ParseTs2String(ts int64, layout string) string {
	return ParseTs2Time(ts).Format(layout)
}

// ParseTs2Time can parse unix timestamp(int64) to time.Time
func ParseTs2Time(ts int64) time.Time {
	return time.Unix(ts, 0).UTC()
}

// ---------------------------------------
// Clock
// ---------------------------------------

const defaultClockInterval = 100 * time.Millisecond

// Clock high performance time utils
var Clock = NewClock(defaultClockInterval)

// SetupClock setup internal Clock with step
func SetupClock(refreshInterval time.Duration) {
	if Clock == nil {
		Clock = NewClock(refreshInterval)
	} else {
		Clock.SetupInterval(refreshInterval)
	}
}

// ClockType high performance clock with lazy refreshing
type ClockType struct {
	sync.RWMutex
	interval time.Duration
	now      time.Time
	// timeStrRFC3339Nano string
	isStop bool
}

// NewClock create new Clock
func NewClock(refreshInterval time.Duration) *ClockType {
	c := &ClockType{
		interval: refreshInterval,
	}
	go c.runRefresh()

	return c
}

// Stop stop Clock update
func (c *ClockType) Stop() {
	c.Lock()
	defer c.Unlock()

	c.isStop = true
}

// Run start Clock
func (c *ClockType) Run() {
	c.Lock()
	defer c.Unlock()

	c.isStop = false
	go c.runRefresh()
}

// SetupInterval setup update interval
func (c *ClockType) SetupInterval(interval time.Duration) {
	c.Lock()
	defer c.Unlock()

	c.interval = interval
}

func (c *ClockType) runRefresh() {
	var interval time.Duration
	for {
		c.Lock()
		if c.isStop {
			return
		}
		c.now = UTCNow()
		interval = c.interval
		c.Unlock()

		time.Sleep(interval)
	}
}

// GetUTCNow return Clock current time.Time
func (c *ClockType) GetUTCNow() time.Time {
	c.RLock()
	defer c.RUnlock()
	return c.now
}

// GetTimeInRFC3339Nano return Clock current time in string
func (c *ClockType) GetTimeInRFC3339Nano() string {
	return c.GetUTCNow().Format(time.RFC3339Nano)
}
