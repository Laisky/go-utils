package utils

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurity50Helper is a finite subprocess, never an unbounded producer.
func TestSecurity50Helper(t *testing.T) {
	mode := os.Getenv("GO_UTILS_CMD50_MODE")
	if mode == "" {
		return
	}
	if pidFile := os.Getenv("GO_UTILS_CMD50_PID"); pidFile != "" {
		require.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0600))
	}
	switch mode {
	case "capture":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, 768*1024))
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'y'}, 768*1024))
	case "stdout", "stderr", "both":
		var wg sync.WaitGroup
		for _, f := range []*os.File{os.Stdout, os.Stderr} {
			if mode == "stdout" && f == os.Stderr || mode == "stderr" && f == os.Stdout {
				continue
			}
			wg.Add(1)
			go func(f *os.File) { defer wg.Done(); _, _ = f.Write(bytes.Repeat([]byte{'x'}, 384*1024)) }(f)
		}
		wg.Wait()
	case "lines":
		_, _ = fmt.Fprint(os.Stdout, "one\r\ntwo\nlast")
	case "failure":
		_, _ = fmt.Fprint(os.Stderr, "SYNTHETIC_OUTPUT_SECRET")
		os.Exit(7)
	case "sized":
		n, err := strconv.Atoi(os.Getenv("GO_UTILS_CMD50_SIZE"))
		require.NoError(t, err)
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, n))
	case "sleep":
		time.Sleep(3 * time.Second)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	os.Exit(0)
}

// security50Child builds a helper invocation without a shell or external commands.
func security50Child(t *testing.T, mode string) (string, []string, []string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	return executable, []string{"-test.run=^TestSecurity50Helper$"}, append(os.Environ(), "GO_UTILS_CMD50_MODE="+mode, "GORACE=atexit_sleep_ms=0")
}

// TestSecurity50CaptureBound reproduces the unbounded default with finite output.
func TestSecurity50CaptureBound(t *testing.T) {
	exe, args, env := security50Child(t, "capture")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := RunCMDWithEnv(ctx, exe, args, env)
	require.Error(t, err)
	require.LessOrEqual(t, len(output), 1024*1024)
}

// TestSecurity50LineOverflow requires cancellation before the caller deadline.
func TestSecurity50LineOverflow(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr", "both"} {
		t.Run(stream, func(t *testing.T) {
			exe, args, env := security50Child(t, stream)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			start := time.Now()
			err := RunCMD2(ctx, exe, args, env, func(string) {}, func(string) {})
			elapsed := time.Since(start)
			require.Error(t, err)
			require.Less(t, elapsed, time.Second, "reader failure must not wait for context timeout")
		})
	}
}

// TestSecurity50ErrorDiagnostics keeps returned output separate from error text.
func TestSecurity50ErrorDiagnostics(t *testing.T) {
	exe, args, env := security50Child(t, "failure")
	args = append(args, "--", "SYNTHETIC_ARGUMENT_SECRET")
	output, err := RunCMDWithEnv(context.Background(), exe, args, env)
	require.Error(t, err)
	require.Contains(t, string(output), "SYNTHETIC_OUTPUT_SECRET")
	require.NotContains(t, fmt.Sprintf("%+v", err), "SYNTHETIC_OUTPUT_SECRET")
	require.NotContains(t, fmt.Sprintf("%+v", err), "SYNTHETIC_ARGUMENT_SECRET")
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 7, exit.ExitCode())
}

// TestSecurity50CallbacksJoined proves no callback remains active after return.
func TestSecurity50CallbacksJoined(t *testing.T) {
	exe, args, env := security50Child(t, "lines")
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	var lines []string
	go func() {
		done <- RunCMD2(context.Background(), exe, args, env, func(s string) {
			once.Do(func() { close(entered) })
			<-release
			lines = append(lines, s)
		}, func(string) {})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("callback did not start")
	}
	returnedEarly := false
	select {
	case <-done:
		returnedEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if returnedEarly {
		t.Fatal("RunCMD2 returned before its output callback completed")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("command did not finish")
	}
	require.Equal(t, []string{"one", "two", "last"}, lines)
}
