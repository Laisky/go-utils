package log

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/internal/fileguard"
)

type rotationWriter struct {
	mu            sync.Mutex
	file          *os.File
	baseDir       string
	retentionDays int
	loggerName    string
	pattern       *rotationPattern
	now           func() time.Time
	currentStart  time.Time
	nextRotate    time.Time
	currentPath   string
}

func newRotationWriter(path string, retentionDays int, pattern string, loggerName string) (*rotationWriter, error) {
	if path == "" {
		return nil, errors.Errorf("rotation path must not be empty")
	}
	if retentionDays < 0 {
		return nil, errors.Errorf("rotation retention days must be >= 0")
	}

	trimmedPattern := strings.TrimSpace(pattern)
	if trimmedPattern == "" {
		trimmedPattern = defaultRotationFilenamePattern
	}

	compiledPattern, err := compileRotationPattern(trimmedPattern)
	if err != nil {
		return nil, errors.Wrap(err, "compile rotation filename pattern")
	}

	cleaned := filepath.Clean(path)
	baseDir := filepath.Dir(cleaned)

	if strings.TrimSpace(loggerName) == "" {
		loggerName = deriveFallbackLoggerName(cleaned)
	}
	sanitizedLogger := sanitizeLoggerSegment(loggerName)

	writer := &rotationWriter{
		baseDir:       baseDir,
		retentionDays: retentionDays,
		loggerName:    sanitizedLogger,
		pattern:       compiledPattern,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}

	return writer, nil
}

func (w *rotationWriter) ensureActiveFile(now time.Time) error {
	if w.file == nil {
		start, next := rotationWindow(now)
		if err := w.openNewFile(start, next); err != nil {
			return err
		}
		return w.cleanup(start)
	}

	if now.Before(w.nextRotate) {
		return nil
	}

	start, next := rotationWindow(now)
	if err := w.rotateTo(start, next); err != nil {
		return err
	}
	return w.cleanup(start)
}

func rotationWindow(now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.Add(24 * time.Hour)
}

func (w *rotationWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	if err := w.ensureActiveFile(now); err != nil {
		return 0, err
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
	path, err := w.pattern.Path(w.loggerName, start, w.baseDir)
	if err != nil {
		return err
	}

	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}

	// Every first open and rollover uses the same destination policy: no final
	// link is followed, a FIFO cannot block, and only a regular single-link file
	// verified through the opened descriptor is appended to.
	file, err := fileguard.OpenAppend(path, 0o600)
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
			return errors.Wrap(err, "sync log file")
		}
		if err := w.file.Close(); err != nil {
			return errors.Wrap(err, "close log file")
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
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return errors.Wrap(err, "remove expired log file")
			}
		}
	}

	return nil
}

// ensureDir creates missing log directories as private (0700 before the umask).
// Existing directories keep their permissions, so operators who need shared
// access can pre-create the directory with broader permissions. It takes the
// directory and returns an error when it cannot be created.
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.Wrap(err, "create log directory")
	}
	return nil
}
