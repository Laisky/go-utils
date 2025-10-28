package log

import (
	"fmt"
	"os"
	"path/filepath"
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

	writer, err := newRotationWriter(logFile, RotationHourly, 0)
	require.NoError(t, err)
	defer require.NoError(t, writer.Close())

	startTime := time.Date(2025, time.January, 1, 0, 30, 0, 0, time.UTC)
	writer.now = func() time.Time { return startTime }

	_, err = writer.Write([]byte("first entry\n"))
	require.NoError(t, err)

	writer.now = func() time.Time { return startTime.Add(90 * time.Minute) }

	_, err = writer.Write([]byte("second entry\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Sync())

	rotatedName := fmt.Sprintf("%s.%s", logFile, startTime.Truncate(time.Hour).Format(rotationSuffixLayout))

	rotatedContent, err := os.ReadFile(rotatedName)
	require.NoError(t, err)
	require.Contains(t, string(rotatedContent), "first entry")

	currentContent, err := os.ReadFile(logFile)
	require.NoError(t, err)
	require.Contains(t, string(currentContent), "second entry")
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

	raw, err := os.ReadFile(logFile)
	require.NoError(t, err)
	require.Contains(t, string(raw), "rotation-enabled")
}

func TestRotationRetention(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")

	writer, err := newRotationWriter(logFile, RotationDaily, 1)
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

	day1Rotated := fmt.Sprintf("%s.%s", logFile, time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC).Format(rotationSuffixLayout))
	require.FileExists(t, day1Rotated)

	writer.now = func() time.Time { return day3 }
	_, err = writer.Write([]byte("day3\n"))
	require.NoError(t, err)

	require.NoError(t, writer.Sync())

	_, err = os.Stat(day1Rotated)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.FileExists(t, logFile)
	day2Rotated := fmt.Sprintf("%s.%s", logFile, time.Date(2025, time.January, 2, 0, 0, 0, 0, time.UTC).Format(rotationSuffixLayout))
	require.FileExists(t, day2Rotated)
}
