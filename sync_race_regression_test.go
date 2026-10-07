package utils

import (
	"context"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// errRaceFirst is the error returned by the fastest function in race tests.
var errRaceFirst = errors.New("first finisher")

// TestRaceErrReturnsFirstResultWithoutLostWakeup verifies that RaceErr returns
// the first finisher's result immediately even when that function completes
// before RaceErr starts waiting. The condition-variable implementation lost
// such a wakeup (no predicate around cond.Wait) and then blocked until a
// slower function finished.
func TestRaceErrReturnsFirstResultWithoutLostWakeup(t *testing.T) {
	t.Parallel()
	// The fast function goes first and is followed by many slow ones, so it
	// finishes while RaceErr is still spawning goroutines, before it waits.
	slow := func() error {
		time.Sleep(3 * time.Second)
		return nil
	}
	gs := []func() error{func() error { return errRaceFirst }}
	for range 2000 {
		gs = append(gs, slow)
	}

	for range 3 {
		start := time.Now()
		err := RaceErr(gs...)
		require.ErrorIs(t, err, errRaceFirst)
		require.Less(t, time.Since(start), time.Second, "RaceErr missed the first finisher")
	}
}

// TestRaceFunctionsWithoutInputsDoNotBlock verifies that RaceErr and
// RaceErrWithCtx return instead of blocking forever when given no functions.
func TestRaceFunctionsWithoutInputsDoNotBlock(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		require.Error(t, RaceErr())
		require.Error(t, RaceErrWithCtx(context.Background()))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("racing zero functions blocked forever")
	}
}

// TestRaceErrWithCtxCanceledContextDoesNotBlock verifies that RaceErrWithCtx
// returns the context error instead of blocking forever when the context is
// already canceled: the racing goroutines then exit without sending a result.
func TestRaceErrWithCtxCanceledContextDoesNotBlock(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- RaceErrWithCtx(ctx, func(context.Context) error { return nil })
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("RaceErrWithCtx blocked forever on a canceled context")
	}
}
