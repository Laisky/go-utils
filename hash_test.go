package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/cespare/xxhash"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/log"
)

const (
	testhashraw = "dfij3ifj2jjl2jelkjdkwef"
)

func TestHashSHA128String(t *testing.T) {
	t.Parallel()
	val := testhashraw
	got := HashSHA128String(val)
	if got != "57dce855bbee0bef97b63527d473c807a424511d" {
		t.Fatalf("got: %v", got)
	}
}
func ExampleHashSHA128String() {
	val := testhashraw
	got := HashSHA128String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashSHA256String(t *testing.T) {
	t.Parallel()
	val := testhashraw
	got := HashSHA256String(val)
	if got != "fef14c65b3d411fee6b2dbcb791a9536cbf637b153bb1de0aae1b41e3834aebf" {
		t.Fatalf("got: %v", got)
	}

	t.Run("hasher", func(t *testing.T) {
		t.Parallel()
		raw := []byte("hello, world")
		hasher := sha256.New()
		_, err := hasher.Write(raw)
		require.NoError(t, err)
		got1 := hasher.Sum(nil)

		got2 := sha256.Sum256(raw)
		require.Equal(t, got1, got2[:])
	})
}

func ExampleHashSHA256String() {
	val := testhashraw
	got := HashSHA256String(val)
	log.Shared.Info("hash", zap.String("got", got))
}

func TestHashXxhashString(t *testing.T) {
	t.Parallel()
	val := testhashraw
	got := HashXxhashString(val)
	if got != "cbd2efc89af5217d" {
		t.Fatalf("got: %v", got)
	}
}

func ExampleHashXxhashString() {
	val := testhashraw
	got := HashXxhashString(val)
	log.Shared.Info("hash", zap.String("got", got))
}

// hashXxhashStringByWrite hashes input by writing data into xxhash hasher.
//
// Parameters:
// - val: the input string that should be hashed.
//
// Returns:
// - string: the hex-encoded xxhash64 digest of input.
func hashXxhashStringByWrite(val string) string {
	h := xxhash.New()
	_, _ = h.Write([]byte(val))

	return hex.EncodeToString(h.Sum(nil))
}

// TestHashXxhashStringCompareImplementations compares two xxhash coding styles.
//
// Parameters:
// - t: the testing context.
//
// Returns:
// - none.
func TestHashXxhashStringCompareImplementations(t *testing.T) {
	t.Parallel()
	val := testhashraw

	gotCurrent := HashXxhashString(val)
	gotWrite := hashXxhashStringByWrite(val)
	expectedLegacy := hex.EncodeToString([]byte(val)) + hex.EncodeToString(xxhash.New().Sum(nil))

	require.Equal(t, gotWrite, gotCurrent)
	require.NotEqual(t, expectedLegacy, gotCurrent)
	require.Len(t, gotCurrent, 16)

	gotFromGenericHash, err := Hash(HashTypeXxhash, strings.NewReader(val))
	require.NoError(t, err)
	require.Equal(t, gotCurrent, hex.EncodeToString(gotFromGenericHash))
}

// TestSecurity42WeakHashWarnings checks process-wide budgets in fresh children,
// including concurrent first use and a reentrant logging hook. This isolates
// global logger changes and one-time state from all other tests.
func TestSecurity42WeakHashWarnings(t *testing.T) {
	mode := os.Getenv("GO_UTILS_SECURITY42_HASHER_MODE")
	if mode == "" {
		executable, err := os.Executable()
		require.NoError(t, err)
		for _, mode := range []string{"sequential", "concurrent", "reentrant"} {
			t.Run(mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				child := exec.CommandContext(ctx, executable, "-test.run=^TestSecurity42WeakHashWarnings$", "-test.timeout=8s")
				child.Env = append(os.Environ(), "GO_UTILS_SECURITY42_HASHER_MODE="+mode)
				output, err := child.CombinedOutput()
				require.NoError(t, ctx.Err(), "warning policy must not deadlock")
				require.NoError(t, err, string(output))
			})
		}
		return
	}
	logFile := filepath.Join(t.TempDir(), "hash.log")
	var hookEntered atomic.Bool
	logger, err := log.New(
		log.WithLevel(log.LevelWarn), log.WithEncoding(log.EncodingJSON),
		log.WithOutputPaths([]string{logFile}),
		log.WithZapOptions(zap.Hooks(func(entry zapcore.Entry) error {
			if mode == "reentrant" && strings.Contains(entry.Message, "sha1 is not safe") && hookEntered.CompareAndSwap(false, true) {
				_, err := HashTypeSha1.Hasher()
				return err
			}
			return nil
		})),
	)
	require.NoError(t, err)
	originalLogger := log.Shared
	log.Shared = logger
	defer func() { log.Shared = originalLogger }()
	if mode == "concurrent" {
		var workers sync.WaitGroup
		results := make(chan error, 64)
		for range 32 {
			workers.Go(func() {
				_, err := HashTypeSha1.Hasher()
				results <- err
				_, err = HashTypeMD5.Hasher()
				results <- err
			})
		}
		workers.Wait()
		close(results)
		for err := range results {
			require.NoError(t, err)
		}
	} else {
		for _, iterations := range []int{3, 31} {
			for range iterations {
				_, err = HashTypeSha1.Hasher()
				require.NoError(t, err)
				_, err = HashTypeMD5.Hasher()
				require.NoError(t, err)
			}
		}
	}
	for _, algorithm := range []HashType{HashTypeSha256, HashTypeSha512, HashTypeXxhash} {
		_, err := algorithm.Hasher()
		require.NoError(t, err)
	}
	_, err = HashType("unsupported").Hasher()
	require.Error(t, err)
	require.NoError(t, logger.Sync())
	raw, err := os.ReadFile(logFile)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(raw), "sha1 is not safe"))
	require.Equal(t, 1, strings.Count(string(raw), "md5 is not safe"))
	require.Len(t, strings.Split(strings.TrimSpace(string(raw)), "\n"), 2)
	if mode == "reentrant" {
		require.True(t, hookEntered.Load())
	}
}
