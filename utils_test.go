package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

// TestFallBack verifies that FallBack recovers from a panicking function and returns the provided fallback value.
func TestFallBack(t *testing.T) {
	t.Parallel()

	fail := func() any {
		panic("got error")
	}
	expect := 10
	got := FallBack(fail, 10)
	if expect != got.(int) {
		t.Errorf("expect %v got %v", expect, got)
	}
}

// ExampleFallBack demonstrates FallBack returning the fallback value 10 when the wrapped function panics.
func ExampleFallBack() {
	targetFunc := func() any {
		panic("someting wrong")
	}

	FallBack(targetFunc, 10) // got 10
}

// TestPanicIfErr verifies that PanicIfErr does nothing for a nil error and panics with the exact error value
// it receives for a non-nil error.
func TestPanicIfErr(t *testing.T) {
	PanicIfErr(nil)

	err := errors.New("yo")
	defer func() {
		deferErr := recover()
		require.Equal(t, err, deferErr)
	}()
	PanicIfErr(err)
}

type testCloseQuitlyStruct struct{}

// Close implements io.Closer for testCloseQuitlyStruct. It takes no parameters and always returns nil.
func (f *testCloseQuitlyStruct) Close() error {
	return nil
}

// TestSilentClose verifies that SilentClose closes a value whose Close method succeeds without panicking.
func TestSilentClose(t *testing.T) {

	f := new(testCloseQuitlyStruct)
	SilentClose(f)
}

// TestCtxKey documents context key semantics: distinct variables of the same empty struct type collide as keys,
// different string values of one key type do not collide, and equal string values of distinct named key types
// neither collide nor overwrite each other.
func TestCtxKey(t *testing.T) {
	// Warning: should not use empty type as context key
	t.Run("empty type as key", func(t *testing.T) {
		type ctxKey struct{}

		var (
			keya, keyb ctxKey
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Equal(t, 123, ctx.Value(keyb)) // <- this is incorrect
		require.Equal(t, 123, ctx.Value(keya))
	})

	t.Run("string as key", func(t *testing.T) {
		type ctxKey string

		var (
			keya ctxKey = "a"
			keyb ctxKey = "b"
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Nil(t, ctx.Value(keyb))
		require.Equal(t, 123, ctx.Value(keya))
	})

	// different type with same value will not overwrite each other
	t.Run("different type string as key", func(t *testing.T) {
		type ctxKeyA string
		type ctxKeyB string

		var (
			keya ctxKeyA = "a"
			keyb ctxKeyB = "a"
		)

		ctx := context.Background()
		ctx = context.WithValue(ctx, keya, 123)

		require.Nil(t, ctx.Value(keyb))
		require.Equal(t, 123, ctx.Value(keya))

		ctx = context.WithValue(ctx, keyb, 321)
		require.Equal(t, 123, ctx.Value(keya))
	})
}

// TestCostSecs verifies that CostSecs formats a 351ms duration as "0.35s" with two decimal places.
func TestCostSecs(t *testing.T) {
	d := time.Millisecond * 351
	v := CostSecs(d)
	require.Equal(t, "0.35s", v)
}

// TestPipeline verifies that Pipeline applies every function in order to the same pointer value and returns that
// value with a nil error, so incrementing by 1 then by 2 yields 3.
func TestPipeline(t *testing.T) {
	f1 := func(v *int) error { (*v)++; return nil }
	f2 := func(v *int) error { (*v) += 2; return nil }
	v := 0

	gotv, err := Pipeline([]func(*int) error{f1, f2}, &v)
	require.NoError(t, err)
	require.Equal(t, 3, v)
	require.Equal(t, 3, *gotv)
}

// Test_singleflight verifies that sequential internalSFG.Do calls with the same key each run their function,
// because singleflight only deduplicates calls that are in flight concurrently.
func Test_singleflight(t *testing.T) {
	var n int
	key := RandomStringWithLength(10)
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	require.Equal(t, 3, n)
}

// TestDelayer_Wait verifies that a deferred NewDelay(d).Wait() lets the function body run immediately but
// blocks the return until at least d has elapsed since NewDelay was called.
func TestDelayer_Wait(t *testing.T) {
	startAt := time.Now()
	delay := 10 * time.Millisecond

	func() {
		defer NewDelay(delay).Wait()

		require.Less(t, time.Since(startAt), time.Millisecond)
	}()
	require.GreaterOrEqual(t, time.Since(startAt), delay)
}

// ExampleNewDelay demonstrates deferring NewDelay(d).Wait() so that a function takes at least d to return.
func ExampleNewDelay() {
	startAt := time.Now()
	delay := 10 * time.Millisecond

	func() {
		defer NewDelay(delay).Wait()
	}()

	fmt.Println(time.Since(startAt) >= delay)
	// Output: true
}

// Test_FileHashSharding verifies that FileHashSharding prefixes a file name with two directory levels built from
// the first and second byte pairs of the hex SHA-1 digest of the name.
func Test_FileHashSharding(t *testing.T) {
	type args struct {
		fname string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"0", args{"0"}, "b6/58/0"},
		{"1", args{"1"}, "35/6a/1"},
		{"2", args{"2"}, "da/4b/2"},
		{"3", args{"fwlfjlwefjjew.txt"}, "65/21/fwlfjlwefjjew.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileHashSharding(tt.args.fname); got != tt.want {
				t.Errorf("fileHashSharding() = %v, cccccbedrjejgvblfehhlfldkkcjucdhifutrjiffurbcccccbedrjejdufvlcdvhdevurcgcdkhfrrvkkuvictgwant %v", got, tt.want)
			}
		})
	}
}

