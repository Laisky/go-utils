// Package utils 一些常用工具
package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// IsHasField check is struct has field
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func IsHasField(st interface{}, fieldName string) bool {
	valueIface := reflect.ValueOf(st)

	// Check if the passed interface is a pointer
	if valueIface.Type().Kind() != reflect.Ptr {
		// Create a new type of Iface's Type, so we have a pointer to work with
		valueIface = reflect.New(reflect.TypeOf(st))
	}

	// 'dereference' with Elem() and get the field by name
	field := valueIface.Elem().FieldByName(fieldName)
	return field.IsValid()
}

// IsHasMethod check is struct has method
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func IsHasMethod(st interface{}, methodName string) bool {
	valueIface := reflect.ValueOf(st)

	// Check if the passed interface is a pointer
	if valueIface.Type().Kind() != reflect.Ptr {
		// Create a new type of Iface, so we have a pointer to work with
		valueIface = reflect.New(reflect.TypeOf(st))
	}

	// Get the method by name
	method := valueIface.MethodByName(methodName)
	return method.IsValid()
}

// ValidateFileHash validate file content with hashed string
//
// Args:
//   * filepath: file path to check
//   * hashed: hashed string, like `sha256: xxxx`
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
			return fmt.Errorf("ratio must > 0, got %d", ratio)
		}
		if ratio > 100 {
			return fmt.Errorf("ratio must <= 0, got %d", ratio)
		}

		Logger.Debug("set memRatio", zap.Int("ratio", ratio))
		opt.memRatio = uint64(ratio)
		return nil
	}
}

// WithGCMemLimitFilePath set memory limit file
func WithGCMemLimitFilePath(path string) GcOptFunc {
	return func(opt *gcOption) error {
		if _, err := os.Open(path); err != nil {
			return errors.Wrapf(err, "try open path `%s`", path)
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
		return fmt.Errorf("mem limit should > 0, but got: %d", memLimit)
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

// RemoveEmpty remove duplicate string in slice
func RemoveEmpty(vs []string) (r []string) {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			r = append(r, v)
		}
	}

	return
}

// TrimEleSpaceAndRemoveEmpty remove duplicate string in slice
func TrimEleSpaceAndRemoveEmpty(vs []string) (r []string) {
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v != "" {
			r = append(r, v)
		}
	}

	return
}

// InArray if collection contains ele
func InArray(collection interface{}, ele interface{}) bool {
	targetValue := reflect.ValueOf(collection)
	switch reflect.TypeOf(collection).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == ele {
				return true
			}
		}
	default:
		Logger.Panic("unsupport type", zap.String("type", reflect.TypeOf(collection).Kind().String()))
	}

	return false
}

// IsPtr check if t is pointer
func IsPtr(t interface{}) bool {
	return reflect.TypeOf(t).Kind() == reflect.Ptr
}

// RunCMD run command script
func RunCMD(ctx context.Context, app string, args ...string) (stdout []byte, err error) {
	return exec.CommandContext(ctx, app, args...).Output()
}

// Base64Encode encode bytes to string use base64
func Base64Encode(raw []byte) string {
	return base64.URLEncoding.EncodeToString(raw)
}

// Base64Decode decode string to bytes use base64
func Base64Decode(encoded string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(encoded)
}

// ExpCache cache with expires
type ExpCache struct {
	data sync.Map
	exp  time.Duration
}

type expCacheItem struct {
	exp  time.Time
	data interface{}
}

// NewExpCache new cache manager
func NewExpCache(ctx context.Context, exp time.Duration) *ExpCache {
	c := &ExpCache{
		exp: exp,
	}
	go c.runClean(ctx)
	return c
}

func (c *ExpCache) runClean(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.data.Range(func(k, v interface{}) bool {
			if v.(*expCacheItem).exp.After(Clock.GetUTCNow()) {
				// expired
				//
				// if new expCacheItem stored just before delete,
				// may delete item that not expired.
				// but this condition is rare, so may just add a little cose.
				c.data.Delete(k)
			}

			return true
		})

		time.Sleep(c.exp)
	}
}

// Store store new key and val into cache
func (c *ExpCache) Store(key, val interface{}) {
	c.data.Store(key, &expCacheItem{
		data: val,
		exp:  Clock.GetUTCNow().Add(c.exp),
	})
}

// Load load val from cache
func (c *ExpCache) Load(key interface{}) (data interface{}, ok bool) {
	if data, ok = c.data.Load(key); ok && Clock.GetUTCNow().Before(data.(*expCacheItem).exp) {
		return data.(*expCacheItem).data, ok
	} else if ok {
		// delete expired
		c.data.Delete(key)
	}

	return nil, false
}

type expiredMapItem struct {
	sync.RWMutex
	data interface{}
	t    *int64
}

func (e *expiredMapItem) getTime() time.Time {
	return ParseUnix2UTC(atomic.LoadInt64(e.t))
}

func (e *expiredMapItem) refreshTime() {
	atomic.StoreInt64(e.t, Clock.GetUTCNow().Unix())
}

// ExpiredMap map with expire time, auto delete expired item.
//
// `Get` will auto refresh item's expires.
type ExpiredMap struct {
	m   sync.Map
	ttl time.Duration
	new func() interface{}
}

// NewExpiredMap new ExpiredMap
func NewExpiredMap(ctx context.Context, ttl time.Duration, new func() interface{}) (el *ExpiredMap, err error) {
	el = &ExpiredMap{
		ttl: ttl,
		new: new,
	}

	go el.clean(ctx)
	return el, nil
}

func (e *ExpiredMap) clean(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.m.Range(func(k, v interface{}) bool {
			if v.(*expiredMapItem).getTime().Add(e.ttl).After(Clock.GetUTCNow()) {
				return true
			}

			// lock is expired
			v.(*expiredMapItem).Lock()
			defer v.(*expiredMapItem).Unlock()

			if v.(*expiredMapItem).getTime().Add(e.ttl).Before(Clock.GetUTCNow()) {
				// lock still expired
				e.m.Delete(k)
			}

			return true
		})

		time.Sleep(e.ttl / 2)
	}
}

// Get get item
//
func (e *ExpiredMap) Get(key string) interface{} {
	l, _ := e.m.Load(key)
	if l == nil {
		t := Clock.GetUTCNow().Unix()
		l, _ = e.m.LoadOrStore(key, &expiredMapItem{
			t:    &t,
			data: e.new(),
		})
	} else {
		ol := l.(*expiredMapItem)
		ol.RLock()
		ol.refreshTime()
		l, _ = e.m.LoadOrStore(key, ol)
		ol.RUnlock()
	}

	return l.(*expiredMapItem).data
}
