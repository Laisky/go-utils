package utils

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/Laisky/zap/buffer"

	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/pkg/errors"
	"github.com/shurcooL/graphql"
)

var (
	/*Logger logging tool.

	* Info(msg string, fields ...Field)
	* Debug(msg string, fields ...Field)
	* Warn(msg string, fields ...Field)
	* Error(msg string, fields ...Field)
	* Panic(msg string, fields ...Field)
	* DebugSample(sample int, msg string, fields ...zapcore.Field)
	* InfoSample(sample int, msg string, fields ...zapcore.Field)
	* WarnSample(sample int, msg string, fields ...zapcore.Field)
	 */
	Logger *LoggerType
)

// SampleRateDenominator sample rate = sample / SampleRateDenominator
const SampleRateDenominator = 1000

// LoggerType extend from zap.Logger
type LoggerType struct {
	*zap.Logger
	level zap.AtomicLevel
}

// SetDefaultLogger set default utils.Logger
func SetDefaultLogger(name, level string, opts ...zap.Option) (l *LoggerType, err error) {
	if l, err = NewLoggerWithName(name, level, opts...); err != nil {
		return nil, errors.Wrap(err, "create new logger")
	}

	Logger = l
	return Logger, nil
}

// NewLogger create new logger
func NewLogger(level string, opts ...zap.Option) (l *LoggerType, err error) {
	return NewLoggerWithName("", level, opts...)
}

// NewLoggerWithName create new logger with name
func NewLoggerWithName(name, level string, opts ...zap.Option) (l *LoggerType, err error) {
	zl := zap.NewAtomicLevel()
	cfg := zap.Config{
		Level:            zl,
		Development:      false,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	cfg.EncoderConfig.MessageKey = "message"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zapLogger, err := cfg.Build(opts...)
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %+v", err)
	}
	zapLogger = zapLogger.Named(name)

	l = &LoggerType{
		Logger: zapLogger,
		level:  zl,
	}
	return l, l.ChangeLevel(level)
}

// ChangeLevel change logger level
func (l *LoggerType) ChangeLevel(level string) (err error) {
	switch level {
	case "debug":
		l.level.SetLevel(zap.DebugLevel)
	case "info":
		l.level.SetLevel(zap.InfoLevel)
	case "warn":
		l.level.SetLevel(zap.WarnLevel)
	case "error":
		l.level.SetLevel(zap.ErrorLevel)
	default:
		return fmt.Errorf("log level only be debug/info/warn/error")
	}

	return
}

// DebugSample emit debug log with propability sample/SampleRateDenominator.
// sample could be [0, 1000], less than 0 means never, great than 1000 means certainly
func (l *LoggerType) DebugSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Debug(msg, fields...)
}

// InfoSample emit info log with propability sample/SampleRateDenominator
func (l *LoggerType) InfoSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Info(msg, fields...)
}

// WarnSample emit warn log with propability sample/SampleRateDenominator
func (l *LoggerType) WarnSample(sample int, msg string, fields ...zapcore.Field) {
	if rand.Intn(SampleRateDenominator) > sample {
		return
	}

	l.Warn(msg, fields...)
}

// With creates a child logger and adds structured context to it. Fields added
// to the child don't affect the parent, and vice versa.
func (l *LoggerType) With(fields ...zapcore.Field) *LoggerType {
	return &LoggerType{
		Logger: l.Logger.With(fields...),
		level:  l.level,
	}
}

// WithOptions clones the current Logger, applies the supplied Options, and
// returns the resulting Logger. It's safe to use concurrently.
func (l *LoggerType) WithOptions(opts ...zap.Option) *LoggerType {
	return &LoggerType{
		Logger: l.Logger.WithOptions(opts...),
		level:  l.level,
	}
}

func init() {
	var err error
	if Logger, err = NewLogger("info"); err != nil {
		panic(fmt.Sprintf("create logger: %+v", err))
	}

	Logger.Info("create logger", zap.String("level", "info"))
}

type alertMutation struct {
	TelegramMonitorAlert struct {
		Name graphql.String
	} `graphql:"TelegramMonitorAlert(type: $type, token: $token, msg: $msg)"`
}

// AlertPusher send alert to laisky's alert API
//
// https://github.com/Laisky/laisky-blog-graphql/tree/master/telegram
type AlertPusher struct {
	cli        *graphql.Client
	stopChan   chan struct{}
	senderChan chan *alertMsg

	token, alertType string

	pushAPI string
	timeout time.Duration
}

