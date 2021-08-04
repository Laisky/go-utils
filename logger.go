package utils

import (
	"bytes"
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

type LoggerLevel string

func (l LoggerLevel) String() string {
	return string(l)
}

const (
	// SampleRateDenominator sample rate = sample / SampleRateDenominator
	SampleRateDenominator = 1000

	defaultAlertPusherTimeout = 10 * time.Second
	defaultAlertPusherBufSize = 20
	defaultAlertHookLevel     = zapcore.ErrorLevel

	// LoggerLevelInfo Logger level info
	LoggerLevelInfo LoggerLevel = "info"
	// LoggerLevelDebug Logger level debug
	LoggerLevelDebug LoggerLevel = "debug"
	// LoggerLevelWarn Logger level warn
	LoggerLevelWarn LoggerLevel = "warn"
	// LoggerLevelError Logger level error
	LoggerLevelError LoggerLevel = "error"
	// LoggerLevelFatal Logger level fatal
	LoggerLevelFatal LoggerLevel = "fatal"
	// LoggerLevelPanic Logger level panic
	LoggerLevelPanic LoggerLevel = "panic"
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

// LoggerType extend from zap.Logger
type LoggerType struct {
	*zap.Logger
	level zap.AtomicLevel
}

// CreateNewDefaultLogger set default utils.Logger
func CreateNewDefaultLogger(name string, level LoggerLevel, opts ...zap.Option) (l *LoggerType, err error) {
	if l, err = NewLoggerWithName(name, level, opts...); err != nil {
		return nil, errors.Wrap(err, "create new logger")
	}

	Logger = l
	return Logger, nil
}

// NewLoggerWithName create new logger with name
func NewLoggerWithName(name string, level LoggerLevel, opts ...zap.Option) (l *LoggerType, err error) {
	return NewLogger(
		WithLoggerName(name),
		WithLoggerEncoding(LoggerEncodingJSON),
		WithLoggerLevel(level),
		WithLoggerZapOptions(opts...),
	)
}

// NewConsoleLoggerWithName create new logger with name
func NewConsoleLoggerWithName(name string, level LoggerLevel, opts ...zap.Option) (l *LoggerType, err error) {
	return NewLogger(
		WithLoggerName(name),
		WithLoggerEncoding(LoggerEncodingConsole),
		WithLoggerLevel(level),
		WithLoggerZapOptions(opts...),
	)
}

type LoggerConfig struct {
	zap.Config
	zapOptions []zap.Option
	Name       string
}

type LoggerEncoding string

const (
	LoggerEncodingConsole = "console"
	LoggerEncodingJSON    = "json"
)

type LoggerOption func(l *LoggerConfig) error

// WithLoggerOutputPaths set output path
//
// like "stdout"
func WithLoggerOutputPaths(paths []string) LoggerOption {
	return func(c *LoggerConfig) error {
		c.OutputPaths = paths
		return nil
	}
}

// WithLoggerErrorOutputPaths set error logs output path
//
// like "stderr"
func WithLoggerErrorOutputPaths(paths []string) LoggerOption {
	return func(c *LoggerConfig) error {
		c.ErrorOutputPaths = paths
		return nil
	}
}

func WithLoggerEncoding(format LoggerEncoding) LoggerOption {
	return func(c *LoggerConfig) error {
		switch format {
		case LoggerEncodingConsole:
			c.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		case LoggerEncodingJSON:
		default:
			return errors.Errorf("invalid format: %s", format)
		}

		return nil
	}
}

func WithLoggerZapOptions(opts ...zap.Option) LoggerOption {
	return func(c *LoggerConfig) error {
		c.zapOptions = opts
		return nil
	}
}

func WithLoggerName(name string) LoggerOption {
	return func(c *LoggerConfig) error {
		c.Name = name
		return nil
	}
}

func WithLoggerLevel(level LoggerLevel) LoggerOption {
	return func(c *LoggerConfig) error {
		switch level {
		case LoggerLevelInfo:
			c.Level.SetLevel(zap.InfoLevel)
		case LoggerLevelDebug:
			c.Level.SetLevel(zap.DebugLevel)
		case LoggerLevelWarn:
			c.Level.SetLevel(zap.WarnLevel)
		case LoggerLevelError:
			c.Level.SetLevel(zap.ErrorLevel)
		case LoggerLevelFatal:
			c.Level.SetLevel(zap.FatalLevel)
		case LoggerLevelPanic:
			c.Level.SetLevel(zap.PanicLevel)
		default:
			return errors.Errorf("invalid level: %s", level)
		}

		return nil
	}
}

// NewLogger create new logger
func NewLogger(optfs ...LoggerOption) (l *LoggerType, err error) {
	opt := &LoggerConfig{
		Name: "app",
		Config: zap.Config{
			Level:            zap.NewAtomicLevel(),
			Development:      false,
			Encoding:         string(LoggerEncodingConsole),
			EncoderConfig:    zap.NewProductionEncoderConfig(),
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		},
	}
	opt.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	opt.EncoderConfig.MessageKey = "message"
	opt.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder

	for _, optf := range optfs {
		if err = optf(opt); err != nil {
			return nil, errors.Wrap(err, "create logger")
		}
	}

	zapLogger, err := opt.Build(opt.zapOptions...)
	if err != nil {
		return nil, fmt.Errorf("build zap logger: %+v", err)
	}
	zapLogger = zapLogger.Named(opt.Name)

	l = &LoggerType{
		Logger: zapLogger,
		level:  opt.Level,
	}

	return l, nil
}

// Level get current level of logger
func (l *LoggerType) Level() LoggerLevel {
	return LoggerLevel(l.level.String())
}

// ChangeLevel change logger level
func (l *LoggerType) ChangeLevel(level LoggerLevel) (err error) {
	switch level {
	case LoggerLevelDebug:
		l.level.SetLevel(zap.DebugLevel)
	case LoggerLevelInfo:
		l.level.SetLevel(zap.InfoLevel)
	case LoggerLevelWarn:
		l.level.SetLevel(zap.WarnLevel)
	case LoggerLevelError:
		l.level.SetLevel(zap.ErrorLevel)
	case LoggerLevelPanic:
		l.level.SetLevel(zap.PanicLevel)
	case LoggerLevelFatal:
		l.level.SetLevel(zap.FatalLevel)
	default:
		return errors.Errorf("invalid level %s", string(level))
	}

	l.Debug("set logger level", zap.String("level", level.String()))
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

// Clone clone new Logger that inherit all config
func (l *LoggerType) Clone() *LoggerType {
	return &LoggerType{
		Logger: l.Logger.With(),
		level:  l.level,
	}
}

// Named adds a new path segment to the logger's name. Segments are joined by
// periods. By default, Loggers are unnamed.
func (l *LoggerType) Named(s string) *LoggerType {
	return &LoggerType{
		Logger: l.Logger.Named(s),
		level:  l.level,
	}
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
	if Logger, err = NewConsoleLoggerWithName("go-utils", "info"); err != nil {
		panic(fmt.Sprintf("create logger: %+v", err))
	}

	Logger.Info("create logger", zap.String("level", "info"))
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

func newAlertHookOpt() *alertHookOption {
	return &alertHookOption{
		encPool: &sync.Pool{
			New: func() interface{} {
				return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
			},
		},
		level:   defaultAlertHookLevel,
		timeout: defaultAlertPusherTimeout,
	}
}

// AlertHookOptFunc option for create AlertHook
type AlertHookOptFunc func(*alertHookOption)

// WithAlertHookLevel level to trigger AlertHook
func WithAlertHookLevel(level zapcore.Level) AlertHookOptFunc {
	if level.Enabled(zap.DebugLevel) {
		// because AlertPusher will use `debug` logger,
		// hook with debug will cause infinite recursive
		Logger.Panic("level should higher than debug")
	}
	if level.Enabled(zap.WarnLevel) {
		Logger.Warn("level is better higher than warn")
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
	Logger.Debug("create new AlertPusher", zap.String("pushAPI", pushAPI))
	if pushAPI == "" {
		return nil, fmt.Errorf("pushAPI should nout empty")
	}

	opt := newAlertHookOpt()
	for _, optf := range opts {
		optf(opt)
	}

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
func NewAlertPusherWithAlertType(ctx context.Context, pushAPI string, alertType, pushToken string, opts ...AlertHookOptFunc) (a *AlertPusher, err error) {
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

// GetZapHook get hook for zap logger
func (a *AlertPusher) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(e zapcore.Entry, fs []zapcore.Field) (err error) {
		if !a.level.Enabled(e.Level) {
			return nil
		}

		var bb *buffer.Buffer
		enc := a.encPool.Get().(zapcore.Encoder)
		if bb, err = enc.EncodeEntry(e, fs); err != nil {
			Logger.Debug("zapcore encode fields got error", zap.Error(err))
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
			Logger.Debug("send alert got error", zap.Error(err))
			return nil
		}

		return nil
	}
}

type pateoAlertMsg struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Time    string `json:"time"`
}

// PateoAlertPusher alert pusher for pateo wechat service
type PateoAlertPusher struct {
	*alertHookOption
	cli        *http.Client
	api, token string

	senderBufChan chan *pateoAlertMsg
}

// NewPateoAlertPusher create new PateoAlertPusher
func NewPateoAlertPusher(ctx context.Context, api, token string, opts ...AlertHookOptFunc) (p *PateoAlertPusher, err error) {
	opt := newAlertHookOpt()
	for _, optf := range opts {
		optf(opt)
	}

	p = &PateoAlertPusher{
		alertHookOption: opt,
		api:             api,
		token:           token,
		cli: &http.Client{
			Timeout: opt.timeout,
		},
	}

	p.senderBufChan = make(chan *pateoAlertMsg, defaultAlertPusherBufSize)
	go p.runSender(ctx)

	return
}

func (p *PateoAlertPusher) runSender(ctx context.Context) {
	var (
		ok   bool
		msg  *pateoAlertMsg
		req  *http.Request
		resp *http.Response
		jb   []byte
		err  error
	)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok = <-p.senderBufChan:
			if !ok {
				return
			}
		}

		if jb, err = json.Marshal(msg); err != nil {
			Logger.Debug("marshal msg to json", zap.Error(err))
			continue
		}
		if req, err = http.NewRequest("POST", p.api, bytes.NewBuffer(jb)); err != nil {
			Logger.Debug("make pateo alert request", zap.Error(err))
			continue
		}
		req.Header.Add(HTTPHeaderContentType, HTTPHeaderContentTypeValJSON)
		req.Header.Add("Authorization", "Bearer "+p.token)
		if resp, err = p.cli.Do(req); err != nil {
			Logger.Debug("http post pateo alert server", zap.Error(err))
			continue
		}
		if err = CheckResp(resp); err != nil {
			Logger.Debug("pateo alert server return error", zap.Error(err))
			continue
		}
	}
}

// Send send alert msg
func (p *PateoAlertPusher) Send(title, content string, ts time.Time) (err error) {
	select {
	case p.senderBufChan <- &pateoAlertMsg{
		Title:   title,
		Content: content,
		Time:    ts.Format(time.RFC3339Nano),
	}:
		return nil
	default:
		return fmt.Errorf("sender chan overflow")
	}
}

// GetZapHook get hook for zap logger
func (p *PateoAlertPusher) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(e zapcore.Entry, fs []zapcore.Field) (err error) {
		if !p.level.Enabled(e.Level) {
			return nil
		}

		var bb *buffer.Buffer
		enc := p.encPool.Get().(zapcore.Encoder)
		if bb, err = enc.EncodeEntry(e, fs); err != nil {
			Logger.Debug("zapcore encode fields got error", zap.Error(err))
			return nil
		}
		fsb := bb.String()
		bb.Reset()
		p.encPool.Put(enc)
		msg := "logger: " + e.LoggerName + "\n" +
			"time: " + e.Time.Format(time.RFC3339Nano) + "\n" +
			"level: " + e.Level.String() + "\n" +
			"caller: " + e.Caller.FullPath() + "\n" +
			"stack: " + e.Stack + "\n" +
			"message: " + e.Message + "\n" +
			fsb

		if err = p.Send(e.LoggerName+":"+e.Message, msg, e.Time); err != nil {
			Logger.Debug("send alert got error", zap.Error(err))
			return nil
		}

		return nil
	}
}
