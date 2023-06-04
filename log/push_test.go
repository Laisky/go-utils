package log

import (
	"context"
	"net/http"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

func TestPusherHTTPSender_Send(t *testing.T) {
	ctx := context.Background()

	// run http server for test
	var got string
	var wait = make(chan struct{})
	srv := &http.Server{
		Addr: ":18082",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { wait <- struct{}{} }()
			w.WriteHeader(200)
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			got = string(body)
		}),
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(ctx)

	sender := NewPusherHTTPSender(
		&http.Client{},
		"http://0.0.0.0:18082",
		map[string]string{"content-type": "application/json"},
	)

	p, err := NewPusher(ctx,
		WithPusherSender(sender),
	)
	require.NoError(t, err)

	logger := Shared.Named("test")
	logger = logger.WithOptions(zap.HooksWithFields(p.GetZapHook()))
	logger.Info("slava, ukriane")

	<-wait
	// "{\"level\":\"info\",\"time\":\"2023-06-04T07:45:44.227Z\",\"logger\":\"go-utils.test\",\"caller\":\"log/push_test.go:46\",\"msg\":\"test\"}\n"
	require.Contains(t, got, "slava, ukriane")
}
