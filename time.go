package utils

import (
	"context"
	"sync"
	"sync/atomic"
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

// ClockItf high performance lazy clock
type ClockItf interface {
	GetTimeInRFC3339Nano() string
	GetUTCNow() time.Time
	SetupInterval(time.Duration)
	Close()
}

const defaultClockInterval = 100 * time.Millisecond

// Clock (Deprecated) high performance time utils
var Clock = NewClock2(context.Background(), defaultClockInterval)

// SetupClock setup internal Clock with step
func SetupClock(refreshInterval time.Duration) {
	if Clock == nil {
		Clock = NewClock2(context.Background(), refreshInterval)
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
		now:      UTCNow(),
	}
	go c.runRefresh()

	return c
}

func (c *ClockType) Close() {
	c.Stop()
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
func (c *ClockType) GetUTCNow() (t time.Time) {
	c.RLock()
	t = c.now
	c.RUnlock()
	return t
}

// GetTimeInRFC3339Nano return Clock current time in string
func (c *ClockType) GetTimeInRFC3339Nano() string {
	return c.GetUTCNow().Format(time.RFC3339Nano)
}

// Clock2 high performance time utils, replace Clock1
var Clock2 = NewClock2(context.Background(), defaultClockInterval)

// Clock2Type high performance clock with lazy refreshing
type Clock2Type struct {
	sync.RWMutex
	ctx    context.Context
	cancel func()

	interval time.Duration
	now      int64
}

// NewClock2 create new Clock2
func NewClock2(ctx context.Context, refreshInterval time.Duration) *Clock2Type {
	c := &Clock2Type{
		interval: refreshInterval,
		now:      UTCNow().UnixNano(),
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	go c.runRefresh(c.ctx)

	return c
}

// Close stop Clock2 update
func (c *Clock2Type) Close() {
	c.cancel()
}

func (c *Clock2Type) runRefresh(ctx context.Context) {
	var interval time.Duration
	for {
		select {
		case <-ctx.Done():
			Logger.Info("clock refresher exit")
			return
		default:
			c.RLock()
			interval = c.interval
			c.RUnlock()
			time.Sleep(interval)
		}

		atomic.StoreInt64(&c.now, time.Now().UnixNano())
	}
}

// GetUTCNow return Clock2 current time.Time
func (c *Clock2Type) GetUTCNow() (t time.Time) {
	ts := atomic.LoadInt64(&c.now)
	return time.Unix(ts/1e9, ts%1e9).UTC()
}

// GetTimeInRFC3339Nano return Clock2 current time in string
func (c *Clock2Type) GetTimeInRFC3339Nano() string {
	return c.GetUTCNow().Format(time.RFC3339Nano)
}

// SetupInterval setup update interval
func (c *Clock2Type) SetupInterval(interval time.Duration) {
	c.Lock()
	defer c.Unlock()

	c.interval = interval
}
