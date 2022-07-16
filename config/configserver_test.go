package config

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v2"
	"github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
)

func ExampleConfigSrv() {
	var (
		url     = "http://config-server.un.org"
		app     = "appname"
		profile = "sit"
		label   = "master"
	)

	c := NewConfigSrv(url, app, profile, label)
	c.Get("management.context-path")
	c.GetString("management.context-path")
	c.GetBool("endpoints.health.sensitive")
	c.GetInt("spring.cloud.config.retry")
}

var fakeConfigSrvData = map[string]interface{}{
	"name":     "app",
	"profiles": []string{"profile"},
	"label":    "label",
	"version":  "12345",
	"propertySources": []map[string]interface{}{
		{
			"name": "config name",
			"source": map[string]string{
				"profile": "profile",
				"key1":    "abc",
				"key2":    "123",
				"key3":    "true",
			},
		},
	},
}

func fakeHandler(data interface{}) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		d, err := gutils.JSON.Marshal(data)
		if err != nil {
			log.Shared.Panic("marashal fake config")
		}

		if _, err := w.Write(d); err != nil {
			log.Shared.Panic("write http response")
		}
	}
}

func runMockHTTPServer(ctx context.Context, port int, path string, fakadata interface{}) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Shared.Panic("listen", zap.Error(err))
	}

	go func() {
		defer ln.Close()
		<-ctx.Done()
	}()

	mux := http.NewServeMux()
	mux.HandleFunc(path, fakeHandler(fakadata))

	// srv.HandleFunc("/app/profile/label", fakeHandler func)
	if err = http.Serve(ln, mux); err != nil {
		log.Shared.Error("http server exit", zap.Error(err))
	}
}

func TestConfigSrv(t *testing.T) {
	// jb, err := gutils..Marshal(fakeConfigSrvData)
	// if err != nil {
	// 	log.Shared.Panic("try to marshal fake data got error", zap.Error(err))
	// }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := 24951
	addr := fmt.Sprintf("http://localhost:%v", port)
	go runMockHTTPServer(ctx, port, "/app/profile/label", fakeConfigSrvData)
	time.Sleep(100 * time.Millisecond)

	var (
		profile = "profile"
		label   = "label"
		app     = "app"
		name    = "app"
	)

	c := NewConfigSrv(addr, app, profile, label)
	if err := c.Fetch(); err != nil {
		t.Fatalf("init ConfigSrv got error: %+v", err)
	}

	t.Logf("got cfg name: %v", c.RemoteCfg.Name)
	t.Logf("got cfg profile: %v", c.RemoteCfg.Profiles[0])
	t.Logf("got cfg source name: %v", c.RemoteCfg.Sources[0].Name)

	if c.RemoteCfg.Name != name {
		t.Fatalf("cfg name error")
	}

	// check interface
	if val, ok := c.Get("key1"); !ok {
		t.Fatal("need to check whether contains `k1`")
	} else if val.(string) != "abc" {
		t.Fatal("`k1` should equal to `abc`")
	}

	// check int
	if val, ok := c.GetInt("key2"); !ok {
		t.Fatalf("need to check whether contains `key2, but got %v", val)
	} else if val != 123 {
		t.Fatalf("`key2` should equal to `123`, but got %v", val)
	}

	// check string
	if val, ok := c.GetString("key1"); !ok {
		t.Fatal("need to check whether contains `key1`")
	} else if val != "abc" {
		t.Fatal("`key1` should equal to `abc`")
	}

	// check bool
	if val, ok := c.GetBool("key3"); !ok { // "true"
		t.Fatal("need to check whether contains `key3`")
	} else if val != true {
		t.Fatal("`key3` should equal to `true`")
	}
}
