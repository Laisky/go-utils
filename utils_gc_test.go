package utils

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-utils/v6/log"
)

// TestTriggerGC verifies that TriggerGC and ForceGC can be called directly without panicking.
func TestTriggerGC(t *testing.T) {
	TriggerGC()
	ForceGC()
}

// TestAutoGC verifies that AutoGC starts successfully with an 85% memory ratio and a readable memory-limit file,
// and that it returns an error for memory ratios of -1, 0, or 101 and for a nonexistent memory-limit file path.
func TestAutoGC(t *testing.T) {
	t.Parallel()

	var err error
	if err = log.Shared.ChangeLevel("debug"); err != nil {
		t.Fatalf("%+v", err)
	}

	var fp *os.File
	if fp, err = os.CreateTemp("", "test-gc*"); err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()

	if _, err = fp.WriteString("123456789"); err != nil {
		t.Fatalf("%+v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = AutoGC(ctx,
		WithGCMemRatio(85),
		WithGCMemLimitFilePath(fp.Name()),
	)
	require.NoError(t, err)
	<-ctx.Done()
	// t.Error()

	// case: test err arguments
	{
		err = AutoGC(ctx, WithGCMemRatio(-1))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemRatio(0))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemRatio(101))
		require.Error(t, err)

		err = AutoGC(ctx, WithGCMemLimitFilePath(RandomStringWithLength(100)))
		require.Error(t, err)
	}
}

// ExampleAutoGC demonstrates enabling AutoGC for one second with the default 85% memory ratio and the default
// cgroup memory-limit file path, logging any setup error.
func ExampleAutoGC() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := AutoGC(
		ctx,
		WithGCMemRatio(85), // default
		WithGCMemLimitFilePath("/sys/fs/cgroup/memory/memory.limit_in_bytes"), // default
	); err != nil {
		log.Shared.Error("enable autogc", zap.Error(err))
	}
}

// TestForceGCBlocking verifies that ForceGCBlocking runs a blocking garbage collection without panicking.
func TestForceGCBlocking(t *testing.T) {
	t.Parallel()

	ForceGCBlocking()
}

// ExampleForceGCBlocking demonstrates running a blocking garbage collection that also returns freed memory to
// the operating system.
func ExampleForceGCBlocking() {
	ForceGCBlocking()
}

// ExampleForceGCUnBlocking demonstrates triggering a garbage collection in the background without blocking the
// caller.
func ExampleForceGCUnBlocking() {
	ForceGCUnBlocking()
}

// TestForceGCUnBlocking verifies that ForceGCUnBlocking returns without blocking and is safe to call from 1000
// concurrent goroutines.
func TestForceGCUnBlocking(t *testing.T) {
	t.Parallel()

	ForceGCUnBlocking()

	var pool errgroup.Group
	for i := 0; i < 1000; i++ {
		pool.Go(func() error {
			ForceGCUnBlocking()
			return nil
		})
	}

	require.NoError(t, pool.Wait())
}
