package log

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
)

const rotationSuffixLayout = "20060102T150405Z"

type rotationConfig struct {
	path          string
	interval      RotationInterval
	retentionDays int
}

func (o *option) ensureRotationConfig() *rotationConfig {
	if o.rotation == nil {
		o.rotation = &rotationConfig{}
	}

	return o.rotation
}

func (o *option) configureRotation() error {
	if o.rotation == nil {
		return nil
	}

	if o.rotation.path == "" {
		return errors.Errorf("rotation path must not be empty")
	}

	if !o.rotation.interval.isSupported() {
		return errors.Errorf("rotation interval %q is not supported", o.rotation.interval.String())
	}

	if o.rotation.retentionDays < 0 {
		return errors.Errorf("rotation retention days must be >= 0")
	}

	if err := ensureRotationSinkRegistered(); err != nil {
		return err
	}

	query := url.Values{}
	query.Set("path", o.rotation.path)
	query.Set("interval", o.rotation.interval.String())
	query.Set("retention_days", strconv.Itoa(o.rotation.retentionDays))

	u := &url.URL{
		Scheme:   rotationScheme,
		RawQuery: query.Encode(),
	}

	o.OutputPaths = []string{u.String()}
	return nil
}

// RotationInterval represents the supported rotation cadences.
type RotationInterval time.Duration

const (
	// RotationHourly rotates log files at the top of every hour (UTC).
	RotationHourly RotationInterval = RotationInterval(time.Hour)
	// RotationDaily rotates log files every day at 00:00 UTC.
	RotationDaily RotationInterval = RotationInterval(24 * time.Hour)
	// RotationWeekly rotates log files every week at 00:00 UTC on Monday.
	RotationWeekly RotationInterval = RotationInterval(7 * 24 * time.Hour)
)

func (ri RotationInterval) String() string {
	switch time.Duration(ri) {
	case time.Hour:
		return "hourly"
	case 24 * time.Hour:
		return "daily"
	case 7 * 24 * time.Hour:
		return "weekly"
	default:
		return time.Duration(ri).String()
	}
}

func (ri RotationInterval) isSupported() bool {
	switch time.Duration(ri) {
	case time.Hour, 24 * time.Hour, 7 * 24 * time.Hour:
		return true
	default:
		return false
	}
}

func parseRotationInterval(raw string) (RotationInterval, error) {
	switch raw {
	case "hourly":
		return RotationHourly, nil
	case "daily":
		return RotationDaily, nil
	case "weekly":
		return RotationWeekly, nil
	default:
		return 0, errors.Errorf("unsupported rotation interval: %s", raw)
	}
}

var (
	registerRotationSinkOnce sync.Once
	rotationSinkErr          error
)

const rotationScheme = "rotate"

func ensureRotationSinkRegistered() error {
	registerRotationSinkOnce.Do(func() {
		rotationSinkErr = zap.RegisterSink(rotationScheme, createRotationSink)
	})

	if rotationSinkErr != nil {
		return errors.Wrap(rotationSinkErr, "register rotation sink")
	}

	return nil
}

// WithRotation configures the logger to rotate the output file according to interval.
func WithRotation(path string, interval RotationInterval) Option {
	return func(c *option) error {
		if path == "" {
			return errors.Errorf("rotation path must not be empty")
		}
		if !interval.isSupported() {
			return errors.Errorf("rotation interval %q is not supported", interval.String())
		}

		cfg := c.ensureRotationConfig()
		cfg.path = path
		cfg.interval = interval
		return nil
	}
}

// WithRotationRetention configures how many days of rotated logs should be retained.
//
// When days is 0, rotated files are preserved indefinitely.
func WithRotationRetention(days int) Option {
	return func(c *option) error {
		if days < 0 {
			return errors.Errorf("rotation retention days must be >= 0")
		}

		cfg := c.ensureRotationConfig()
		cfg.retentionDays = days
		return nil
	}
}

type rotationWriter struct {
	mu            sync.Mutex
	file          *os.File
	path          string
	interval      RotationInterval
	retentionDays int
	now           func() time.Time
	currentStart  time.Time
	nextRotate    time.Time
}

func newRotationWriter(path string, interval RotationInterval, retentionDays int) (*rotationWriter, error) {
	if path == "" {
		return nil, errors.Errorf("rotation path must not be empty")
	}
	if !interval.isSupported() {
		return nil, errors.Errorf("rotation interval %q is not supported", interval.String())
	}
	if retentionDays < 0 {
		return nil, errors.Errorf("rotation retention days must be >= 0")
	}

	cleaned := filepath.Clean(path)
	writer := &rotationWriter{
		path:          cleaned,
		interval:      interval,
		retentionDays: retentionDays,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}

	return writer, nil
}

