package log

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

func TestFilter(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestWriteToFile*")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "test.log")
	logger, err := New(
		WithOutputPaths([]string{file}),
		WithZapOptions(
			zap.Filter(func(e zapcore.Entry, f []zapcore.Field) bool {
				return e.Message != "world"
			}),
		),
	)
	require.NoError(t, err)

	logger.Info("hello")
	logger.Info("beautiful")
	logger.Info("world")
	logger.Info("how")
	logger.Info("are")
	logger.Info("you")
	_ = logger.Sync()

	cntBytes, err := os.ReadFile(file)
	content := string(cntBytes)
	t.Logf("content:\n%s", content)
	require.NoError(t, err)
	require.Contains(t, content, "log/filter_test.go")
	require.Contains(t, content, "hello\n")
	require.NotContains(t, content, "world\n")
	require.Len(t, regexp.MustCompile(`hello`).FindAllString(content, -1), 1)
}
