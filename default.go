// Package utils 一些常用工具
package utils

import (
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/astaxie/beego"
)

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
