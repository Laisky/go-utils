// Package utils 一些常用工具
package utils

import (
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// GetFuncName return the name of func
func GetFuncName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// FallBack return the fallback when orig got error
// utils.FallBack(func() interface{} { return getIOStatMetric(fs) }, &IOStat{}).(*IOStat)
func FallBack(orig func() interface{}, fallback interface{}) (ret interface{}) {
	defer func() {
		if recover() != nil {
			ret = fallback
		}
	}()

	ret = orig()
	return
}

// RegexNamedSubMatch extract key:val map from string by group match
func RegexNamedSubMatch(r *regexp.Regexp, str string, subMatchMap map[string]string) error {
	match := r.FindStringSubmatch(str)
	names := r.SubexpNames()
	if len(names) != len(match) {
		return errors.New("the number of args in `regexp` and `str` not matched")
	}

	for i, name := range r.SubexpNames() {
		if i != 0 && name != "" {
			subMatchMap[name] = match[i]
		}
	}
	return nil
}

// FlattenMap make embedded map into flatten map
func FlattenMap(data map[string]interface{}, delimiter string) {
	for k, vi := range data {
		if v2i, ok := vi.(map[string]interface{}); ok {
			FlattenMap(v2i, delimiter)
			for k3, v3i := range v2i {
				data[k+delimiter+k3] = v3i
			}
			delete(data, k)
		}
	}
}

// ForceGCSignalC send signal to trigger manual gc
var forceGCSignalC = make(chan struct{})

// ForceGC force to run blocking manual gc.
func ForceGC() {
	Logger.Info("force gc")
	runtime.GC()
	debug.FreeOSMemory()
}

// TriggerGC trigger GC unblocking
func TriggerGC() {
	select {
	case forceGCSignalC <- struct{}{}:
	default:
	}
}

func init() {
	go func() {
		defer fmt.Println("manual gc trigger quit")
		for range forceGCSignalC {
			ForceGC()
		}
	}()
}
