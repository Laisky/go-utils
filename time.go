package utils

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
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

// SleepWithContext sleep duration with context, if context is done, return
func SleepWithContext(ctx context.Context, duration time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	<-ctx.Done()
}

// UTCNow get current time in utc
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

// TimeEqual compare two time with difference,
// return true if time difference less than difference
func TimeEqual(ts1, ts2 time.Time, difference time.Duration) bool {
	sub := ts1.Sub(ts2)
	return sub < difference && sub > -difference
}

// ParseHexNano2UTC parse hex contains nano to UTC time
func ParseHexNano2UTC(ts string) (t time.Time, err error) {
	var ut int64
	if ut, err = strconv.ParseInt(ts, BaseHex, BitSize64); err != nil {
		return
	}

	return ParseUnixNano2UTC(ut), nil
}

// ParseTimeWithTruncate parse time with truncate
func ParseTimeWithTruncate(layout, value string, precision time.Duration) (t time.Time, err error) {
	t, err = time.Parse(layout, value)
	if err != nil {
		return t, errors.Wrap(err, "parse time")
	}

	return t.Truncate(precision), nil
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
	Close()
	runRefresh(ctx context.Context)
	GetUTCNow() time.Time
	GetDate() (time.Time, error)
	GetTimeInRFC3339Nano() string
	SetInterval(interval time.Duration)
	GetTimeInHex() string
	GetNanoTimeInHex() string
	Interval() time.Duration
}

const defaultClockInterval = 10 * time.Millisecond

// SetInternalClock set internal Clock with refresh interval
func SetInternalClock(interval time.Duration) {
	if interval < time.Microsecond {
		panic("interval must greater than 1us")
	}

	if Clock == nil {
		Clock = NewClock(context.Background(), interval)
	} else {
		Clock.SetInterval(interval)
	}
}

var (
	// Clock high performance time utils, replace Clock1
	Clock = NewClock(context.Background(), defaultClockInterval)
)

// ClockT high performance ClockT with lazy refreshing
type ClockT struct {
	sync.RWMutex
	stopChan chan struct{}

	intervalNanos int64
	now           int64
}

// NewClock creates a ClockT and starts its refresh loop.
// It takes ctx to control the goroutine lifecycle and refreshInterval as the update period, and returns a *ClockT.
func NewClock(ctx context.Context, refreshInterval time.Duration) *ClockT {
	if refreshInterval < time.Microsecond {
		panic("interval must greater than 1us")
	}

	c := &ClockT{
		intervalNanos: int64(refreshInterval),
		now:           UTCNow().UnixNano(),
		stopChan:      make(chan struct{}),
	}
	go c.runRefresh(ctx)

	return c
}

// Close stop Clock update
func (c *ClockT) Close() {
	c.stopChan <- struct{}{}
}

func (c *ClockT) runRefresh(ctx context.Context) {
	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Duration(atomic.LoadInt64(&c.intervalNanos)))
		}

		atomic.StoreInt64(&c.now, time.Now().UnixNano())
	}
}

// GetUTCNow return Clock current time.Time
func (c *ClockT) GetUTCNow() time.Time {
	return ParseUnixNano2UTC(atomic.LoadInt64(&c.now))
}

// GetDate return "yyyy-mm-dd"
func (c *ClockT) GetDate() (time.Time, error) {
	return c.GetUTCNow().Truncate(24 * time.Hour), nil
}

// GetTimeInRFC3339Nano return Clock current time in string
func (c *ClockT) GetTimeInRFC3339Nano() string {
	return c.GetUTCNow().Format(time.RFC3339Nano)
}

// SetInterval updates the refresh interval used by ClockT.
// It takes interval as the update period and does not return a value.
func (c *ClockT) SetInterval(interval time.Duration) {
	if interval < time.Microsecond {
		panic("interval must greater than 1us")
	}

	atomic.StoreInt64(&c.intervalNanos, int64(interval))
}

// GetTimeInHex return current time in hex
func (c *ClockT) GetTimeInHex() string {
	return strconv.FormatInt(atomic.LoadInt64(&c.now)/Nano2Sec, BaseHex)
}

// GetNanoTimeInHex return current time with nano in hex
func (c *ClockT) GetNanoTimeInHex() string {
	return strconv.FormatInt(atomic.LoadInt64(&c.now), BaseHex)
}

// Interval get current interval
func (c *ClockT) Interval() time.Duration {
	return time.Duration(atomic.LoadInt64(&c.intervalNanos))
}

var (
	// TimeZoneUTC timezone UTC
	TimeZoneUTC = time.UTC
	// TimeZoneShanghai timezone Shanghai
	// TimeZoneShanghai = time.FixedZone("Asia/Shanghai", 8*3600)
	TimeZoneShanghai *time.Location
)

func init() {
	var err error
	TimeZoneShanghai, err = time.LoadLocation("Asia/Shanghai")
	PanicIfErr(err)
}
