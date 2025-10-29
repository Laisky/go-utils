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
	"unicode"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
)

const (
	rotationSuffixLayout           = "20060102T150405Z"
	defaultRotationFilenamePattern = "{logger}-YYYYMMDD.log"
)

type rotationConfig struct {
	path            string
	interval        RotationInterval
	retentionDays   int
	filenamePattern string
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

	pattern := o.rotation.filenamePattern
	if pattern == "" {
		pattern = defaultPatternForInterval(o.rotation.interval)
	}
	pattern = strings.TrimSpace(pattern)
	if _, err := compileRotationPattern(pattern, o.rotation.interval); err != nil {
		return errors.Wrap(err, "validate rotation filename pattern")
	}

	loggerName := sanitizeLoggerSegment(o.Name)
	baseName := deriveFallbackLoggerName(o.rotation.path)
	if (o.Name == "" || o.Name == defaultLoggerName) && baseName != "" {
		loggerName = sanitizeLoggerSegment(baseName)
	}

	query := url.Values{}
	query.Set("path", o.rotation.path)
	query.Set("interval", o.rotation.interval.String())
	query.Set("retention_days", strconv.Itoa(o.rotation.retentionDays))
	query.Set("pattern", pattern)
	query.Set("logger", loggerName)

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

// WithRotationFilenamePattern overrides the default filename pattern used when creating
// rotated log files. The pattern must include YYYY, MM, and DD tokens and may reference
// the logger name via {logger}.
func WithRotationFilenamePattern(pattern string) Option {
	return func(c *option) error {
		if strings.TrimSpace(pattern) == "" {
			return errors.Errorf("rotation filename pattern must not be empty")
		}
		cfg := c.ensureRotationConfig()
		cfg.filenamePattern = pattern
		return nil
	}
}

type rotationWriter struct {
	mu            sync.Mutex
	file          *os.File
	basePath      string
	baseDir       string
	interval      RotationInterval
	retentionDays int
	pattern       *rotationPattern
	loggerName    string
	now           func() time.Time
	currentStart  time.Time
	nextRotate    time.Time
	currentPath   string
}

func newRotationWriter(path string, interval RotationInterval, retentionDays int, pattern string, loggerName string) (*rotationWriter, error) {
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
	if pattern == "" {
		pattern = defaultPatternForInterval(interval)
	}

	rp, err := compileRotationPattern(pattern, interval)
	if err != nil {
		return nil, errors.Wrap(err, "compile rotation filename pattern")
	}

	if strings.TrimSpace(loggerName) == "" {
		loggerName = deriveFallbackLoggerName(cleaned)
	}
	sanitizedLogger := sanitizeLoggerSegment(loggerName)

	writer := &rotationWriter{
		basePath:      cleaned,
		baseDir:       filepath.Dir(cleaned),
		interval:      interval,
		retentionDays: retentionDays,
		pattern:       rp,
		loggerName:    sanitizedLogger,
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

	pattern := query.Get("pattern")
	loggerName := query.Get("logger")

	writer, err := newRotationWriter(path, interval, retentionDays, pattern, loggerName)
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
	if err := w.rotateTo(start, next); err != nil {
		return err
	}
	return w.cleanup(start)
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
	path, err := w.pattern.Path(w.loggerName, start, w.interval, w.baseDir)
	if err != nil {
		return err
	}

	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	w.file = file
	w.currentStart = start
	w.nextRotate = next
	w.currentPath = filepath.Clean(path)
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
		w.file = nil
	}

	return w.openNewFile(start, next)
}

func (w *rotationWriter) cleanup(current time.Time) error {
	if w.retentionDays <= 0 {
		return nil
	}

	threshold := current.AddDate(0, 0, -w.retentionDays)
	dir := w.baseDir
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(err, "list log directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Clean(filepath.Join(dir, name))
		if path == w.currentPath {
			continue
		}

		start, ok := w.pattern.Parse(name, w.loggerName)
		if !ok {
			continue
		}

		if start.Before(threshold) {
			if err := os.Remove(path); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return errors.Wrapf(err, "remove expired log %s", path)
			}
		}
	}

	return nil
}

type patternElementKind int

