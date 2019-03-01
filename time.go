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

func SetupClock(refreshInterval time.Duration) {
	if Clock == nil {
		Clock = NewClock(refreshInterval)
	} else {
		Clock.SetupInterval(refreshInterval)
	}
}

type ClockType struct {
	*sync.RWMutex
	interval           time.Duration
	now                time.Time
	timeStrRFC3339Nano string
	isStop             bool
}

func NewClock(refreshInterval time.Duration) *ClockType {
	c := &ClockType{
		RWMutex:  &sync.RWMutex{},
		interval: refreshInterval,
	}
	go c.runRefresh()

	return c
}

func (c *ClockType) Stop() {
	c.Lock()
	defer c.Unlock()

	c.isStop = true
}

func (c *ClockType) Run() {
	c.Lock()
	defer c.Unlock()

	c.isStop = false
	go c.runRefresh()
}

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
		c.now = time.Now().UTC()
		c.timeStrRFC3339Nano = c.now.Format(time.RFC3339Nano)
		interval = c.interval
		c.Unlock()

		time.Sleep(interval)
	}
}

func (c *ClockType) GetUTCNow() time.Time {
	c.RLock()
	defer c.RUnlock()
	return c.now
}

func (c *ClockType) GetTimeInRFC3339Nano() string {
	c.RLock()
	defer c.RUnlock()
	return c.timeStrRFC3339Nano
}
