// Package log enhanced zap logger
package log

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/Laisky/errors/v2"
	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
)

// Logger logger interface
type Logger interface {
	zapLoggerItf
	// Level get current level
	Level() Level
	// ChangeLevel change log and all its children's level
	ChangeLevel(level Level) (err error)
	// DebugSample debug with sample/1000
	DebugSample(sample int, msg string, fields ...zapcore.Field)
	// InfoSample info with sample/1000
	InfoSample(sample int, msg string, fields ...zapcore.Field)
	// WarnSample warn with sample/1000
	WarnSample(sample int, msg string, fields ...zapcore.Field)
	// Named create named child logger
	Named(childName string) *LoggerT
	// With with fields
	With(fields ...zapcore.Field) *LoggerT
	// WithOptions with options
	WithOptions(opts ...zap.Option) *LoggerT
}

const (
	// SampleRateDenominator sample rate = sample / SampleRateDenominator
	SampleRateDenominator = 1000

	defaultAlertPusherTimeout = 10 * time.Second
	defaultAlertPusherBufSize = 20
	defaultAlertHookLevel     = zapcore.ErrorLevel
)

var (
	// Shared logging tool.
	//
	// default level is info, you can change it by env `GUTILS_LOGGER_LEVEL`
	//
	// # Methods:
	//
	// 	* Info(msg string, fields ...Field)
	// 	* Debug(msg string, fields ...Field)
	// 	* Warn(msg string, fields ...Field)
	// 	* Error(msg string, fields ...Field)
	// 	* Panic(msg string, fields ...Field)
	// 	* DebugSample(sample int, msg string, fields ...zapcore.Field)
	// 	* InfoSample(sample int, msg string, fields ...zapcore.Field)
	// 	* WarnSample(sample int, msg string, fields ...zapcore.Field)
	Shared *LoggerT
)

// Level logger level
//
//   - LevelInfo
//   - LevelDebug
//   - LevelWarn
//   - LevelError
//   - LevelFatal
//   - LevelPanic
type Level string

// String convert to string
func (l Level) String() string {
	return string(l)
}

// Zap convert to zap level
func (l Level) Zap() zapcore.Level {
	zl, err := LevelToZap(l)
	if err != nil {
		panic(err)
	}

	return zl
}

const (
	// LevelUnspecified unknown level
	LevelUnspecified Level = "unspecified"
	// LevelInfo Logger level info
	LevelInfo Level = "info"
	// LevelDebug Logger level debug
	LevelDebug Level = "debug"
	// LevelWarn Logger level warn
	LevelWarn Level = "warn"
	// LevelError Logger level error
	LevelError Level = "error"
	// LevelFatal Logger level fatal
	LevelFatal Level = "fatal"
	// LevelPanic Logger level panic
	LevelPanic Level = "panic"
)

type zapLoggerItf interface {
	Debug(msg string, fields ...zapcore.Field)
	Info(msg string, fields ...zapcore.Field)
	Warn(msg string, fields ...zapcore.Field)
	Error(msg string, fields ...zapcore.Field)
	DPanic(msg string, fields ...zapcore.Field)
	Panic(msg string, fields ...zapcore.Field)
	Fatal(msg string, fields ...zapcore.Field)
	Sync() error
	Core() zapcore.Core
}

// LoggerT extend from zap.Logger
type LoggerT struct {
	*zap.Logger

	// level level of current logger
	//
	// zap logger do not expose api to change log's level,
	// so we have to save the pointer of zap.AtomicLevel.
	level zap.AtomicLevel
}

// NewWithName create new logger with name
func NewWithName(name string, level Level, opts ...zap.Option) (l *LoggerT, err error) {
	return New(
		WithName(name),
		WithEncoding(EncodingJSON),
		WithLevel(level),
		WithZapOptions(opts...),
	)
}

// NewConsoleWithName create new logger with name
func NewConsoleWithName(name string, level Level, opts ...zap.Option) (l *LoggerT, err error) {
	return New(
		WithName(name),
		WithEncoding(EncodingConsole),
		WithLevel(level),
		WithZapOptions(opts...),
	)
}

type option struct {
	zap.Config
	zapOptions []zap.Option
	Name       string
}

