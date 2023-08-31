package utils

import (
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
)

/*
goos: linux
goarch: amd64
pkg: github.com/Laisky/go-utils
cpu: Intel(R) Core(TM) i7-4790 CPU @ 3.60GHz
Benchmark_excape/escape-8               67278787                17.30 ns/op            8 B/op          1 allocs/op
Benchmark_excape/not_escape-8           756616492                1.544 ns/op           0 B/op          0 allocs/op
Benchmark_excape/escape_str-8           39111782                35.15 ns/op           16 B/op          1 allocs/op
Benchmark_excape/not_escape_str-8       757973242                1.594 ns/op           0 B/op          0 allocs/op
*/
func Benchmark_excape(b *testing.B) {
	// case 1: int
	escapeint := func() *int {
		var x = 1
		return &x
	}
	notescapeint := func() int {
		x := new(int)
		*x = 1
		return *x
	}
	b.Run("escape", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			escapeint()
		}
	})
	b.Run("not escape", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			notescapeint()
		}
	})

	// case 2: string
	v := "~~~~~~~~~~~~~~~~~~~~Hello, World~~~~~~~~~~~~~~~~~~~~"
	escapestr := func() *string { // 34.75 ns/op
		var x = v // moved to heap: x
		return &x
	}
	notescapestr := func() string { // 1.558 ns/op
		x := new(string) // new(string) does not escape
		*x = v
		return *x
	}
	b.Run("escape str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			escapestr()
		}
	})
	b.Run("not escape str", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			notescapestr()
		}
	})

	// case 3: struct
	type tt struct {
		v string
	}
	escapestruct := func() *tt {
		x := &tt{v}
		return x
	}
	notescapestruct := func() tt {
		x := &tt{v: v}
		return *x
	}
	b.Run("escape struct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			escapestruct()
		}
	})
	b.Run("not escape struct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			notescapestruct()
		}
	})
}

func Test_DeferErr(t *testing.T) {
	doSomeCleanning := func() error { return nil }

	// expect return io.EOF, actual return nil
	foo := func() (err error) {
		defer func() {
			// bug: returned error has been shadowed to nil
			if err = doSomeCleanning(); err != nil {
				log.Printf("got err: %+v", err)
			}
		}()

		return io.EOF
	}

	require.NoError(t, foo())
}