type alertMsg struct {
	alertType,
	pushToken,
	msg string
}

const (
	defaultAlertPusherTimeout = 10 * time.Second
)

// AlertPushOption is AlertPusher's options
type AlertPushOption func(*AlertPusher)

// WithAlertPushTimeout set AlertPusher HTTP timeout
func WithAlertPushTimeout(timeout time.Duration) AlertPushOption {
	return func(a *AlertPusher) {
		a.timeout = timeout
	}
}

// NewAlertPusher create new AlertPusher
func NewAlertPusher(ctx context.Context, pushAPI string, opts ...AlertPushOption) (a *AlertPusher, err error) {
	Logger.Debug("create new AlertPusher", zap.String("pushAPI", pushAPI))
	if pushAPI == "" {
		return nil, fmt.Errorf("pushAPI should nout empty")
	}

	a = &AlertPusher{
		stopChan:   make(chan struct{}),
		senderChan: make(chan *alertMsg, 100),

		timeout: defaultAlertPusherTimeout,
		pushAPI: pushAPI,
	}
	for _, opt := range opts {
		opt(a)
	}

	a.cli = graphql.NewClient(a.pushAPI, &http.Client{
		Timeout: a.timeout,
	})

	go a.runSender(ctx)
	return a, nil
}

// NewAlertPusherWithAlertType create new AlertPusher with default type and token
func NewAlertPusherWithAlertType(ctx context.Context, pushAPI string, alertType, pushToken string, opts ...AlertPushOption) (a *AlertPusher, err error) {
	Logger.Debug("create new AlertPusher with alert type", zap.String("pushAPI", pushAPI), zap.String("type", alertType))
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
		return fmt.Errorf("send channel overflow")
	}

	return nil
}

func (a *AlertPusher) runSender(ctx context.Context) {
	var (
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
		case payload = <-a.senderChan:
			if payload == nil {
				return
			}
		}

		// only allow use debug level logger
		Logger.Debug("send alert", zap.String("type", payload.alertType))
		vars["type"] = graphql.String(payload.alertType)
		vars["token"] = graphql.String(payload.pushToken)
		vars["msg"] = graphql.String(payload.msg)
		if err = a.cli.Mutate(ctx, query, vars); err != nil {
			Logger.Debug("send alert mutation", zap.Error(err))
		}

		Logger.Debug("send telegram msg",
			zap.String("alert", payload.alertType),
			zap.String("msg", payload.msg))
	}
}

// Send send with default alertType and pushToken
func (a *AlertPusher) Send(msg string) (err error) {
	return a.SendWithType(a.alertType, a.token, msg)
}

const (
	defaultAlertHookLevel = zapcore.ErrorLevel
)

// AlertHook hook for zap.Logger
type AlertHook struct {
	pusher  *AlertPusher
	encPool *sync.Pool
	level   zapcore.LevelEnabler
}

// AlertHookOption option for create AlertHook
type AlertHookOption func(*AlertHook)

// WithAlertHookLevel level to trigger AlertHook
func WithAlertHookLevel(level zapcore.Level) AlertHookOption {
	if level.Enabled(zap.DebugLevel) {
		// because AlertPusher will use `debug` logger,
		// hook with debug will cause infinite recursive
		Logger.Panic("level should higher than debug")
	}

	return func(a *AlertHook) {
		a.level = level
	}
}

// NewAlertHook create AlertHook
func NewAlertHook(pusher *AlertPusher, opts ...AlertHookOption) (a *AlertHook) {
	a = &AlertHook{
		encPool: &sync.Pool{
			New: func() interface{} {
				return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
			},
		},
		pusher: pusher,
		level:  defaultAlertHookLevel,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// GetZapHook get hook for zap logger
func (a *AlertHook) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(e zapcore.Entry, fs []zapcore.Field) (err error) {
		if !a.level.Enabled(e.Level) {
			return nil
		}

		var bb *buffer.Buffer
		enc := a.encPool.Get().(zapcore.Encoder)
		bb, err = enc.EncodeEntry(e, fs)
		if err != nil {
			return errors.Wrap(err, "zapcore encode fields")
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
		return a.pusher.Send(msg)
	}
}
