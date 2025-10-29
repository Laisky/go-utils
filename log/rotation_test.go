package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRotationWindowBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		when     time.Time
		interval RotationInterval
		start    time.Time
		next     time.Time
	}{
		{
			name:     "hourly",
			when:     time.Date(2025, time.October, 28, 13, 37, 0, 0, time.UTC),
			interval: RotationHourly,
			start:    time.Date(2025, time.October, 28, 13, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.October, 28, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily",
			when:     time.Date(2025, time.October, 28, 13, 37, 0, 0, time.UTC),
			interval: RotationDaily,
			start:    time.Date(2025, time.October, 28, 0, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.October, 29, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "weekly",
			when:     time.Date(2025, time.October, 28, 13, 37, 0, 0, time.UTC),
			interval: RotationWeekly,
			start:    time.Date(2025, time.October, 27, 0, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.November, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "weekly-boundary",
			when:     time.Date(2025, time.November, 3, 0, 0, 0, 0, time.UTC),
			interval: RotationWeekly,
			start:    time.Date(2025, time.November, 3, 0, 0, 0, 0, time.UTC),
			next:     time.Date(2025, time.November, 10, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			start, next := rotationWindow(tc.when, tc.interval)
			require.Equal(t, tc.start, start)
			require.Equal(t, tc.next, next)
		})
	}
}

func TestRotationWriterRotatesHourly(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, RotationHourly, 0, "", "")
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	startTime := time.Date(2025, time.January, 1, 0, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return startTime }

	_, err = writer.Write([]byte("first entry\n"))
	require.NoError(t, err)

	firstStart, _ := rotationWindow(startTime, RotationHourly)
	firstPath, err := writer.pattern.Path(writer.loggerName, firstStart, RotationHourly, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, firstPath)

	firstContent, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	require.Contains(t, string(firstContent), "first entry")

	writer.now = func() time.Time { return startTime.Add(time.Hour) }

	_, err = writer.Write([]byte("second entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	secondStart, _ := rotationWindow(startTime.Add(time.Hour), RotationHourly)
	secondPath, err := writer.pattern.Path(writer.loggerName, secondStart, RotationHourly, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, secondPath)

	secondContent, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	require.Contains(t, string(secondContent), "second entry")
}

func TestLoggerWithRotationWrites(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	logger, err := New(
		WithEncoding(EncodingJSON),
		WithRotation(logFile, RotationHourly),
	)
	require.NoError(t, err)

	logger.Info("rotation-enabled")
	require.NoError(t, logger.Sync())

	files, err := os.ReadDir(dir)
	require.NoError(t, err)

	var target string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasPrefix(f.Name(), "app-") && strings.HasSuffix(f.Name(), ".log") {
			target = filepath.Join(dir, f.Name())
			break
		}
	}
	require.NotEmpty(t, target)

	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Contains(t, string(raw), "rotation-enabled")
}

func TestRotationRetention(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, RotationDaily, 1, "", "")
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

	day1Start, _ := rotationWindow(day1, RotationDaily)
	day1Path, err := writer.pattern.Path(writer.loggerName, day1Start, RotationDaily, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, day1Path)

	writer.now = func() time.Time { return day3 }
	_, err = writer.Write([]byte("day3\n"))
	require.NoError(t, err)

	require.NoError(t, writer.Sync())

	_, err = os.Stat(day1Path)
	require.ErrorIs(t, err, os.ErrNotExist)
	day2Start, _ := rotationWindow(day2, RotationDaily)
	day2Path, err := writer.pattern.Path(writer.loggerName, day2Start, RotationDaily, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, day2Path)
}

func TestRotationWriterCustomPattern(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "Service.log")

	writer, err := newRotationWriter(logFile, RotationHourly, 0, "{logger}-YYYYMMDD-HH.log", "MySvc")
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	base := time.Date(2025, time.January, 1, 10, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return base }

	_, err = writer.Write([]byte("alpha\n"))
	require.NoError(t, err)

	firstStart, _ := rotationWindow(base, RotationHourly)
	firstPath, err := writer.pattern.Path(writer.loggerName, firstStart, RotationHourly, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, firstPath)
	content, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "alpha")

	writer.now = func() time.Time { return base.Add(time.Hour) }
	_, err = writer.Write([]byte("beta\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	secondStart, _ := rotationWindow(base.Add(time.Hour), RotationHourly)
	secondPath, err := writer.pattern.Path(writer.loggerName, secondStart, RotationHourly, writer.baseDir)
	require.NoError(t, err)
	require.FileExists(t, secondPath)
	secondContent, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	require.Contains(t, string(secondContent), "beta")
}
