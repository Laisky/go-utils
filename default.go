// Package utils 一些常用工具
package utils

import (
	"bytes"
	"html/template"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/astaxie/beego"
	log "github.com/cihub/seelog"
)

// SetupLogger 初始化日志
func SetupLogger(logLevel string) {
	args := struct {
		LogLevel string
	}{
		LogLevel: logLevel,
	}
	logConfig := `
		<seelog type="asynctimer" asyncinterval="1000000" minlevel="{{.LogLevel}}" maxlevel="error">
			<exceptions>
				<exception funcpattern="*main.test*Something*" minlevel="{{.LogLevel}}"/>
				<exception filepattern="*main.go" minlevel="{{.LogLevel}}"/>
			</exceptions>
			<outputs formatid="main">
				<console/>  <!-- 输出到控制台 -->
			</outputs>
			<formats>
				<format id="main" format="[%UTCDate(2006-01-02T15:04:05.000000Z) - %LEVEL - %RelFile:%Line] %Msg%n"/>
			</formats>
		</seelog>
	`
	tmpl, err := template.New("seelogConfig").Parse(logConfig)
	if err != nil {
		panic(err.Error())
	}
	var configBytes bytes.Buffer
	if err := tmpl.Execute(&configBytes, args); err != nil {
		panic(err.Error())
	}
	logger, err := log.LoggerFromConfigAsBytes(configBytes.Bytes())
	if err != nil {
		panic(err.Error())
	}
	logger.Info("SetupLogger ok")
	log.ReplaceLogger(logger)
}

// GetRunmode 获取运行模式
func GetRunmode() string {
	Runmode := os.Getenv("DOCKERKIT_RUNMODE")
	if Runmode == "" {
		Runmode = beego.AppConfig.String("runmode")
	}
	if Runmode == "" {
		Runmode = "dev"
	}

	Runmode = strings.ToLower(Runmode)
	return Runmode
}

// GetFuncName return the name of func
func GetFuncName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// FallBack return the fallback when orig got error
func FallBack(orig func() interface{}, fallback interface{}) (ret interface{}) {
	defer func() {
		if recover() != nil {
			ret = fallback
		}
	}()

	ret = orig()
	return
}
