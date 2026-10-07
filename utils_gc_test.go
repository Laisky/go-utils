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

func TestTriggerGC(t *testing.T) {
	TriggerGC()
	ForceGC()
}

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

func TestForceGCBlocking(t *testing.T) {
	t.Parallel()

	ForceGCBlocking()
}

func ExampleForceGCBlocking() {
	ForceGCBlocking()
}

func ExampleForceGCUnBlocking() {
	ForceGCUnBlocking()
}

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
