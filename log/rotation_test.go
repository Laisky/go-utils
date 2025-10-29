package log

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotationWindowDaily(t *testing.T) {
	cases := []struct {
		name  string
		when  time.Time
		start time.Time
		next  time.Time
	}{
		{
			name:  "midday",
			when:  time.Date(2025, time.October, 28, 13, 37, 0, 0, time.UTC),
			start: time.Date(2025, time.October, 28, 0, 0, 0, 0, time.UTC),
			next:  time.Date(2025, time.October, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "boundary",
			when:  time.Date(2025, time.October, 29, 0, 0, 0, 0, time.UTC),
			start: time.Date(2025, time.October, 29, 0, 0, 0, 0, time.UTC),
			next:  time.Date(2025, time.October, 30, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			start, next := rotationWindow(tc.when)
			require.Equal(t, tc.start, start)
			require.Equal(t, tc.next, next)
		})
	}
}

func TestRotationWriterDailyRotation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, 0, "", "")
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	day1 := time.Date(2025, time.January, 1, 10, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return day1 }

	_, err = writer.Write([]byte("first entry\n"))
	require.NoError(t, err)

	day1Path := filepath.Join(dir, "app-20250101.log")
	require.FileExists(t, day1Path)

	content, err := os.ReadFile(day1Path)
	require.NoError(t, err)
	require.Contains(t, string(content), "first entry")

	day2 := day1.Add(24 * time.Hour)
	writer.now = func() time.Time { return day2 }

	_, err = writer.Write([]byte("second entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	day2Path := filepath.Join(dir, "app-20250102.log")
	require.FileExists(t, day2Path)

	content, err = os.ReadFile(day2Path)
	require.NoError(t, err)
	require.Contains(t, string(content), "second entry")
}

func TestLoggerWithRotationWrites(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	logger, err := New(
		WithEncoding(EncodingJSON),
		WithRotation(logFile),
	)
	require.NoError(t, err)

	logger.Info("rotation-enabled")
	require.NoError(t, logger.Sync())

	files, err := os.ReadDir(dir)
	require.NoError(t, err)

	expectedPrefix := sanitizeLoggerSegment(deriveFallbackLoggerName(logFile))
	namePattern := regexp.MustCompile("^" + regexp.QuoteMeta(expectedPrefix) + `-[0-9]{8}\.log$`)
	matched := false
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if namePattern.MatchString(f.Name()) {
			matched = true
			break
		}
	}

	require.True(t, matched, "expected daily log file to match pattern {logger}-YYYYMMDD.log")
}

func TestRotationRetention(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, 1, "", "")
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	day1 := time.Date(2025, time.January, 1, 1, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)
	day3 := day1.AddDate(0, 0, 2)

	writer.now = func() time.Time { return day1 }
	_, err = writer.Write([]byte("day1\n"))
	require.NoError(t, err)

	writer.now = func() time.Time { return day2 }
	_, err = writer.Write([]byte("day2\n"))
	require.NoError(t, err)

	day1Path := filepath.Join(dir, "app-20250101.log")
	require.FileExists(t, day1Path)

	writer.now = func() time.Time { return day3 }
	_, err = writer.Write([]byte("day3\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	_, err = os.Stat(day1Path)
	require.ErrorIs(t, err, os.ErrNotExist)

	day2Path := filepath.Join(dir, "app-20250102.log")
	require.FileExists(t, day2Path)
}

func TestRotationWriterCustomPattern(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "Service.log")

	writer, err := newRotationWriter(logFile, 0, "{logger}-YYYYMMDD-HH.log", "MySvc")
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	base := time.Date(2025, time.January, 1, 10, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return base }

	_, err = writer.Write([]byte("alpha\n"))
	require.NoError(t, err)

	firstStart, _ := rotationWindow(base)
	firstPath, err := writer.pattern.Path(writer.loggerName, firstStart, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, firstPath)
	require.Equal(t, filepath.Join(dir, "mysvc-20250101-00.log"), firstPath)

	content, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "alpha")

	writer.now = func() time.Time { return base.Add(24 * time.Hour) }
	_, err = writer.Write([]byte("beta\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	secondStart, _ := rotationWindow(base.Add(24 * time.Hour))
	secondPath, err := writer.pattern.Path(writer.loggerName, secondStart, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, secondPath)
	require.Equal(t, filepath.Join(dir, "mysvc-20250102-00.log"), secondPath)
	secondContent, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	require.Contains(t, string(secondContent), "beta")
}
