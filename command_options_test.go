package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity50CaptureEdges tests below, at, above, and explicitly raised budgets.
func TestSecurity50CaptureEdges(t *testing.T) {
	for _, n := range []int{0, 1, 31, 32, 33, 127} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			exe, args, env := security50Child(t, "sized")
			env = append(env, fmt.Sprintf("GO_UTILS_CMD50_SIZE=%d", n))
			output, err := RunCMDWithOptions(context.Background(), exe, args, env, CMDOptions{MaxOutputBytes: 32})
			require.Equal(t, strings.Repeat("x", min(n, 32)), string(output))
			if n > 32 {
				require.ErrorIs(t, err, ErrCMDOutputLimit)
			} else {
				require.NoError(t, err)
			}
			output, err = RunCMDWithOptions(context.Background(), exe, args, env, CMDOptions{MaxOutputBytes: 128})
			require.NoError(t, err)
			require.Len(t, output, n)
		})
	}
}

// TestSecurity50FramingEdges checks CRLF, final lines, ownership and exact boundaries.
func TestSecurity50FramingEdges(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 9} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := &cmdBudget{remaining: 64, cancel: cancel}
			var got []string
			w := &cmdLines{state: b, limit: 8, handler: func(s string) { got = append(got, s) }}
			input := strings.Repeat("x", n)
			_, err := w.Write([]byte(input + "\n"))
			if n > 8 {
				require.ErrorIs(t, err, ErrCMDLineLimit)
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				return
			}
			require.NoError(t, err)
			_, err = w.Write([]byte("z\r\nlast"))
			require.NoError(t, err)
			w.flush()
			require.Equal(t, []string{input, "z", "last"}, got)
		})
	}
}

// TestSecurity50ValidationRejectsBeforeStart preserves cancellation and rejects invalid limits.
func TestSecurity50ValidationRejectsBeforeStart(t *testing.T) {
	for _, opts := range []CMDOptions{{MaxOutputBytes: -1}, {MaxLineBytes: -1}, {WaitDelay: -1}} {
		_, err := RunCMDWithOptions(context.Background(), "does-not-exist", nil, nil, opts)
		require.ErrorContains(t, err, "limits")
	}
	_, err := RunCMD(nil, "does-not-exist")
	require.ErrorContains(t, err, "context")
	exe, args, env := security50Child(t, "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = RunCMDWithEnv(ctx, exe, args, env)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
}

// TestSecurity50SharedBudget enforces one cap across separate output streams.
func TestSecurity50SharedBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := &cmdBudget{remaining: 6, cancel: cancel}
	out := &cmdLines{state: state, limit: 16, handler: func(string) {}}
	errout := &cmdLines{state: state, limit: 16, handler: func(string) {}}
	n, err := out.Write([]byte("abc\n"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	n, err = errout.Write([]byte("def\n"))
	require.ErrorIs(t, err, ErrCMDOutputLimit)
	require.Equal(t, 2, n)
	n, err = out.Write([]byte("x"))
	require.ErrorIs(t, err, ErrCMDOutputLimit)
	require.Zero(t, n)
	require.True(t, errors.Is(ctx.Err(), context.Canceled))
}
