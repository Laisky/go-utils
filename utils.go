// Package utils 一些常用工具
package utils

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"

	"github.com/Laisky/zap"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const (
	defaultCgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	defaultGCMemRatio         = uint64(85)
)

// CtxKeyT type of context key
type CtxKeyT struct{}

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

// ForceGCBlocking force to run blocking manual gc.
func ForceGCBlocking() {
	Logger.Info("force gc")
	runtime.GC()
	debug.FreeOSMemory()
}

// ForceGCUnBlocking trigger GC unblocking
func ForceGCUnBlocking() {
	go func() {
		ForceGC()
	}()
}

type gcOption struct {
	memRatio         uint64
	memLimitFilePath string
}

// GcOptFunc option for GC utils
type GcOptFunc func(*gcOption)

// WithGCMemRatio set mem ratio trigger for GC
func WithGCMemRatio(ratio int) GcOptFunc {
	if ratio <= 0 {
		Logger.Panic("ratio must > 0", zap.Int("ratio", ratio))
	}
	if ratio > 100 {
		Logger.Panic("ratio must <= 0", zap.Int("ratio", ratio))
	}

	return func(opt *gcOption) {
		Logger.Debug("set memRatio", zap.Int("ratio", ratio))
		opt.memRatio = uint64(ratio)
	}
}

// WithGCMemLimitFilePath set memory limit file
func WithGCMemLimitFilePath(path string) GcOptFunc {
	if !fileutil.Exist(path) {
		Logger.Panic("file path not exists", zap.String("file", path))
	}

	return func(opt *gcOption) {
		Logger.Debug("set memLimitFilePath", zap.String("file", path))
		opt.memLimitFilePath = path
	}
}

// AutoGC auto trigger GC when memory usage exceeds the custom ration
func AutoGC(ctx context.Context, opts ...GcOptFunc) (err error) {
	opt := &gcOption{
		memRatio:         defaultGCMemRatio,
		memLimitFilePath: defaultCgroupMemLimitPath,
	}
	for _, optf := range opts {
		optf(opt)
	}

	var (
		fp       *os.File
		memByte  []byte
		memLimit uint64
	)
	if fp, err = os.Open(opt.memLimitFilePath); err != nil {
		return errors.Wrapf(err, "open file got error: %+v", opt.memLimitFilePath)
	}
	defer fp.Close()
	if memByte, err = ioutil.ReadAll(fp); err != nil {
		return errors.Wrap(err, "read cgroup mem limit file")
	}
	if err = fp.Close(); err != nil {
		Logger.Error("close cgroup mem limit file", zap.Error(err), zap.String("file", opt.memLimitFilePath))
	}

	if memLimit, err = strconv.ParseUint(string(bytes.TrimSpace(memByte)), 10, 64); err != nil {
		return errors.Wrap(err, "parse cgroup memory limit")
	}
	if memLimit <= 0 {
		return fmt.Errorf("mem limit should > 0, but got: %v", memLimit)
	}
	Logger.Info("enable auto gc", zap.Uint64("ratio", opt.memRatio), zap.Uint64("limit", memLimit))

	go func(ctx context.Context) {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var (
			m runtime.MemStats
		)
		for {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
			runtime.ReadMemStats(&m)
			Logger.Debug("memo stat",
				zap.Uint64("alloc", m.Alloc),
				zap.Uint64("limit", memLimit),
			)
			if m.Alloc/memLimit >= opt.memRatio {
				ForceGCBlocking()
			}
		}
	}(ctx)

	return nil
}

var (
	// ForceGC force to start gc blocking
	ForceGC = ForceGCBlocking
	// TriggerGC force to start gc unblocking
	TriggerGC = ForceGCUnBlocking
)

var defaultTemplateWithMappReg = regexp.MustCompile(`(?sm)\$\{([^}]+)\}`)

// TemplateWithMap replace `${var}` in template string
func TemplateWithMap(tpl string, data map[string]interface{}) string {
	return TemplateWithMapAndRegexp(defaultTemplateWithMappReg, tpl, data)
}

// TemplateWithMapAndRegexp replace `${var}` in template string
func TemplateWithMapAndRegexp(tplReg *regexp.Regexp, tpl string, data map[string]interface{}) string {
	var (
		k, vs string
		vi    interface{}
	)
	for _, kg := range tplReg.FindAllStringSubmatch(tpl, -1) {
		k = kg[1]
		vi = data[k]
		switch vi := vi.(type) {
		case string:
			vs = vi
		case []byte:
			vs = string(vi)
		case int:
			vs = strconv.FormatInt(int64(vi), 10)
		case int64:
			vs = strconv.FormatInt(vi, 10)
		case float64:
			vs = strconv.FormatFloat(vi, 'f', -1, 64)
		}
		tpl = strings.ReplaceAll(tpl, "${"+k+"}", vs)
	}

	return tpl
}

var (
	urlMaskingRegexp = regexp.MustCompile(`(\S+:)\S+(@\w+)`)
)

// URLMasking masking password in url
func URLMasking(url, mask string) string {
	return urlMaskingRegexp.ReplaceAllString(url, `${1}`+mask+`${2}`)
}

// DirSize calculate directory size.
// https://stackoverflow.com/a/32482941/2368737
func DirSize(path string) (size int64, err error) {
	err = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})

	return
}
