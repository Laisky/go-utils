package utils_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func ExampleConfigSrv() {
	var (
		url     = "http://config-server.un.org"
		app     = "appname"
		profile = "sit"
		label   = "master"
	)

	c := utils.NewConfigSrv(url, app, profile, label)
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

func RunMockConfigSrv(port int, fakadata interface{}) {
	httpsrv := gin.New()

	httpsrv.GET("/app/profile/label", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, fakadata)
	})

	// run mock config-server
	addr := fmt.Sprintf("localhost:%v", port)
	utils.Logger.Debug("run config-server", zap.String("addr", addr))
	if err := httpsrv.Run(addr); err != nil {
		utils.Logger.Panic("try to run server got error", zap.Error(err))
	}
}

func TestConfigSrv(t *testing.T) {
	// jb, err := json.Marshal(fakeConfigSrvData)
	// if err != nil {
	// 	utils.Logger.Panic("try to marshal fake data got error", zap.Error(err))
	// }

	port := 24951
	addr := fmt.Sprintf("http://localhost:%v", port)
	go RunMockConfigSrv(port, fakeConfigSrvData)
	time.Sleep(100 * time.Millisecond)

	var (
		profile = "profile"
		label   = "label"
		app     = "app"
		name    = "app"
	)

	c := utils.NewConfigSrv(addr, app, profile, label)
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