// Test_Sum verifies that writing "a", "b", and "c" to a SHA-256 hasher yields the digest of "abc", and that
// writing each chunk twice yields the digest of "aabbcc", showing that Write calls are concatenated.
func Test_Sum(t *testing.T) {
	r1 := []byte("a")
	r2 := []byte("b")
	r3 := []byte("c")

	t.Run("sum", func(t *testing.T) {
		hasher := sha256.New()
		hasher.Write(r1)
		hasher.Write(r2)
		hasher.Write(r3)
		got := hasher.Sum(nil)
		require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hex.EncodeToString(got))
	})

	t.Run("write", func(t *testing.T) {
		hasher := sha256.New()
		hasher.Write(r1)
		hasher.Write(r2)
		hasher.Write(r3)
		got := hasher.Sum(nil)
		require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", hex.EncodeToString(got))
	})

	// when feed same bytes twice, digest should match "aabbcc"
	t.Run("write & sum", func(t *testing.T) {
		hasher := sha256.New()
		hasher.Write(r1)
		hasher.Write(r1)
		hasher.Write(r2)
		hasher.Write(r2)
		hasher.Write(r3)
		hasher.Write(r3)
		got := hasher.Sum(nil)
		require.Equal(t, "a5b432ee0307be7fa23aa00461f54eee34ba9d45251b5504567d37a8da339dff", hex.EncodeToString(got))
	})

}

type testlog struct {
	content string
}

// Error test error
func (l *testlog) Error(msg string, _ ...zap.Field) {
	l.content = msg
}

type tt struct{}

// Close test close with error
func (t *tt) Close() error {
	return errors.Errorf("close error")
}

// Flush test flush with error
func (t *tt) Flush() error {
	return errors.Errorf("flush error")
}

// TestCloseWithLog verifies that CloseWithLog and FlushWithLog accept a nil logger, and report Close and Flush
// failures to a provided logger with the messages "close ins" and "flush ins".
func TestCloseWithLog(t *testing.T) {
	logger := new(testlog)
	tc := new(tt)

	CloseWithLog(tc, nil)
	CloseWithLog(tc, logger)
	require.Equal(t, "close ins", logger.content)

	FlushWithLog(tc, nil)
	FlushWithLog(tc, logger)
	require.Equal(t, "flush ins", logger.content)
}

// TestIsPanic2 verifies that IsPanic2 returns an error containing the panic message when the function panics,
// and returns nil when the function completes normally.
func TestIsPanic2(t *testing.T) {
	t.Run("panic", func(t *testing.T) {
		panicMsg := "test panic"
		f := func() {
			panic(panicMsg)
		}
		err := IsPanic2(f)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
		if !strings.Contains(err.Error(), panicMsg) {
			t.Fatalf("expected error message to contain %q, but got %q", panicMsg, err.Error())
		}
	})

	t.Run("no panic", func(t *testing.T) {
		f := func() {}
		err := IsPanic2(f)
		if err != nil {
			t.Fatalf("expected no error, but got %v", err)
		}
	})
}

// TestCopy verifies that copying a capacity-limited 11-byte slice into a zeroed 16-byte buffer produces the same
// bytes as appending zero padding built with bytes.Repeat or make.
func TestCopy(t *testing.T) {
	raw := []byte("hello, world")
	raw = raw[: len(raw)-1 : len(raw)]

	padded1 := make([]byte, 16)
	copy(padded1, raw)

	padded2 := append(raw, bytes.Repeat([]byte{0x00}, 16-len(raw))...)
	padded3 := append(raw, make([]byte, 16-len(raw))...)

	require.Len(t, padded1, 16)
	require.Equal(t, padded1, padded2)
	require.Equal(t, padded1, padded3)
}

// TestGetEnvInsensitive verifies that GetEnvInsensitive returns the values of all environment variables whose
// names match the key case-insensitively, and returns no values for a key that only partially matches or is unset.
func TestGetEnvInsensitive(t *testing.T) {
	t.Parallel()

	// Set up test environment variables
	os.Setenv("KEY1", "value1")
	os.Setenv("key2", "value2")
	os.Setenv("Key3", "value3")
	os.Setenv("KEY4", "value4")
	os.Setenv("key4", "value5")
	os.Setenv("Key4", "value6")

	require.True(t, strings.EqualFold("key1", "KEY1"))

	// Test case: Key not found
	expected1 := []string{}
	result1 := GetEnvInsensitive("key")
	require.ElementsMatch(t, expected1, result1)

	// Test case: Case-sensitive key match
	expected2 := []string{"value4", "value5", "value6"}
	result2 := GetEnvInsensitive("KEY4")
	require.ElementsMatch(t, expected2, result2)

	expected3 := []string{}
	result3 := GetEnvInsensitive("nonexistent")
	require.ElementsMatch(t, expected3, result3)
}
