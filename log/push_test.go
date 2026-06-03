package log

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

func TestPusherHTTPSender_Send(t *testing.T) {
	ctx := context.Background()

	// run http server for test
	var got string
	wait := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %+v", err)
		}
		got = string(body)
		wait <- struct{}{}
	}))
	defer srv.Close()

	sender := NewPusherHTTPSender(
		srv.Client(),
		srv.URL,
		map[string]string{"content-type": "application/json"},
	)

	p, err := NewPusher(ctx,
		WithPusherSender(sender),
	)
	require.NoError(t, err)

	logger := Shared.Named("test")
	logger = logger.WithOptions(zap.HooksWithFields(p.GetZapHook()))
	logger.Info("slava, ukriane")

	select {
	case <-wait:
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive payload in time")
	}
	// "{\"level\":\"info\",\"time\":\"2023-06-04T07:45:44.227Z\",\"logger\":\"go-utils.test\",\"caller\":\"log/push_test.go:46\",\"msg\":\"test\"}\n"
	require.Contains(t, got, "slava, ukriane")
}

// TestPusher_HookReturnsAfterCtxCancel verifies the zap hook does not block
// forever once the ctx passed to NewPusher is cancelled. With the default
// unbuffered senderChan the old bare send deadlocked because the sender
// goroutine had already returned and nothing drained the channel.
func TestPusher_HookReturnsAfterCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	p, err := NewPusher(ctx)
	require.NoError(t, err)

	// Cancel the ctx so the sender goroutine returns; nothing drains
	// senderChan after this point.
	cancel()
	// Give the sender goroutine a moment to observe cancellation and exit.
	time.Sleep(100 * time.Millisecond)

	hook := p.GetZapHook()

	done := make(chan struct{})
	go func() {
		_ = hook(zapcore.Entry{Message: "after cancel"}, nil)
		close(done)
	}()

	select {
	case <-done:
		// hook returned without blocking, as expected.
	case <-time.After(2 * time.Second):
		t.Fatal("zap hook blocked after ctx cancel")
	}
}
