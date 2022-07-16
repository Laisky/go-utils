// Package log enhanced zap logger
package log

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/graphql"
	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/buffer"
	"github.com/Laisky/zap/zapcore"
	"github.com/pkg/errors"
)

const (
	// SampleRateDenominator sample rate = sample / SampleRateDenominator
	SampleRateDenominator = 1000

	defaultAlertPusherTimeout = 10 * time.Second
	defaultAlertPusherBufSize = 20
	defaultAlertHookLevel     = zapcore.ErrorLevel
)

var (
	/*Shared logging tool.

	* Info(msg string, fields ...Field)
	* Debug(msg string, fields ...Field)
	* Warn(msg string, fields ...Field)
	* Error(msg string, fields ...Field)
	* Panic(msg string, fields ...Field)
	* DebugSample(sample int, msg string, fields ...zapcore.Field)
	* InfoSample(sample int, msg string, fields ...zapcore.Field)
	* WarnSample(sample int, msg string, fields ...zapcore.Field)
	 */
	Shared Logger
)

type Level string

func (l Level) String() string {
	return string(l)
}

func (l Level) Zap() zapcore.Level {
	zl, err := LevelToZap(l)
	if err != nil {
		panic(err)
	}

	return zl
}

const (
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

type Logger interface {
	zapLoggerItf
	Level() Level
	ChangeLevel(level Level) (err error)
	DebugSample(sample int, msg string, fields ...zapcore.Field)
	InfoSample(sample int, msg string, fields ...zapcore.Field)
	WarnSample(sample int, msg string, fields ...zapcore.Field)
	Named(s string) Logger
	With(fields ...zapcore.Field) Logger
	WithOptions(opts ...zap.Option) Logger
}

// logger extend from zap.Logger
type logger struct {
	*zap.Logger

	// level level of current logger
	//
	// zap logger do not expose api to change log's level,
	// so we have to save the pointer of zap.AtomicLevel.
	level zap.AtomicLevel
}

// NewWithName create new logger with name
func NewWithName(name string, level Level, opts ...zap.Option) (l Logger, err error) {
	return New(
		WithName(name),
		WithEncoding(EncodingJSON),
		WithLevel(level),
		WithZapOptions(opts...),
	)
}

// NewConsoleWithName create new logger with name
func NewConsoleWithName(name string, level Level, opts ...zap.Option) (l Logger, err error) {
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

type Encoding string

func (e Encoding) String() string {
	return string(e)
}

const (
	EncodingConsole Encoding = "console"
	EncodingJSON    Encoding = "json"
)

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
func New(optfs ...Option) (l Logger, err error) {
	opt, err := new(option).fillDefault().applyOpts(optfs...)
	if err != nil {
		return nil, err
	}

	zapLogger, err := opt.Build(opt.zapOptions...)
	if err != nil {
		return nil, errors.Errorf("build zap logger: %+v", err)
	}
	zapLogger = zapLogger.Named(opt.Name)

	l = &logger{
		Logger: zapLogger,
		level:  opt.Level,
	}

	return l, nil
}

// Level get current level of logger
func (l *logger) Level() Level {
	lvl, err := LevelFromZap(l.level.Level())
	if err != nil {
		panic(err)
	}

	return lvl
}

// Zap return internal z*ap.Logger
func (l *logger) Zap() *zap.Logger {
	return l.Logger
}

// ChangeLevel change logger level
//
// Because all children loggers share the same level as their parent logger,
// if you modify one logger's level, it will affect all of its parent and children loggers.
func (l *logger) ChangeLevel(level Level) (err error) {
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
func (l *logger) DebugSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Debug(msg, fields...)
}

// InfoSample emit info log with propability sample/SampleRateDenominator
func (l *logger) InfoSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Info(msg, fields...)
}

// WarnSample emit warn log with propability sample/SampleRateDenominator
func (l *logger) WarnSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Warn(msg, fields...)
}

// Named adds a new path segment to the logger's name. Segments are joined by
// periods. By default, Loggers are unnamed.
func (l *logger) Named(s string) Logger {
	return &logger{
		Logger: l.Logger.Named(s),
		level:  l.level,
	}
}

// With creates a child logger and adds structured context to it. Fields added
// to the child don't affect the parent, and vice versa.
func (l *logger) With(fields ...zapcore.Field) Logger {
	return &logger{
		Logger: l.Logger.With(fields...),
		level:  l.level,
	}
}

// WithOptions clones the current Logger, applies the supplied Options, and
// returns the resulting Logger. It's safe to use concurrently.
func (l *logger) WithOptions(opts ...zap.Option) Logger {
	return &logger{
		Logger: l.Logger.WithOptions(opts...),
		level:  l.level,
	}
}

func init() {
	var err error
	if Shared, err = NewConsoleWithName("go-utils", "info"); err != nil {
		panic(fmt.Sprintf("create logger: %+v", err))
	}

	Shared.Info("create logger", zap.String("level", "info"))
}

// ================================
// alert pusher hook
// ================================

type alertMutation struct {
	TelegramMonitorAlert struct {
		Name graphql.String
	} `graphql:"TelegramMonitorAlert(type: $type, token: $token, msg: $msg)"`
}

// AlertPusher send alert to laisky's alert API
//
// https://github.com/Laisky/laisky-blog-graphql/tree/master/telegram
type AlertPusher struct {
	*alertHookOption

	cli        *graphql.Client
	stopChan   chan struct{}
	senderChan chan *alertMsg

	token, alertType,
	pushAPI string
}

