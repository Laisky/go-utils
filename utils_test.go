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

func ExampleFallBack() {
	targetFunc := func() any {
		panic("someting wrong")
	}

	FallBack(targetFunc, 10) // got 10
}

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

func (f *testCloseQuitlyStruct) Close() error {
	return nil
}

func TestSilentClose(t *testing.T) {

	f := new(testCloseQuitlyStruct)
	SilentClose(f)
}

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

func TestCostSecs(t *testing.T) {
	d := time.Millisecond * 351
	v := CostSecs(d)
	require.Equal(t, "0.35s", v)
}

func TestPipeline(t *testing.T) {
	f1 := func(v *int) error { (*v)++; return nil }
	f2 := func(v *int) error { (*v) += 2; return nil }
	v := 0

	gotv, err := Pipeline([]func(*int) error{f1, f2}, &v)
	require.NoError(t, err)
	require.Equal(t, 3, v)
	require.Equal(t, 3, *gotv)
}

func Test_singleflight(t *testing.T) {
	var n int
	key := RandomStringWithLength(10)
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	internalSFG.Do(key, func() (interface{}, error) { n++; return nil, nil })
	require.Equal(t, 3, n)
}

func TestDelayer_Wait(t *testing.T) {
	startAt := time.Now()
	delay := 10 * time.Millisecond

	func() {
		defer NewDelay(delay).Wait()

		require.Less(t, time.Since(startAt), time.Millisecond)
	}()
	require.GreaterOrEqual(t, time.Since(startAt), delay)
}

func ExampleNewDelay() {
	startAt := time.Now()
	delay := 10 * time.Millisecond

	func() {
		defer NewDelay(delay).Wait()
	}()

	fmt.Println(time.Since(startAt) >= delay)
	// Output: true
}

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
