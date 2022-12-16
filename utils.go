package utils

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/Laisky/errors"
	"github.com/Laisky/zap"
	"github.com/google/go-cpy/cpy"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/go-utils/v3/log"

	// automaxprocs compatable with cgroup
	_ "go.uber.org/automaxprocs"
)

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
	// JSON effective json
	JSON = json

	internalSFG singleflight.Group
)

const (
	defaultCgroupMemLimitPath = "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	defaultGCMemRatio         = uint64(85)
)

var cloner = cpy.New(
	cpy.IgnoreAllUnexported(),
)

// DeepClone deep clone a struct
//
// will ignore all unexported fields
func DeepClone(src any) (dst any) {
	return cloner.Copy(src)
}

var dedentMarginChar = regexp.MustCompile(`^[ \t]*`)

type dedentOpt struct {
	replaceTabBySpaces int
}

func (d *dedentOpt) fillDefault() *dedentOpt {
	d.replaceTabBySpaces = 4
	return d
}

func (d *dedentOpt) applyOpts(optfs ...DedentOptFunc) *dedentOpt {
	for _, optf := range optfs {
		optf(d)
	}
	return d
}

// SilentClose close and ignore error
//
// Example
//
//	defer SilentClose(fp)
func SilentClose(v interface{ Close() error }) {
	_ = v.Close()
}

// SilentFlush flush and ignore error
func SilentFlush(v interface{ Flush() error }) {
	_ = v.Flush()
}

// DedentOptFunc dedent option
type DedentOptFunc func(opt *dedentOpt)

// WithReplaceTabBySpaces replace tab to spaces
func WithReplaceTabBySpaces(spaces int) DedentOptFunc {
	return func(opt *dedentOpt) {
		opt.replaceTabBySpaces = spaces
	}
}

// Dedent removes leading whitespace or tab from the beginning of each line
//
// will replace all tab to 4 blanks.
func Dedent(v string, optfs ...DedentOptFunc) string {
	opt := new(dedentOpt).fillDefault().applyOpts(optfs...)
	ls := strings.Split(v, "\n")
	var (
		firstLine      = true
		NSpaceTobeTrim int
		result         []string
	)
	for _, l := range ls {
		if strings.TrimSpace(l) == "" {
			if !firstLine {
				result = append(result, "")
			}

			continue
		}

		m := dedentMarginChar.FindString(l)
		spaceIndent := strings.ReplaceAll(m, "\t", strings.Repeat(" ", opt.replaceTabBySpaces))
		n := len(spaceIndent)
		l = strings.Replace(l, m, spaceIndent, 1)
		if firstLine {
			NSpaceTobeTrim = n
			firstLine = false
		} else if n != 0 && n < NSpaceTobeTrim {
			// choose the smallest margin
			NSpaceTobeTrim = n
		}

		result = append(result, l)
	}

	for i := range result {
		if result[i] == "" {
			continue
		}

		result[i] = result[i][NSpaceTobeTrim:]
	}

	// remove tail blank lines
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] == "" {
			result = result[:i]
		} else {
			break
		}
	}

	return strings.Join(result, "\n")
}

