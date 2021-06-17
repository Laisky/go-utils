package utils

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// TimeFormatDate "2006-01-02"
	TimeFormatDate = "2006-01-02"
	// Nano2Sec 1e9
	Nano2Sec = 1e9
	// BitSize64 64
	BitSize64 = 64
	// BaseHex 16
	BaseHex = 16
)

// UTCNow 获取当前 UTC 时间
func UTCNow() time.Time {
	return time.Now().UTC()
}

// ParseUnix2String can parse unix timestamp(int64) to string
func ParseUnix2String(ts int64, layout string) string {
	return ParseUnix2UTC(ts).Format(layout)
}

// ParseUnix2UTC convert unix to UTC time
func ParseUnix2UTC(ts int64) time.Time {
	return time.Unix(ts, 0).UTC()
}

var (
	// ParseTs2UTC can parse unix timestamp(int64) to time.Time
	ParseTs2UTC = ParseUnix2UTC
	// ParseTs2String can parse unix timestamp(int64) to string
	ParseTs2String = ParseUnix2String
)

// ParseUnixNano2UTC convert unixnano to UTC time
func ParseUnixNano2UTC(ts int64) time.Time {
	return time.Unix(ts/Nano2Sec, ts%Nano2Sec).UTC()
}

// ParseHex2UTC parse hex to UTC time
func ParseHex2UTC(ts string) (t time.Time, err error) {
	var ut int64
	if ut, err = strconv.ParseInt(ts, BaseHex, BitSize64); err != nil {
		return
	}

	return ParseUnix2UTC(ut), nil
}

// ParseHexNano2UTC parse hex contains nano to UTC time
func ParseHexNano2UTC(ts string) (t time.Time, err error) {
	var ut int64
	if ut, err = strconv.ParseInt(ts, BaseHex, BitSize64); err != nil {
		return
	}

	return ParseUnixNano2UTC(ut), nil
}

var ( // compatable to old version
	// ParseTs2Time can parse unix timestamp(int64) to time.Time
	ParseTs2Time = ParseTs2UTC
	// UnixNano2UTC convert unixnano to UTC time
	UnixNano2UTC = ParseUnixNano2UTC
)

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

const defaultClockInterval = 10 * time.Millisecond

// SetupClock setup internal Clock with step
func SetupClock(refreshInterval time.Duration) {
	if Clock == nil {
		Clock = NewClock(context.Background(), refreshInterval)
	} else {
		Clock.SetupInterval(refreshInterval)
	}
}

var (
	// Clock high performance time utils, replace Clock1
	Clock = NewClock(context.Background(), defaultClockInterval)

	// compatable to old version

	// Clock2 high performance time utils
	Clock2 = Clock
	// NewClock2 create new Clock
	NewClock2 = NewClock
)

// Clock2Type high performance clock with lazy refreshing
type Clock2Type ClockType

// ClockType high performance clock with lazy refreshing
type ClockType struct {
	sync.RWMutex
	stopChan chan struct{}

	interval time.Duration
	now      int64
}

// NewClock create new Clock
func NewClock(ctx context.Context, refreshInterval time.Duration) *ClockType {
	c := &ClockType{
		interval: refreshInterval,
		now:      UTCNow().UnixNano(),
		stopChan: make(chan struct{}),
	}
	go c.runRefresh(ctx)

	return c
}

// Close stop Clock update
func (c *ClockType) Close() {
	c.stopChan <- struct{}{}
}

func (c *ClockType) runRefresh(ctx context.Context) {
	var interval time.Duration
	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
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

// GetUTCNow return Clock current time.Time
func (c *ClockType) GetUTCNow() time.Time {
	return ParseUnixNano2UTC(atomic.LoadInt64(&c.now))
}

// GetDate return "yyyy-mm-dd"
func (c *ClockType) GetDate() (time.Time, error) {
	return time.Parse(TimeFormatDate, c.GetUTCNow().Format(TimeFormatDate))
}

// GetTimeInRFC3339Nano return Clock current time in string
func (c *ClockType) GetTimeInRFC3339Nano() string {
	return c.GetUTCNow().Format(time.RFC3339Nano)
}

// SetupInterval setup update interval
//
// Deprecated: use SetInterval instead
func (c *ClockType) SetupInterval(interval time.Duration) {
	c.SetInterval(interval)
}

// SetInterval setup update interval
func (c *ClockType) SetInterval(interval time.Duration) {
	c.Lock()
	defer c.Unlock()

	c.interval = interval
}

// GetTimeInHex return current time in hex
func (c *ClockType) GetTimeInHex() string {
	return strconv.FormatInt(c.GetUTCNow().Unix(), BaseHex)
}

// GetNanoTimeInHex return current time with nano in hex
func (c *ClockType) GetNanoTimeInHex() string {
	return strconv.FormatInt(c.GetUTCNow().UnixNano(), BaseHex)
}
