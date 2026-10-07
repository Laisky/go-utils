package compress

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity56CopyBoundary exercises the actual-output limit independently of ZIP headers.
func TestSecurity56CopyBoundary(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 9, 32} {
		for _, chunk := range []int{1, 3, 32768} {
			src := bytes.NewReader(bytes.Repeat([]byte{'x'}, n))
			var dst bytes.Buffer
			written, err := copyZIPLimited(&dst, src, 8, chunk)
			require.EqualValues(t, min(n, 8), written)
			require.Len(t, dst.Bytes(), min(n, 8))
			require.Equal(t, max(0, n-9), src.Len(), "overflow probe consumes at most one extra byte")
			if n > 8 {
				require.ErrorIs(t, err, ErrUnzipByteLimit)
			} else {
				require.NoError(t, err)
			}
		}
	}
	var dst bytes.Buffer
	n, err := copyZIPLimited(&dst, strings.NewReader("tiny"), math.MaxInt64, 1)
	require.NoError(t, err)
	require.EqualValues(t, 4, n)
	_, err = copyZIPLimited(io.Discard, strings.NewReader("tiny"), 1, 0)
	require.Error(t, err)
}

// security56Failure supplies a stable synthetic read or write failure.
type security56Failure struct{ err error }

// Read returns a synthetic failure without producing bytes.
func (f security56Failure) Read([]byte) (int, error) { return 0, f.err }

// Write returns a synthetic failure without accepting bytes.
func (f security56Failure) Write([]byte) (int, error) { return 0, f.err }

// TestSecurity56CopyErrors preserves read and write errors, including the final probe.
func TestSecurity56CopyErrors(t *testing.T) {
	failure := errors.New("synthetic I/O failure")
	_, err := copyZIPLimited(io.Discard, security56Failure{failure}, 8, 1)
	require.ErrorIs(t, err, failure)
	_, err = copyZIPLimited(security56Failure{failure}, strings.NewReader("x"), 8, 1)
	require.ErrorIs(t, err, failure)
	_, err = copyZIPLimited(io.Discard, io.MultiReader(strings.NewReader("12345678"), security56Failure{failure}), 8, 1)
	require.ErrorIs(t, err, failure)
}

// TestSecurity56Policies verifies finite defaults, explicit opt-out, and independent caps.
func TestSecurity56Policies(t *testing.T) {
	def := new(unzipOption).fillDefault()
	require.Equal(t, DefaultUnzipMaxBytes, def.maxBytes)
	require.Equal(t, DefaultUnzipMaxFileBytes, def.maxFileBytes)
	for _, option := range []UnzipOption{nil, UnzipWithMaxBytes(0), UnzipWithMaxBytes(-1), UnzipWithMaxFileBytes(0), UnzipWithCopyChunkBytes(0), UnzipWithCopyChunkBytes(1024*1024 + 1), UnzipWithMaxEntries(-1)} {
		_, err := new(unzipOption).fillDefault().applyOpts(option)
		require.Error(t, err)
	}
	policy, err := new(unzipOption).fillDefault().applyOpts(UnzipWithUnsafeUnlimitedBytes(), UnzipWithMaxBytes(10))
	require.NoError(t, err)
	require.EqualValues(t, 10, policy.maxBytes)
	require.Zero(t, policy.maxFileBytes)
	require.Positive(t, policy.maxEntries)
	path := security56Archive(t, "1234", "5678", "")
	_, err = Unzip(path, t.TempDir(), UnzipWithMaxBytes(8), UnzipWithMaxFileBytes(4), UnzipWithCopyChunkBytes(1))
	require.NoError(t, err)
	for _, opts := range [][]UnzipOption{{UnzipWithMaxBytes(7)}, {UnzipWithMaxFileBytes(3)}} {
		dest := filepath.Join(t.TempDir(), "out")
		_, err = Unzip(path, dest, opts...)
		require.ErrorIs(t, err, ErrUnzipByteLimit)
		_, err = os.Stat(dest)
		require.True(t, os.IsNotExist(err), "all advertised sizes are checked before the first write")
	}
	_, err = Unzip(path, t.TempDir(), UnzipWithMaxBytes(1), UnzipWithUnsafeUnlimitedBytes())
	require.NoError(t, err)
}