// HasField check is struct has field
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func HasField(st any, fieldName string) bool {
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

// HasMethod check is struct has method
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func HasMethod(st any, methodName string) bool {
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

// MD5JSON calculate md5(jsonify(data))
func MD5JSON(data any) (string, error) {
	if NilInterface(data) {
		return "", errors.New("data is nil")
	}

	b, err := JSON.Marshal(data)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", md5.Sum(b)), nil
}

// NilInterface make sure data is nil interface or another type with nil value
//
// Example:
//
//	type foo struct{}
//	var f *foo
//	var v any
//	v = f
//	v == nil // false
//	NilInterface(v) // true
func NilInterface(data any) bool {
	if data == nil {
		return true
	}

	if reflect.TypeOf(data).Kind() == reflect.Ptr &&
		reflect.ValueOf(data).IsNil() {
		return true
	}

	return false
}

// GetStructFieldByName get struct field by name
func GetStructFieldByName(st any, fieldName string) any {
	stv := reflect.ValueOf(st)
	if IsPtr(st) {
		stv = stv.Elem()
	}

	v := stv.FieldByName(fieldName)
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Slice,
		reflect.Array,
		reflect.Interface,
		reflect.Ptr,
		reflect.Map:
		if v.IsNil() {
			return nil
		}
	}

	return v.Interface()
}

// ValidateFileHash validate file content with hashed string
//
// Args:
//   - filepath: file path to check
//   - hashed: hashed string, like `sha256: xxxx`
func ValidateFileHash(filepath string, hashed string) error {
	hs := strings.Split(hashed, ":")
	if len(hs) != 2 {
		return errors.Errorf("unknown hashed format, expect is `sha256:xxxx`, but got `%s`", hashed)
	}

	var hasher hash.Hash
	switch hs[0] {
	case "sha256":
		hasher = sha256.New()
	case "md5":
		hasher = md5.New()
	default:
		return errors.Errorf("unknown hasher `%s`", hs[0])
	}

	fp, err := os.Open(filepath)
	if err != nil {
		return errors.Wrapf(err, "open file `%s`", filepath)
	}
	defer SilentClose(fp)

	if _, err = io.Copy(hasher, fp); err != nil {
		return errors.Wrap(err, "read file content")
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if hs[1] != actualHash {
		return errors.Errorf("hash `%s` not match expect `%s`", actualHash, hs[1])
	}

	return nil
}

// GetFuncName return the name of func
func GetFuncName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// FallBack return the fallback when orig got error
// utils.FallBack(func() any { return getIOStatMetric(fs) }, &IOStat{}).(*IOStat)
func FallBack(orig func() any, fallback any) (ret any) {
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
func FlattenMap(data map[string]any, delimiter string) {
	for k, vi := range data {
		if v2i, ok := vi.(map[string]any); ok {
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
	log.Shared.Debug("force gc")
	runtime.GC()
	debug.FreeOSMemory()
}

// ForceGCUnBlocking trigger GC unblocking
func ForceGCUnBlocking() {
	go func() {
		_, _, _ = internalSFG.Do("ForceGCUnBlocking", func() (any, error) {
			ForceGC()
			return nil, nil
		})
	}()
}

type gcOption struct {
	memRatio         uint64
	memLimitFilePath string
}

// GcOptFunc option for GC utils
type GcOptFunc func(*gcOption) error

// WithGCMemRatio set mem ratio trigger for GC
//
// default to 85
func WithGCMemRatio(ratio int) GcOptFunc {
	return func(opt *gcOption) error {
		if ratio <= 0 {
			return errors.Errorf("ratio must > 0, got %d", ratio)
		}
		if ratio > 100 {
			return errors.Errorf("ratio must <= 0, got %d", ratio)
		}

		log.Shared.Debug("set memRatio", zap.Int("ratio", ratio))
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

		log.Shared.Debug("set memLimitFilePath", zap.String("file", path))
		opt.memLimitFilePath = path
		return nil
	}
}

// AutoGC auto trigger GC when memory usage exceeds the custom ration
//
// default to /sys/fs/cgroup/memory/memory.limit_in_bytes
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
	defer SilentClose(fp)

	if memByte, err = io.ReadAll(fp); err != nil {
		return errors.Wrap(err, "read cgroup mem limit file")
	}

	if err = fp.Close(); err != nil {
		log.Shared.Error("close cgroup mem limit file", zap.Error(err), zap.String("file", opt.memLimitFilePath))
	}

	if memLimit, err = strconv.ParseUint(string(bytes.TrimSpace(memByte)), 10, 64); err != nil {
		return errors.Wrap(err, "parse cgroup memory limit")
	}
	if memLimit == 0 {
		return errors.Errorf("mem limit should > 0, but got: %d", memLimit)
	}
	log.Shared.Info("enable auto gc", zap.Uint64("ratio", opt.memRatio), zap.Uint64("limit", memLimit))

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
			log.Shared.Debug("mem stat",
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
func TemplateWithMap(tpl string, data map[string]any) string {
	return TemplateWithMapAndRegexp(defaultTemplateWithMappReg, tpl, data)
}

// TemplateWithMapAndRegexp replace `${var}` in template string
func TemplateWithMapAndRegexp(tplReg *regexp.Regexp, tpl string, data map[string]any) string {
	var (
		k, vs string
		vi    any
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
func SetStructFieldsBySlice(structs, vals any) (err error) {
	sv := reflect.ValueOf(structs)
	vv := reflect.ValueOf(vals)

	typeCheck := func(name string, v *reflect.Value) error {
		switch v.Kind() {
		case reflect.Slice:
		case reflect.Array:
		default:
			return errors.Errorf(name + " must be array/slice")
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
	for i := 0; i < Min(sv.Len(), vv.Len()); i++ {
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
		for iField = 0; iField < Min(eachGrpValsV.Len(), nFields); iField++ {
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

// Contains if collection contains ele
func Contains[V comparable](collection []V, ele V) bool {
	for _, v := range collection {
		if v == ele {
			return true
		}
	}

	return false
}

// InArray if collection contains ele
//
// Depracated: use Contains instead
func InArray(collection any, ele any) bool {
	targetValue := reflect.ValueOf(collection)
	switch reflect.TypeOf(collection).Kind() {
	case reflect.Slice, reflect.Array:
		if reflect.TypeOf(collection).Elem().Kind() != reflect.TypeOf(ele).Kind() {
			panic(fmt.Sprintf(
				"collection(%v) and ele(%v) must in same type",
				reflect.TypeOf(collection).Elem().Kind().String(),
				reflect.TypeOf(ele).Kind().String(),
			))
		}

		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == ele {
				return true
			}
		}
	default:
		log.Shared.Panic("unsupport type", zap.String("type", reflect.TypeOf(collection).Kind().String()))
	}

	return false
}

// IsPtr check if t is pointer
func IsPtr(t any) bool {
	return reflect.TypeOf(t).Kind() == reflect.Ptr
}

// RunCMD run command script
func RunCMD(ctx context.Context, app string, args ...string) (stdout []byte, err error) {
	stdout, err = exec.CommandContext(ctx, app, args...).CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, string(stdout))
	}

	return stdout, nil
}

// Base64Encode encode bytes to string use base64
func Base64Encode(raw []byte) string {
	return base64.URLEncoding.EncodeToString(raw)
}

// Base64Decode decode string to bytes use base64
func Base64Decode(encoded string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(encoded)
}

// SingleItemExpCache single item with expires
type SingleItemExpCache struct {
	expiredAt time.Time
	ttl       time.Duration
	data      any
	mu        sync.RWMutex
}

// NewSingleItemExpCache new expcache contains single data
func NewSingleItemExpCache(ttl time.Duration) *SingleItemExpCache {
	return &SingleItemExpCache{
		ttl: ttl,
	}
}

// Set set data and refresh expires
func (c *SingleItemExpCache) Set(data any) {
	c.mu.Lock()
	c.data = data
	c.expiredAt = Clock.GetUTCNow().Add(c.ttl)
	c.mu.Unlock()
}

// Get get data
//
// if data is expired, ok=false
func (c *SingleItemExpCache) Get() (data any, ok bool) {
	c.mu.RLock()
	data = c.data

	ok = Clock.GetUTCNow().Before(c.expiredAt)
	c.mu.RUnlock()

	return
}

// GetString same as Get, but return string
func (c *SingleItemExpCache) GetString() (data string, ok bool) {
	var itf any
	if itf, ok = c.Get(); !ok {
		return "", false
	}

	return itf.(string), true
}

// GetUintSlice same as Get, but return []uint
func (c *SingleItemExpCache) GetUintSlice() (data []uint, ok bool) {
	var itf any
	if itf, ok = c.Get(); !ok {
		return nil, false
	}

	return itf.([]uint), true
}

// ExpCache cache with expires
//
// can Store/Load like map
type ExpCache struct {
	data sync.Map
	ttl  time.Duration
}

type expCacheItem struct {
	exp  time.Time
	data any
}

// NewExpCache new cache manager
func NewExpCache(ctx context.Context, ttl time.Duration) *ExpCache {
	c := &ExpCache{
		ttl: ttl,
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

		c.data.Range(func(k, v any) bool {
			if v.(*expCacheItem).exp.After(Clock.GetUTCNow()) {
				// expired
				//
				// if new expCacheItem stored just before delete,
				// may delete item that not expired.
				// but this condition is rare, so may just add a little cost.
				c.data.Delete(k)
			}

			return true
		})

		time.Sleep(c.ttl)
	}
}

// Store store new key and val into cache
func (c *ExpCache) Store(key, val any) {
	c.data.Store(key, &expCacheItem{
		data: val,
		exp:  Clock.GetUTCNow().Add(c.ttl),
	})
}

// Delete remove key
func (c *ExpCache) Delete(key any) {
	c.data.Delete(key)
}

// Load load val from cache
func (c *ExpCache) Load(key any) (data any, ok bool) {
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
	data any
	t    *int64
}

func (e *expiredMapItem) getTime() time.Time {
	return ParseUnix2UTC(atomic.LoadInt64(e.t))
}

func (e *expiredMapItem) refreshTime() {
	atomic.StoreInt64(e.t, Clock.GetUTCNow().Unix())
}

// LRUExpiredMap map with expire time, auto delete expired item.
//
// `Get` will auto refresh item's expires.
type LRUExpiredMap struct {
	m   sync.Map
	ttl time.Duration
	new func() any
}

// NewLRUExpiredMap new ExpiredMap
func NewLRUExpiredMap(ctx context.Context,
	ttl time.Duration,
	new func() any) (el *LRUExpiredMap, err error) {
	el = &LRUExpiredMap{
		ttl: ttl,
		new: new,
	}

	go el.clean(ctx)
	return el, nil
}

func (e *LRUExpiredMap) clean(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.m.Range(func(k, v any) bool {
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
// will auto refresh key's ttl
func (e *LRUExpiredMap) Get(key string) any {
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

// Str2Bytes unsafe convert str to bytes
func Str2Bytes(s string) []byte {
	sp := (*[2]uintptr)(unsafe.Pointer(&s))
	bp := [3]uintptr{sp[0], sp[1], sp[1]}
	return *(*[]byte)(unsafe.Pointer(&bp))
}

// Bytes2Str unsafe convert bytes to str
func Bytes2Str(b []byte) string {
	bp := (*[3]uintptr)(unsafe.Pointer(&b))
	sp := [2]uintptr{bp[0], bp[1]}
	return *(*string)(unsafe.Pointer(&sp))
}

// ConvertMap2StringKey convert any map to `map[string]any`
func ConvertMap2StringKey(inputMap any) map[string]any {
	v := reflect.ValueOf(inputMap)
	if v.Kind() != reflect.Map {
		return nil
	}

	m2 := map[string]any{}
	ks := v.MapKeys()
	for _, k := range ks {
		if k.Kind() == reflect.Interface {
			m2[k.Elem().String()] = v.MapIndex(k).Interface()
		} else {
			m2[fmt.Sprint(k)] = v.MapIndex(k).Interface()
		}
	}

	return m2
}

// func CalculateCRC(cnt []byte) {
// 	cw := crc64.New(crc64.MakeTable(crc64.ISO))
// }

// IsPanic is `f()` throw panic
func IsPanic(f func()) (isPanic bool) {
	defer func() {
		if err := recover(); err != nil {
			isPanic = true
		}
	}()

	f()
	return false
}

var onlyOneSignalHandler = make(chan struct{})

type stopSignalOpt struct {
	closeSignals []os.Signal
	// closeFunc    func()
}

// StopSignalOptFunc options for StopSignal
type StopSignalOptFunc func(*stopSignalOpt)

// WithStopSignalCloseSignals set signals that will trigger close
func WithStopSignalCloseSignals(signals ...os.Signal) StopSignalOptFunc {
	if len(signals) == 0 {
		log.Shared.Panic("signals cannot be empty")
	}

	return func(opt *stopSignalOpt) {
		opt.closeSignals = signals
	}
}

// // WithStopSignalCloseFunc set func that will be called when signal is triggered
// func WithStopSignalCloseFunc(f func()) StopSignalOptFunc {
// 	if f == nil {
// 		log.Shared.Panic("f cannot be nil")
// 	}

// 	return func(opt *stopSignalOpt) {
// 		opt.closeFunc = f
// 	}
// }

// StopSignal registered for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
//
// Copied from https://github.com/kubernetes/sample-controller
func StopSignal(optfs ...StopSignalOptFunc) (stopCh <-chan struct{}) {
	opt := &stopSignalOpt{
		closeSignals: []os.Signal{syscall.SIGTERM, syscall.SIGINT},
		// closeFunc:    func() { os.Exit(1) },
	}
	for _, optf := range optfs {
		optf(opt)
	}

	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		close(stop)
	}()

	return stop
}

// PanicIfErr panic if err is not nil
func PanicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

// GracefulCancel is a function that will be called when the process is about to be terminated.
func GracefulCancel(cancel func()) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	cancel()
}

// EmptyAllChans receive all thins in all chans
func EmptyAllChans[T any](chans ...chan T) {
	for _, c := range chans {
		for range c {
		}
	}
}

// PrettyBuildInfo get build info in formatted json
//
// Print:
//
//	{
//	  "Path": "github.com/Laisky/go-ramjet",
//	  "Version": "v0.0.0-20220718014224-2b10e57735f1",
//	  "Sum": "h1:08Ty2gR+Xxz0B3djHVuV71boW4lpNdQ9hFn4ZIGrhec=",
//	  "Replace": null
//	}
func PrettyBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Shared.Error("failed to read build info")
		return ""
	}

	ver, err := JSON.MarshalIndent(info.Main, "", "  ")
	if err != nil {
		log.Shared.Error("failed to marshal version", zap.Error(err))
		return ""
	}

	return string(ver)
}