type alertHookOption struct {
	encPool *sync.Pool
	level   zapcore.LevelEnabler
	timeout time.Duration
}

func (o *alertHookOption) fillDefault() *alertHookOption {
	o.encPool = &sync.Pool{
		New: func() interface{} {
			return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
		},
	}
	o.level = defaultAlertHookLevel
	o.timeout = defaultAlertPusherTimeout
	return o
}

func (o *alertHookOption) applyOpts(opts ...AlertHookOptFunc) *alertHookOption {
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// AlertHookOptFunc option for create AlertHook
type AlertHookOptFunc func(*alertHookOption)

// WithAlertHookLevel level to trigger AlertHook
func WithAlertHookLevel(level zapcore.Level) AlertHookOptFunc {
	if level.Enabled(zap.DebugLevel) {
		// because AlertPusher will use `debug` logger,
		// hook with debug will cause infinite recursive
		Shared.Panic("level should higher than debug")
	}
	if level.Enabled(zap.WarnLevel) {
		Shared.Warn("level is better higher than warn")
	}

	return func(a *alertHookOption) {
		a.level = level
	}
}

// WithAlertPushTimeout set AlertPusher HTTP timeout
func WithAlertPushTimeout(timeout time.Duration) AlertHookOptFunc {
	return func(a *alertHookOption) {
		a.timeout = timeout
	}
}

type alertMsg struct {
	alertType,
	pushToken,
	msg string
}

// NewAlertPusher create new AlertPusher
func NewAlertPusher(ctx context.Context, pushAPI string, opts ...AlertHookOptFunc) (a *AlertPusher, err error) {
	Shared.Debug("create new AlertPusher", zap.String("pushAPI", pushAPI))
	if pushAPI == "" {
		return nil, errors.Errorf("pushAPI should nout empty")
	}

	opt := new(alertHookOption).fillDefault().applyOpts(opts...)
	a = &AlertPusher{
		alertHookOption: opt,
		stopChan:        make(chan struct{}),
		senderChan:      make(chan *alertMsg, defaultAlertPusherBufSize),

		pushAPI: pushAPI,
	}

	a.cli = graphql.NewClient(a.pushAPI, &http.Client{
		Timeout: a.timeout,
	})

	go a.runSender(ctx)
	return a, nil
}

// NewAlertPusherWithAlertType create new AlertPusher with default type and token
func NewAlertPusherWithAlertType(ctx context.Context,
	pushAPI string,
	alertType,
	pushToken string,
	opts ...AlertHookOptFunc,
) (a *AlertPusher, err error) {
	Shared.Debug("create new AlertPusher with alert type",
		zap.String("pushAPI", pushAPI),
		zap.String("type", alertType))
	if a, err = NewAlertPusher(ctx, pushAPI, opts...); err != nil {
		return nil, err
	}

	a.alertType = alertType
	a.token = pushToken
	return a, nil
}

// Close close AlertPusher
func (a *AlertPusher) Close() {
	close(a.stopChan) // should close stopChan first
	close(a.senderChan)
}

// SendWithType send alert with specific type, token and msg
func (a *AlertPusher) SendWithType(alertType, pushToken, msg string) (err error) {
	select {
	case a.senderChan <- &alertMsg{
		alertType: alertType,
		pushToken: pushToken,
		msg:       msg,
	}:
	default:
		return errors.Errorf("send channel overflow")
	}

	return nil
}

func (a *AlertPusher) runSender(ctx context.Context) {
	var (
		ok      bool
		payload *alertMsg
		err     error
		query   = new(alertMutation)
		vars    = map[string]interface{}{}
	)
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopChan:
			return
		case payload, ok = <-a.senderChan:
			if !ok {
				return
			}
		}

		// only allow use debug level logger
		Shared.Debug("send alert", zap.String("type", payload.alertType))
		vars["type"] = graphql.String(payload.alertType)
		vars["token"] = graphql.String(payload.pushToken)
		vars["msg"] = graphql.String(payload.msg)
		if err = a.cli.Mutate(ctx, query, vars); err != nil {
			Shared.Debug("send alert mutation", zap.Error(err))
		}

		Shared.Debug("send telegram msg",
			zap.String("alert", payload.alertType),
			zap.String("msg", payload.msg))
	}
}

// Send send with default alertType and pushToken
func (a *AlertPusher) Send(msg string) (err error) {
	return a.SendWithType(a.alertType, a.token, msg)
}

// GetZapHook get hook for zap logger
func (a *AlertPusher) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(e zapcore.Entry, fs []zapcore.Field) (err error) {
		if !a.level.Enabled(e.Level) {
			return nil
		}

		var bb *buffer.Buffer
		enc := a.encPool.Get().(zapcore.Encoder)
		if bb, err = enc.EncodeEntry(e, fs); err != nil {
			Shared.Debug("zapcore encode fields got error", zap.Error(err))
			return nil
		}
		fsb := bb.String()
		bb.Reset()
		a.encPool.Put(enc)

		msg := "logger: " + e.LoggerName + "\n" +
			"time: " + e.Time.Format(time.RFC3339Nano) + "\n" +
			"level: " + e.Level.String() + "\n" +
			"caller: " + e.Caller.FullPath() + "\n" +
			"stack: " + e.Stack + "\n" +
			"message: " + e.Message + "\n" +
			fsb
		if err = a.Send(msg); err != nil {
			Shared.Debug("send alert got error", zap.Error(err))
			return nil
		}

		return nil
	}
}