const (
	patternLiteral patternElementKind = iota
	patternLogger
	patternYear
	patternMonth
	patternDay
	patternHour
	patternMinute
	patternSecond
	patternAutoSubdaily
)

type patternElement struct {
	kind    patternElementKind
	literal string
}

type rotationPattern struct {
	raw          string
	elements     []patternElement
	hasYear      bool
	hasMonth     bool
	hasDay       bool
	hasHour      bool
	hasMinute    bool
	hasSecond    bool
	autoSubdaily bool
}

func (rp *rotationPattern) Path(logger string, start time.Time, _ RotationInterval, baseDir string) (string, error) {
	relative := rp.format(logger, start)
	if relative == "" {
		return "", errors.Errorf("rotation filename pattern produced empty filename")
	}

	if filepath.IsAbs(relative) {
		return filepath.Clean(relative), nil
	}

	if baseDir == "" || baseDir == "." {
		return filepath.Clean(relative), nil
	}

	return filepath.Clean(filepath.Join(baseDir, relative)), nil
}

func (rp *rotationPattern) format(logger string, start time.Time) string {
	var b strings.Builder
	for _, el := range rp.elements {
		switch el.kind {
		case patternLiteral:
			b.WriteString(el.literal)
		case patternLogger:
			b.WriteString(logger)
		case patternYear:
			b.WriteString(fmt.Sprintf("%04d", start.Year()))
		case patternMonth:
			b.WriteString(fmt.Sprintf("%02d", int(start.Month())))
		case patternDay:
			b.WriteString(fmt.Sprintf("%02d", start.Day()))
		case patternHour:
			b.WriteString(fmt.Sprintf("%02d", start.Hour()))
		case patternMinute:
			b.WriteString(fmt.Sprintf("%02d", start.Minute()))
		case patternSecond:
			b.WriteString(fmt.Sprintf("%02d", start.Second()))
		case patternAutoSubdaily:
			b.WriteByte('-')
			b.WriteString(start.Format("150405"))
		}
	}

	return b.String()
}

func (rp *rotationPattern) Parse(name string, logger string) (time.Time, bool) {
	pos := 0
	parts := timeParts{year: -1, month: -1, day: -1, hour: 0, minute: 0, second: 0}

	for _, el := range rp.elements {
		switch el.kind {
		case patternLiteral:
			if !strings.HasPrefix(name[pos:], el.literal) {
				return time.Time{}, false
			}
			pos += len(el.literal)
		case patternLogger:
			if !strings.HasPrefix(name[pos:], logger) {
				return time.Time{}, false
			}
			pos += len(logger)
		case patternYear:
			value, ok := parseFourDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.year = value
			pos += 4
		case patternMonth:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.month = value
			pos += 2
		case patternDay:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.day = value
			pos += 2
		case patternHour:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.hour = value
			pos += 2
		case patternMinute:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.minute = value
			pos += 2
		case patternSecond:
			value, ok := parseTwoDigits(name, pos)
			if !ok {
				return time.Time{}, false
			}
			parts.second = value
			pos += 2
		case patternAutoSubdaily:
			if pos+7 > len(name) {
				return time.Time{}, false
			}
			if name[pos] != '-' {
				return time.Time{}, false
			}
			segment := name[pos+1 : pos+7]
			if !allDigits(segment) {
				return time.Time{}, false
			}
			hourVal, ok := parseTwoDigits(segment, 0)
			if !ok {
				return time.Time{}, false
			}
			minuteVal, ok := parseTwoDigits(segment, 2)
			if !ok {
				return time.Time{}, false
			}
			secondVal, ok := parseTwoDigits(segment, 4)
			if !ok {
				return time.Time{}, false
			}
			parts.hour = hourVal
			parts.minute = minuteVal
			parts.second = secondVal
			pos += 7
		}
	}

	if pos != len(name) {
		return time.Time{}, false
	}

	if parts.year < 0 || parts.month < 1 || parts.day < 1 {
		return time.Time{}, false
	}

	if parts.month > 12 || parts.day > 31 || parts.hour > 23 || parts.minute > 59 || parts.second > 59 {
		return time.Time{}, false
	}

	result := time.Date(parts.year, time.Month(parts.month), parts.day, parts.hour, parts.minute, parts.second, 0, time.UTC)
	return result, true
}