func (o *option) fillDefault() *option {
	o.Name = "app"
	o.Config = zap.Config{
		Level:            zap.NewAtomicLevel(),
		Development:      false,
		Encoding:         string(EncodingConsole),
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	o.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	o.EncoderConfig.MessageKey = "message"
	o.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	o.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	return o
}

func (o *option) applyOpts(optfs ...Option) (*option, error) {
	for _, optf := range optfs {
		if err := optf(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// Encoding how to print log
type Encoding string

// String convert encoding to string
func (e Encoding) String() string {
	return string(e)
}

const (
	// EncodingConsole is logger format for console
	EncodingConsole Encoding = "console"
	// EncodingJSON is logger format for json
	EncodingJSON Encoding = "json"
)

// Option logger options
type Option func(l *option) error

// WithOutputPaths set output path
//
// like "stdout"
func WithOutputPaths(paths []string) Option {
	return func(c *option) error {
		c.OutputPaths = append(c.OutputPaths, paths...)
		return nil
	}
}

// WithErrorOutputPaths set error logs output path
//
// like "stderr"
func WithErrorOutputPaths(paths []string) Option {
	return func(c *option) error {
		c.ErrorOutputPaths = append(paths, "stderr")
		return nil
	}
}

// WithEncoding set logger encoding formet
func WithEncoding(format Encoding) Option {
	return func(c *option) error {
		switch format {
		case EncodingConsole:
			c.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		case EncodingJSON:
			c.Encoding = string(EncodingJSON)
		default:
			return errors.Errorf("invalid format: %s", format)
		}

		return nil
	}
}

// WithZapOptions set logger with zap.Option
func WithZapOptions(opts ...zap.Option) Option {
	return func(c *option) error {
		c.zapOptions = opts
		return nil
	}
}

// WithName set logger name
func WithName(name string) Option {
	return func(c *option) error {
		c.Name = name
		return nil
	}
}

// LevelToZap
func LevelToZap(level Level) (zapcore.Level, error) {
	switch level {
	case LevelInfo:
		return zap.InfoLevel, nil
	case LevelDebug:
		return zap.DebugLevel, nil
	case LevelWarn:
		return zap.WarnLevel, nil
	case LevelError:
		return zap.ErrorLevel, nil
	case LevelFatal:
		return zap.FatalLevel, nil
	case LevelPanic:
		return zap.PanicLevel, nil
	default:
		return 0, errors.Errorf("invalid level: %s", level)
	}
}

// LevelFromZap convert from zap level
func LevelFromZap(level zapcore.Level) (Level, error) {
	switch level {
	case zap.DebugLevel:
		return LevelDebug, nil
	case zap.InfoLevel:
		return LevelInfo, nil
	case zap.WarnLevel:
		return LevelWarn, nil
	case zap.ErrorLevel:
		return LevelError, nil
	case zap.FatalLevel:
		return LevelFatal, nil
	case zap.PanicLevel:
		return LevelPanic, nil
	default:
		return "", errors.Errorf("invalid level: %s", level)
	}
}

// WithLevel set logger level
func WithLevel(level Level) Option {
	return func(c *option) error {
		lvl, err := LevelToZap(level)
		if err != nil {
			return err
		}

		c.Level.SetLevel(lvl)
		return nil
	}
}

// New create new logger
func New(optfs ...Option) (l *LoggerT, err error) {
	opt, err := new(option).fillDefault().applyOpts(optfs...)
	if err != nil {
		return nil, err
	}

	zapLogger, err := opt.Build(opt.zapOptions...)
	if err != nil {
		return nil, errors.Errorf("build zap logger: %+v", err)
	}
	zapLogger = zapLogger.Named(opt.Name)

	l = &LoggerT{
		Logger: zapLogger,
		level:  opt.Level,
	}

	return l, nil
}

// Level get current level of logger
func (l *LoggerT) Level() Level {
	lvl, err := LevelFromZap(l.level.Level())
	if err != nil {
		panic(err)
	}

	return lvl
}

// Zap return internal z*ap.Logger
func (l *LoggerT) Zap() *zap.Logger {
	return l.Logger
}

// ChangeLevel change logger level
//
// Because all children loggers share the same level as their parent logger,
// if you modify one logger's level, it will affect all of its parent and children loggers.
func (l *LoggerT) ChangeLevel(level Level) (err error) {
	lvl, err := LevelToZap(level)
	if err != nil {
		return err
	}

	l.level.SetLevel(lvl)
	l.Debug("set logger level", zap.String("level", level.String()))
	return
}

// DebugSample emit debug log with propability sample/SampleRateDenominator.
// sample could be [0, 1000], less than 0 means never, great than 1000 means certainly
func (l *LoggerT) DebugSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Debug(msg, fields...)
}

// InfoSample emit info log with propability sample/SampleRateDenominator
func (l *LoggerT) InfoSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Info(msg, fields...)
}

// WarnSample emit warn log with propability sample/SampleRateDenominator
func (l *LoggerT) WarnSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Warn(msg, fields...)
}

// Named adds a new path segment to the logger's name. Segments are joined by
// periods. By default, Loggers are unnamed.
func (l *LoggerT) Named(s string) *LoggerT {
	return &LoggerT{
		Logger: l.Logger.Named(s),
		level:  l.level,
	}
}

// With creates a child logger and adds structured context to it. Fields added
// to the child don't affect the parent, and vice versa.
func (l *LoggerT) With(fields ...zapcore.Field) *LoggerT {
	return &LoggerT{
		Logger: l.Logger.With(fields...),
		level:  l.level,
	}
}

// WithOptions clones the current Logger, applies the supplied Options, and
// returns the resulting Logger. It's safe to use concurrently.
func (l *LoggerT) WithOptions(opts ...zap.Option) *LoggerT {
	return &LoggerT{
		Logger: l.Logger.WithOptions(opts...),
		level:  l.level,
	}
}

func init() {
	level := Level(os.Getenv("GUTILS_LOGGER_LEVEL"))
	switch level {
	case LevelInfo,
		LevelDebug,
		LevelWarn,
		LevelError,
		LevelFatal,
		LevelPanic:
	default:
		level = LevelInfo
	}

	var err error
	if Shared, err = NewConsoleWithName("go-utils", level); err != nil {
		panic(fmt.Sprintf("create logger: %+v", err))
	}

	// Shared.Info("create logger", zap.String("level", "info"))
}
