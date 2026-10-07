package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity67PortablePatterns rejects path syntax on every host without opening it.
func TestSecurity67PortablePatterns(t *testing.T) {
	for _, pattern := range []string{
		"../YYYYMMDD.log", `..\YYYYMMDD.log`, "C:/tmp/YYYYMMDD.log", `C:\tmp\YYYYMMDD.log`,
		"C:YYYYMMDD.log", "//example.invalid/share/YYYYMMDD.log", `\\?\C:\YYYYMMDD.log`,
		"YYYYMMDD.log:stream", "CON.YYYYMMDD.log", "LPT1.YYYYMMDD.log", "COM¹.YYYYMMDD.log",
		"YYYYMMDD.log.", "YYYYMMDD?.log", "YYYYMMDD\x00.log", "YYYYMMDD\n.log",
	} {
		t.Run(pattern, func(t *testing.T) {
			parsed, err := compileRotationPattern(pattern)
			require.Error(t, err)
			require.Nil(t, parsed)
		})
	}
}

// TestSecurity67FormattedFilename validates substitutions and containment before I/O.
func TestSecurity67FormattedFilename(t *testing.T) {
	pattern, err := compileRotationPattern("{logger}.YYYYMMDD.log")
	require.NoError(t, err)
	base := filepath.Join(t.TempDir(), "logs")
	stamp := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"../escape", `..\escape`, "C:escape", "CON", "CON ", "LPT²", "bad?name", "bad\x00name"} {
		output, err := pattern.Path(name, stamp, base)
		require.Error(t, err, name)
		require.Empty(t, output)
	}
	for _, name := range []string{"app", "日志", "COM10", "CONsole", "service-name"} {
		output, err := pattern.Path(name, stamp, base)
		require.NoError(t, err, name)
		require.Equal(t, filepath.Join(base, name+".20261006.log"), output)
		relative, err := filepath.Rel(base, output)
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(relative))
	}
	_, err = os.Stat(base)
	require.True(t, os.IsNotExist(err), "pure path validation must not create a directory or contact a share")
}

// TestSecurity67SingleFilenameContract rejects raw absolute paths even in a prebuilt pattern.
func TestSecurity67SingleFilenameContract(t *testing.T) {
	for _, literal := range []string{"", ".", "..", "/tmp/not-opened.log", `C:\not-opened.log`, "name.log ", "NUL.txt"} {
		pattern := &rotationPattern{elements: []patternElement{{kind: patternLiteral, literal: literal}}}
		output, err := pattern.Path("app", time.Time{}, ".")
		require.Error(t, err, literal)
		require.Empty(t, output)
	}
}

// TestSecurity67WriterRejectsEscape never writes outside its disposable log directory.
func TestSecurity67WriterRejectsEscape(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "logs")
	writer, err := newRotationWriter(filepath.Join(base, "app.log"), 0, "../YYYYMMDD.log", "app")
	if writer != nil {
		defer writer.Close()
	}
	require.Error(t, err)
	require.Nil(t, writer)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}