type timeParts struct {
	year   int
	month  int
	day    int
	hour   int
	minute int
	second int
}

func parseFourDigits(input string, pos int) (int, bool) {
	if pos+4 > len(input) {
		return 0, false
	}
	return parseDigits(input[pos : pos+4])
}

func parseTwoDigits(input string, pos int) (int, bool) {
	if pos+2 > len(input) {
		return 0, false
	}
	return parseDigits(input[pos : pos+2])
}

func parseDigits(segment string) (int, bool) {
	value := 0
	for _, r := range segment {
		if r < '0' || r > '9' {
			return 0, false
		}
		value = value*10 + int(r-'0')
	}
	return value, true
}

func allDigits(segment string) bool {
	for _, r := range segment {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compileRotationPattern(pattern string, interval RotationInterval) (*rotationPattern, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.Errorf("rotation filename pattern must not be empty")
	}

	rp := &rotationPattern{raw: pattern}

	for i := 0; i < len(pattern); {
		if strings.HasPrefix(pattern[i:], "{logger}") {
			rp.elements = append(rp.elements, patternElement{kind: patternLogger})
			i += len("{logger}")
			continue
		}

		if token, kind := matchPatternToken(pattern[i:]); token != "" {
			rp.elements = append(rp.elements, patternElement{kind: kind})
			i += len(token)
			switch kind {
			case patternYear:
				rp.hasYear = true
			case patternMonth:
				rp.hasMonth = true
			case patternDay:
				rp.hasDay = true
			case patternHour:
				rp.hasHour = true
			case patternMinute:
				rp.hasMinute = true
			case patternSecond:
				rp.hasSecond = true
			}
			continue
		}

		j := i + 1
		for j < len(pattern) {
			if strings.HasPrefix(pattern[j:], "{logger}") {
				break
			}
			if token, _ := matchPatternToken(pattern[j:]); token != "" {
				break
			}
			j++
		}
		literal := pattern[i:j]
		if strings.ContainsRune(literal, os.PathSeparator) {
			return nil, errors.Errorf("rotation filename pattern must not contain path separators")
		}
		rp.elements = append(rp.elements, patternElement{kind: patternLiteral, literal: literal})
		i = j
	}

	if !rp.hasYear || !rp.hasMonth || !rp.hasDay {
		return nil, errors.Errorf("rotation filename pattern must include YYYY, MM, and DD tokens")
	}

	if time.Duration(interval) < 24*time.Hour && !rp.hasHour && !rp.hasMinute && !rp.hasSecond {
		rp.autoSubdaily = true
		rp.insertAutoSubdailySuffix()
	}

	return rp, nil
}

func (rp *rotationPattern) insertAutoSubdailySuffix() {
	insertIdx := len(rp.elements)
	for idx, el := range rp.elements {
		if el.kind == patternLiteral && len(el.literal) > 0 && el.literal[0] == '.' {
			insertIdx = idx
			break
		}
	}

	rp.elements = append(rp.elements, patternElement{})
	copy(rp.elements[insertIdx+1:], rp.elements[insertIdx:])
	rp.elements[insertIdx] = patternElement{kind: patternAutoSubdaily}
}

func matchPatternToken(input string) (string, patternElementKind) {
	tokens := []struct {
		text string
		kind patternElementKind
	}{
		{"YYYY", patternYear},
		{"MM", patternMonth},
		{"DD", patternDay},
		{"hh", patternHour},
		{"HH", patternHour},
		{"mm", patternMinute},
		{"ss", patternSecond},
	}

	for _, token := range tokens {
		if strings.HasPrefix(input, token.text) {
			return token.text, token.kind
		}
	}

	return "", 0
}

func defaultPatternForInterval(interval RotationInterval) string {
	return defaultRotationFilenamePattern
}

func sanitizeLoggerSegment(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = defaultLoggerName
	}

	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}

	res := strings.Trim(b.String(), "-_.")
	if res == "" {
		return defaultLoggerName
	}
	return res
}

func deriveFallbackLoggerName(path string) string {
	base := filepath.Base(path)
	if base == "." || base == string(os.PathSeparator) {
		return defaultLoggerName
	}

	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" {
		name = base
	}

	return name
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
