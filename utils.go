// Package utils 一些常用工具
package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
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

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
	// JSON effective json
	JSON = json
)

const (
	defaultCgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	defaultGCMemRatio         = uint64(85)
)

// CtxKeyT type of context key
type CtxKeyT struct{}

// ValidateFileHash validate file content with hashed string
func ValidateFileHash(filepath string, hashed string) error {
	hs := strings.Split(hashed, ":")
	if len(hs) != 2 {
		return fmt.Errorf("unknown hashed format, expect is `sha256:xxxx`, but got `%s`", hashed)
	}

	var hasher hash.Hash
	switch hs[0] {
	case "sha256":
		hasher = sha256.New()
	default:
		return fmt.Errorf("unknown hasher `%s`", hs[0])
	}

	fp, err := os.Open(filepath)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", filepath)
	}
	defer fp.Close()

	if _, err = io.Copy(hasher, fp); err != nil {
		return errors.Wrap(err, "read file content")
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if hs[1] != actualHash {
		return fmt.Errorf("hash `%s` not match expect `%s`", actualHash, hs[1])
	}

	return nil
}

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
type GcOptFunc func(*gcOption) error

// WithGCMemRatio set mem ratio trigger for GC
func WithGCMemRatio(ratio int) GcOptFunc {
	return func(opt *gcOption) error {
		if ratio <= 0 {
			return fmt.Errorf("ratio must > 0, got %v", ratio)
		}
		if ratio > 100 {
			return fmt.Errorf("ratio must <= 0, got %v", ratio)
		}

		Logger.Debug("set memRatio", zap.Int("ratio", ratio))
		opt.memRatio = uint64(ratio)
		return nil
	}
}

// WithGCMemLimitFilePath set memory limit file
func WithGCMemLimitFilePath(path string) GcOptFunc {
	return func(opt *gcOption) error {
		if !fileutil.Exist(path) {
			return fmt.Errorf("file path not exists, got %v", path)
		}

		Logger.Debug("set memLimitFilePath", zap.String("file", path))
		opt.memLimitFilePath = path
		return nil
	}
}

// AutoGC auto trigger GC when memory usage exceeds the custom ration
func AutoGC(ctx context.Context, opts ...GcOptFunc) (err error) {
	opt := &gcOption{
		memRatio:         defaultGCMemRatio,
		memLimitFilePath: defaultCgroupMemLimitPath,
	}
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return errors.Wrap(err, "set option")
		}
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
	if memLimit == 0 {
		return fmt.Errorf("mem limit should > 0, but got: %v", memLimit)
	}
	Logger.Info("enable auto gc", zap.Uint64("ratio", opt.memRatio), zap.Uint64("limit", memLimit))

	go func(ctx context.Context) {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		var (
			m     runtime.MemStats
			ratio uint64
		)
		for {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
			runtime.ReadMemStats(&m)
			ratio = (m.Alloc * 100) / memLimit
			Logger.Debug("mem stat",
				zap.Uint64("mem", m.Alloc),
				zap.Uint64("limit_mem", memLimit),
				zap.Uint64("ratio", ratio),
				zap.Uint64("limit_ratio", opt.memRatio),
			)
			if ratio >= opt.memRatio {
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

// SetStructFieldsBySlice set field value of structs slice by values slice
func SetStructFieldsBySlice(structs, vals interface{}) (err error) {
	sv := reflect.ValueOf(structs)
	vv := reflect.ValueOf(vals)

	typeCheck := func(name string, v *reflect.Value) error {
		switch v.Kind() {
		case reflect.Slice:
		case reflect.Array:
		default:
			return fmt.Errorf(name + " must be array/slice")
		}

		return nil
	}
	if err = typeCheck("structs", &sv); err != nil {
		return err
	}
	if err = typeCheck("vals", &vv); err != nil {
		return err
	}

	var (
		eachGrpValsV    reflect.Value
		iField, nFields int
	)
	for i := 0; i < MinInt(sv.Len(), vv.Len()); i++ {
		eachGrpValsV = vv.Index(i)
		if err = typeCheck("vals."+strconv.FormatInt(int64(i), 10), &eachGrpValsV); err != nil {
			return err
		}
		switch sv.Index(i).Kind() {
		case reflect.Ptr:
			nFields = sv.Index(i).Elem().NumField()
		default:
			nFields = sv.Index(i).NumField()
		}
		for iField = 0; iField < MinInt(eachGrpValsV.Len(), nFields); iField++ {
			switch sv.Index(i).Kind() {
			case reflect.Ptr:
				sv.Index(i).Elem().Field(iField).Set(eachGrpValsV.Index(iField))
			default:
				sv.Index(i).Field(iField).Set(eachGrpValsV.Index(iField))
			}
		}
	}

	return
}

// UniqueStrings remove duplicate string in slice
func UniqueStrings(vs []string) (r []string) {
	m := map[string]struct{}{}
	var ok bool
	for _, v := range vs {
		if _, ok = m[v]; !ok {
			m[v] = struct{}{}
			r = append(r, v)
		}
	}

	return
}

// IsPtr check if t is pointer
func IsPtr(t interface{}) bool {
	return reflect.TypeOf(t).Kind() == reflect.Ptr
}
