package spring_test

import (
	"testing"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/spring"
)

func ExampleConfigSrv() {
	var (
		url     = "http://config-server.paas.ptcloud.t.home"
		app     = "dbdevice"
		profile = "sit"
		label   = "master"
	)

	c := spring.NewConfigSrv(url, profile, label, app)
	c.Get("management.context-path")
	c.GetString("management.context-path")
	c.GetBool("endpoints.health.sensitive")
	c.GetInt("spring.cloud.config.retry")
}

func TestConfigSrv(t *testing.T) {
	var (
		url     = "http://config-server.paas.ptcloud.t.home"
		app     = "dbdevice"
		profile = "sit"
		label   = "master"
	)

	c := spring.NewConfigSrv(url, profile, label, app)
	if err := c.Fetch(); err != nil {
		t.Fatalf("init ConfigSrv got error: %+v", err)
	}

	t.Logf("got cfg name: %v", c.Cfg.Name)
	t.Logf("got cfg profile: %v", c.Cfg.Profiles[0])
	t.Logf("got cfg source name: %v", c.Cfg.Sources[0].Name)

	if c.Cfg.Name != "dbdevice" {
		t.Fatalf("cfg name error")
	}

	// check interface
	if val, ok := c.Get("spring.data.rest.basePath"); !ok {
		t.Fatal("need to check whether contains `spring.data.rest.basePath`")
	} else if val.(string) != "/api" {
		t.Fatal("`spring.data.rest.basePath` should equal to `/api`")
	}

	// check int
	if val, ok := c.GetInt("hystrix.command.default.execution.isolation.thread.timeoutInMilliseconds"); !ok {
		t.Fatal("need to check whether contains `hystrix.command.default.execution.isolation.thread.timeoutInMilliseconds`")
	} else if val != 10000 {
		t.Fatal("`hystrix.command.default.execution.isolation.thread.timeoutInMilliseconds` should equal to `10000`")
	}

	// check string
	if val, ok := c.GetString("management.context-path"); !ok {
		t.Fatal("need to check whether contains `management.context-path`")
	} else if val != "/admin" {
		t.Fatal("`management.context-path` should equal to `/admin`")
	}

	// check bool
	if val, ok := c.GetBool("spring.cloud.config.failFast"); !ok { // "true"
		t.Fatal("need to check whether contains `spring.cloud.config.failFast`")
	} else if val != true {
		t.Fatal("`spring.cloud.config.failFast` should equal to `true`")
	}
	if val, ok := c.GetBool("eureka.instance.preferIpAddress"); !ok { // true
		t.Fatal("need to check whether contains `eureka.instance.preferIpAddress`")
	} else if val != true {
		t.Fatal("`eureka.instance.preferIpAddress` should equal to `true`")
	}

}

func init() {
	utils.SetupLogger("debug")
}
