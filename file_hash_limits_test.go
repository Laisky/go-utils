package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// security64Factory returns a caller-supplied hasher for deterministic I/O transitions.
type security64Factory struct {
	value hash.Hash
	err   error
}

// String describes the test-only hash factory.
func (s security64Factory) String() string { return "test-sha256" }

// Hasher returns the configured hasher or construction error.
func (s security64Factory) Hasher() (hash.Hash, error) { return s.value, s.err }

// security64Hash invokes its hook once while retaining the real SHA-256 computation.
type security64Hash struct {
	hash.Hash
	once sync.Once
	hook func()
}

// Write applies the deterministic transition then hashes the supplied bytes.
func (s *security64Hash) Write(p []byte) (int, error) { s.once.Do(s.hook); return s.Hash.Write(p) }

// TestSecurity64ByteLimits exercises finite below/at/above limits without large files.
func TestSecurity64ByteLimits(t *testing.T) {
	for _, size := range []int{0, 1, 31, 32, 33} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "input")
			data := bytes.Repeat([]byte{'x'}, size)
			require.NoError(t, os.WriteFile(p, data, 0600))
			actual, err := FileHashWithContext(context.Background(), HashTypeSha256, p, 32)
			if size > 32 {
				require.ErrorIs(t, err, ErrFileHashLimit)
				require.Nil(t, actual)
				return
			}
			expected := sha256.Sum256(data)
			require.NoError(t, err)
			require.Equal(t, expected[:], actual)
			require.NoError(t, VerifyFileHashWithContext(context.Background(), p, "sha256:"+hex.EncodeToString(expected[:]), 32))
			require.ErrorContains(t, VerifyFileHash(p, "sha256:"+strings.Repeat("0", 64)), "mismatch")
		})
	}
}

// TestSecurity64GrowthAndCancellation mutates tiny files during real hashing.
func TestSecurity64GrowthAndCancellation(t *testing.T) {
	for _, mode := range []string{"grow-over-budget", "grow-under-budget", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "input")
			require.NoError(t, os.WriteFile(p, []byte("original"), 0600))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := &security64Hash{Hash: sha256.New(), hook: func() {
				if mode == "cancel" {
					cancel()
					return
				}
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
				require.NoError(t, err)
				_, err = f.Write([]byte("growth"))
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}}
			limit := int64(64)
			if mode == "grow-over-budget" {
				limit = 8
			}
			signature, err := FileHashWithContext(ctx, security64Factory{value: h}, p, limit)
			require.Error(t, err)
			require.Nil(t, signature)
			if mode == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if mode == "grow-over-budget" {
				require.ErrorIs(t, err, ErrFileHashLimit)
			}
			if mode == "grow-under-budget" {
				require.ErrorContains(t, err, "changed")
			}
			require.NoError(t, os.Remove(p), "hash descriptor must be closed before return")
		})
	}
}

// TestSecurity64RejectedInputs validates configuration and encoded hashes before I/O.
func TestSecurity64RejectedInputs(t *testing.T) {
	_, err := FileHashWithContext(nil, HashTypeSha256, "missing", 32)
	require.Error(t, err)
	_, err = FileHashWithContext(context.Background(), nil, "missing", 32)
	require.Error(t, err)
	_, err = FileHashWithContext(context.Background(), HashTypeSha256, "missing", -1)
	require.ErrorContains(t, err, "limit")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FileHashWithContext(canceled, HashTypeSha256, "missing", 32)
	require.ErrorIs(t, err, context.Canceled)
	for _, encoded := range []string{"", "sha256:", "sha256:" + strings.Repeat("x", 64), "unknown:abcd", "sha256:" + strings.Repeat("0", 128)} {
		err = VerifyFileHash("missing", encoded)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "open")
		require.NotContains(t, err.Error(), "inspect")
	}
	_, err = FileHashWithContext(context.Background(), security64Factory{err: io.ErrUnexpectedEOF}, "missing", 32)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	_, err = FileHashWithContext(context.Background(), security64Factory{}, "missing", 32)
	require.ErrorContains(t, err, "hasher must not be nil")
}

// TestSecurity64ReaderCompatibility preserves unlimited caller-owned reader Hash behavior.
func TestSecurity64ReaderCompatibility(t *testing.T) {
	data := []byte("trusted reader contents")
	result, err := Hash(HashTypeSha256, bytes.NewReader(data))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	require.Equal(t, sum[:], result)
}

// TestSecurity64ConcurrentCalls checks independent hash state and bounded canceled calls.
func TestSecurity64ConcurrentCalls(t *testing.T) {
	p := filepath.Join(t.TempDir(), "input")
	data := bytes.Repeat([]byte{'q'}, 1024)
	require.NoError(t, os.WriteFile(p, data, 0600))
	for i := range 24 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			if i%2 == 0 {
				cancel()
			} else {
				defer cancel()
			}
			_, err := FileHashWithContext(ctx, HashTypeSha256, p, 1024)
			if i%2 == 0 {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// FuzzSecurity64BoundedHash compares bounded reads with the independent SHA-256 oracle.
func FuzzSecurity64BoundedHash(f *testing.F) {
	f.Add([]byte("abc"), uint16(3))
	f.Add([]byte("abcd"), uint16(3))
	f.Add([]byte{}, uint16(0))
	f.Fuzz(func(t *testing.T, data []byte, maximum uint16) {
		if len(data) > 128*1024 {
			t.Skip()
		}
		hasher := sha256.New()
		err := hashBoundedFile(context.Background(), hasher, bytes.NewReader(data), int64(maximum))
		if len(data) > int(maximum) {
			require.ErrorIs(t, err, ErrFileHashLimit)
			return
		}
		require.NoError(t, err)
		expected := sha256.Sum256(data)
		require.Equal(t, expected[:], hasher.Sum(nil))
	})
}
