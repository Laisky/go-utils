package log

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
)

const defaultRotationFilenamePattern = "{logger}-YYYYMMDD.log"

type rotationConfig struct {
	path            string
	retentionDays   int
	filenamePattern string
}

// ensureRotationConfig lazily allocates the rotation settings on o so rotation options can
// be applied in any order. It returns the existing or newly created rotationConfig.
func (o *option) ensureRotationConfig() *rotationConfig {
	if o.rotation == nil {
		o.rotation = &rotationConfig{}
	}

	return o.rotation
}

// configureRotation validates the rotation settings collected from options and, when
// rotation is enabled, registers the rotation sink and replaces o.OutputPaths with a single
// "rotate:" URL carrying the path, retention days, filename pattern and sanitized logger
// name. A blank pattern falls back to defaultRotationFilenamePattern, and an unset or
// default logger name is replaced by one derived from the log path. It returns nil when
// rotation is not configured, or an error when the path is empty, the retention is
// negative, the pattern is invalid or the sink cannot be registered.
func (o *option) configureRotation() error {
	if o.rotation == nil {
		return nil
	}

	if o.rotation.path == "" {
		return errors.Errorf("rotation path must not be empty")
	}

	if o.rotation.retentionDays < 0 {
		return errors.Errorf("rotation retention days must be >= 0")
	}

	pattern := strings.TrimSpace(o.rotation.filenamePattern)
	if pattern == "" {
		pattern = defaultRotationFilenamePattern
	}

	if _, err := compileRotationPattern(pattern); err != nil {
		return errors.Wrap(err, "validate rotation filename pattern")
	}

	o.rotation.filenamePattern = pattern

	if err := ensureRotationSinkRegistered(); err != nil {
		return err
	}

	loggerName := sanitizeLoggerSegment(o.Name)
	baseName := deriveFallbackLoggerName(o.rotation.path)
	if (o.Name == "" || o.Name == defaultLoggerName) && baseName != "" {
		loggerName = sanitizeLoggerSegment(baseName)
	}

	query := url.Values{}
	query.Set("path", o.rotation.path)
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

const rotationScheme = "rotate"

var (
	registerRotationSinkOnce sync.Once
	errRotationSink          error
)

// ensureRotationSinkRegistered registers createRotationSink with zap for the "rotate" URL
// scheme exactly once per process. It returns the wrapped registration error, which is
// remembered and returned again on every later call, or nil on success.
func ensureRotationSinkRegistered() error {
	registerRotationSinkOnce.Do(func() {
		errRotationSink = zap.RegisterSink(rotationScheme, createRotationSink)
	})

	if errRotationSink != nil {
		return errors.Wrap(errRotationSink, "register rotation sink")
	}

	return nil
}

// WithRotation configures daily log rotation for the provided path. An optional
// rotationDays argument controls how many days of history are retained. Passing
// 0 (the default) keeps all rotated files indefinitely.
func WithRotation(path string, rotationDays ...int) Option {
	return func(c *option) error {
		if path == "" {
			return errors.Errorf("rotation path must not be empty")
		}

		cfg := c.ensureRotationConfig()
		cfg.path = path

		if len(rotationDays) > 1 {
			return errors.Errorf("rotationDays accepts at most one value")
		}

		if len(rotationDays) == 1 {
			days := rotationDays[0]
			if days < 0 {
				return errors.Errorf("rotation retention days must be >= 0")
			}
			cfg.retentionDays = days
		}

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
// the logger name via {logger}. The result must be one portable filename: no
// separators, volume/stream syntax, Windows device names, or trailing dots/spaces.
// Final filenames are limited to 255 UTF-8 bytes. Lexical validation alone does not
// stop links; each daily file is opened without following a final link and must
// be a regular, single-link file, so links, FIFOs and other special files fail.
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

// createRotationSink is the zap sink factory for "rotate:" URLs. It reads the path (from
// the query, falling back to the URL path), retention_days, pattern and logger query
// parameters of u and builds a rotationWriter from them. It returns the writer as a
// zap.Sink, or an error when the query cannot be parsed, the path is missing,
// retention_days is not an integer or the writer settings are invalid.
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

	writer, err := newRotationWriter(path, retentionDays, pattern, loggerName)
	if err != nil {
		return nil, err
	}

	return writer, nil
}

// sanitizeLoggerSegment converts raw into a filename-safe logger segment: letters are
// lowercased, digits, '-', '_' and '.' are kept, every other rune becomes '-', and leading
// or trailing '-', '_' and '.' are trimmed. It returns defaultLoggerName when raw is blank
// or nothing remains after sanitizing.
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

// deriveFallbackLoggerName derives a logger name from the base name of path without its
// extension, for example "app" for "/var/log/app.log". It returns defaultLoggerName when
// the base is "." or the path separator, and the full base name when stripping the
// extension would leave nothing.
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
