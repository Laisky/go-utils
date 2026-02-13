package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Laisky/zap"
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
