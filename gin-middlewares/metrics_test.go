package middlewares

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type urlCase struct {
	path, resp string
}

func TestMetricsSrv(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := "127.0.0.1:48192"

	srv, err := GetHTTPMetricSrv(
		ctx,
		WithMetricAddr(addr),
		WithPprofPath("/pprof"),
		WithMetricGraceWait(1*time.Second),
	)
	if err != nil {
		t.Fatalf("%+v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.ListenAndServe(); err != nil {
			t.Logf("%+v", err)
		}
	}()
	time.Sleep(1 * time.Second) // wait server start

	for _, tcase := range []*urlCase{
		{
			path: "/pprof",
			resp: "CPU profile. You can specify the duration",
		},
		{
			path: "/metrics",
			resp: "go_gc_duration_seconds",
		},
	} {
		resp, err := http.Get("http://" + addr + tcase.path)
		if err != nil {
			t.Fatalf("request: %+v", err)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %+v", err)
		}
		defer resp.Body.Close()
		t.Logf("resp: %+v", string(body))
		if !strings.Contains(string(body), tcase.resp) {
			t.Fatalf("should contains `%v` in return", tcase.resp)
		}
	}

	cancel()
	wg.Wait()
}
