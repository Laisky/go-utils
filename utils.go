package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/google/go-cpy/cpy"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/go-utils/v6/common"
	"github.com/Laisky/go-utils/v6/log"
)

type jsonT struct {
	jsoniter.API
}

var (
	// JSON effective json
	//
	// Deprecated: use github.com/Laisky/go-utils/v6/json instead
	JSON = jsonT{API: jsoniter.ConfigCompatibleWithStandardLibrary}

	internalSFG singleflight.Group

	// for compatibility to old version
	// =====================================

	// Str2Bytes unsafe convert str to bytes
	Str2Bytes = common.Str2Bytes
	// Bytes2Str unsafe convert bytes to str
	Bytes2Str = common.Bytes2Str
	// Number2Roman convert number to roman
	Number2Roman = common.Number2Roman
)

func init() {
	if _, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		log.Shared.Debug(fmt.Sprintf(s, i...))
	})); err != nil {
		log.Shared.Error("auto set maxprocs", zap.Error(err))
	}
}

var cloner = cpy.New(
	cpy.IgnoreAllUnexported(),
)

// DeepClone deep clone a struct
//
// will ignore all unexported fields
func DeepClone[T any](src T) (dst T) {
	return cloner.Copy(src).(T) //nolint:forcetypeassert
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

// CloseWithLog close and log error.
// logger could be nil, then will use internal log.Shared logger instead.
func CloseWithLog(ins interface{ Close() error },
	logger interface{ Error(string, ...zap.Field) }) {
	LogErr(ins.Close, logger)
}

// LogErr invoke f and log error if got error.
func LogErr(f func() error, logger interface{ Error(string, ...zap.Field) }) {
	if logger == nil {
		logger = log.Shared
	}

	if err := f(); err != nil {
		logger.Error("close ins", zap.Error(err))
	}
}

// FlushWithLog flush and log error.
// logger could be nil, then will use internal log.Shared logger instead.
func FlushWithLog(ins interface{ Flush() error },
	logger interface{ Error(string, ...zap.Field) }) {
	if logger == nil {
		logger = log.Shared
	}

	if err := ins.Flush(); err != nil {
		logger.Error("flush ins", zap.Error(err))
	}
}

// ValidateFileHash validate file hash against a hashed string
//
// Deprecated: use VerifyFileHash instead
var ValidateFileHash = VerifyFileHash

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

// func CalculateCRC(cnt []byte) {
// 	cw := crc64.New(crc64.MakeTable(crc64.ISO))
// }

// IsPanic is `f()` throw panic
//
// if you want to get the data throwed by panic, use `IsPanic2`
func IsPanic(f func()) (isPanic bool) {
	defer func() {
		if deferErr := recover(); deferErr != nil {
			isPanic = true
		}
	}()

	f()
	return isPanic
}

// IsPanic2 check is `f()` throw panic, and return panic as error
func IsPanic2(f func()) (err error) {
	defer func() {
		if panicRet := recover(); panicRet != nil {
			err = errors.Errorf("panic: %v", panicRet)
		}
	}()

	f()
	return nil
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
		for range c { //nolint: revive
		}
	}
}

// CostSecs convert duration to string like `0.25s`
func CostSecs(cost time.Duration) string {
	return fmt.Sprintf("%.2fs", float64(cost)/float64(time.Second))
}

// Pipeline run f(v) for all funcs
func Pipeline[T any](funcs []func(T) error, v T) (T, error) {
	for _, f := range funcs {
		if err := f(v); err != nil {
			return v, errors.Wrap(err, "execute pipeline function")
		}
	}

	return v, nil
}

// Delayer create by NewDelay
//
// do not use this type directly.
type Delayer struct {
	startAt time.Time
	d       time.Duration
}

// NewDelay ensures the execution time of a function is not less than a predefined threshold.
//
//	defer NewDelay(time.Second).Wait()
func NewDelay(d time.Duration) *Delayer {
	return &Delayer{
		startAt: time.Now(),
		d:       d,
	}
}

// Wait wait in defer
func (d *Delayer) Wait() {
	time.Sleep(d.d - time.Since(d.startAt))
}

// FileHashSharding get file hash sharding path
func FileHashSharding(fname string) string {
	hasher := sha1.New()
	if _, err := hasher.Write([]byte(fname)); err != nil {
		log.Shared.Panic("failed to write file name to hasher", zap.Error(err))
	}

	hashed := hex.EncodeToString(hasher.Sum(nil))
	return filepath.Join(hashed[:2], hashed[2:4], fname)
}

// GetEnvInsensitive get env case insensitive
func GetEnvInsensitive(key string) (values []string) {
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		if strings.EqualFold(pair[0], key) {
			values = append(values, pair[1])
		}
	}

	return
}