func createRotationSink(u *url.URL) (zap.Sink, error) {
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, errors.Wrap(err, "parse rotation sink query")
	}

	path := query.Get("path")
	if path == "" {
		if u.Path != "" {
			path = u.Path
		} else {
			return nil, errors.Errorf("rotation sink missing path")
		}
	}

	intervalRaw := query.Get("interval")
	if intervalRaw == "" {
		return nil, errors.Errorf("rotation sink missing interval")
	}

	interval, err := parseRotationInterval(intervalRaw)
	if err != nil {
		return nil, errors.Wrap(err, "parse rotation interval")
	}

	retentionRaw := query.Get("retention_days")
	retentionDays := 0
	if retentionRaw != "" {
		retentionDays, err = strconv.Atoi(retentionRaw)
		if err != nil {
			return nil, errors.Wrap(err, "parse rotation retention days")
		}
	}

	writer, err := newRotationWriter(path, interval, retentionDays)
	if err != nil {
		return nil, err
	}

	return writer, nil
}

func (w *rotationWriter) ensureActiveFile(now time.Time) error {
	if w.file == nil {
		start, next := rotationWindow(now, w.interval)
		if err := w.openNewFile(start, next); err != nil {
			return err
		}
		return w.cleanup(start)
	}

	if now.Before(w.nextRotate) {
		return nil
	}

	start, next := rotationWindow(now, w.interval)
	return w.rotateTo(start, next)
}

func rotationWindow(now time.Time, interval RotationInterval) (time.Time, time.Time) {
	now = now.UTC()
	switch interval {
	case RotationHourly:
		start := now.Truncate(time.Hour)
		return start, start.Add(time.Hour)
	case RotationDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.Add(24 * time.Hour)
	case RotationWeekly:
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		weekday := midnight.Weekday()
		daysSinceMonday := (int(weekday) - int(time.Monday) + 7) % 7
		start := midnight.AddDate(0, 0, -daysSinceMonday)
		return start, start.Add(7 * 24 * time.Hour)
	default:
		panic(fmt.Sprintf("unexpected rotation interval: %s", interval.String()))
	}
}

func (w *rotationWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now().UTC()
	if err := w.ensureActiveFile(now); err != nil {
		return 0, errors.Wrap(err, "prepare log file")
	}

	n, err := w.file.Write(p)
	if err != nil {
		return n, errors.Wrap(err, "write log file")
	}
	return n, nil
}

func (w *rotationWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if err := w.file.Sync(); err != nil {
		return errors.Wrap(err, "sync log file")
	}

	return nil
}

func (w *rotationWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}

	if err := w.file.Close(); err != nil {
		return errors.Wrap(err, "close log file")
	}
	w.file = nil
	return nil
}

func (w *rotationWriter) openNewFile(start, next time.Time) error {
	if err := ensureDir(filepath.Dir(w.path)); err != nil {
		return err
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	w.file = file
	w.currentStart = start
	w.nextRotate = next
	return nil
}

func (w *rotationWriter) rotateTo(start, next time.Time) error {
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return errors.Wrap(err, "sync rotating log file")
		}
		if err := w.file.Close(); err != nil {
			return errors.Wrap(err, "close rotating log file")
		}
		rotated := fmt.Sprintf("%s.%s", w.path, w.currentStart.Format(rotationSuffixLayout))
		if err := os.Rename(w.path, rotated); err != nil {
			if !os.IsNotExist(err) {
				return errors.Wrap(err, "rename rotated log file")
			}
		}
	}

	if err := w.openNewFile(start, next); err != nil {
		return err
	}

	return w.cleanup(start)
}

func (w *rotationWriter) cleanup(current time.Time) error {
	if w.retentionDays <= 0 {
		return nil
	}

	threshold := current.AddDate(0, 0, -w.retentionDays)
	dir := filepath.Dir(w.path)
	base := filepath.Base(w.path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(err, "list log directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name == base {
			continue
		}

		if !strings.HasPrefix(name, base+".") {
			continue
		}

		tsPart := strings.TrimPrefix(name, base+".")
		ts, err := time.ParseInLocation(rotationSuffixLayout, tsPart, time.UTC)
		if err != nil {
			continue
		}

		if ts.Before(threshold) {
			target := filepath.Join(dir, name)
			if err := os.Remove(target); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return errors.Wrapf(err, "remove expired log %s", target)
			}
		}
	}

	return nil
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.Wrap(err, "create log directory")
	}
	return nil
}
