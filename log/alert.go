package log

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/graphql"
	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/buffer"
	"github.com/Laisky/zap/zapcore"
)

// alertMutation defines the GraphQL mutation for sending alerts.
type alertMutation struct {
	TelegramMonitorAlert struct {
		Name graphql.String
	} `graphql:"TelegramMonitorAlert(type: $type, token: $token, msg: $msg)"`
}

// RateLimiter defines an interface for rate limiting alert sending.
type RateLimiter interface {
	// Allow check if allow to send alert
	Allow() bool
}

// Alert sends alerts to Laisky's alert API.
// See: https://github.com/Laisky/laisky-blog-graphql/tree/master/telegram
type Alert struct {
	*alertOption
	cli        *graphql.Client
	stopChan   chan struct{}
	senderChan chan *alertMsg
	pushAPI    string
}

// alertOption holds configuration options for the Alert hook.
type alertOption struct {
	encPool     *sync.Pool
	level       zapcore.LevelEnabler
	timeout     time.Duration
	alertType   string
	alertToken  string
	ratelimiter RateLimiter
}

// applyOpts applies the given AlertOptions to the alertOption.
func (o *alertOption) applyOpts(opts ...AlertOption) (*alertOption, error) {
	// fill default
	o.encPool = &sync.Pool{
		New: func() any {
			return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
		},
	}
	o.level = defaultAlertHookLevel
	o.timeout = defaultAlertPusherTimeout

	// apply options
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// AlertOption is a function that configures an Alert hook.
type AlertOption func(*alertOption) error

// WithAlertHookLevel sets the minimum log level that triggers the Alert hook.
func WithAlertHookLevel(level zapcore.Level) AlertOption {
	return func(o *alertOption) error {
		if level.Enabled(zap.DebugLevel) {
			// Because Alert will use `debug` logger,
			// hook with debug will cause infinite recursive
			return errors.Errorf("level should higher than debug")
		}
		if level.Enabled(zap.WarnLevel) {
			Shared.Warn("level is better higher than warn")
		}
		o.level = level
		return nil
	}
}

// WithAlertPushTimeout sets the HTTP timeout for pushing alerts.
func WithAlertPushTimeout(timeout time.Duration) AlertOption {
	return func(o *alertOption) error {
		o.timeout = timeout
		return nil
	}
}

// WithAlertType sets the alert type for the hook.
func WithAlertType(alertType string) AlertOption {
	return func(o *alertOption) error {
		alertType = strings.TrimSpace(alertType)
		if alertType == "" {
			return errors.Errorf("alertType should not be empty")
		}
		o.alertType = alertType
		return nil
	}
}

// WithAlertToken sets the alert token for the hook.
func WithAlertToken(token string) AlertOption {
	return func(o *alertOption) error {
		token = strings.TrimSpace(token)
		if token == "" {
			return errors.Errorf("token should not be empty")
		}
		o.alertToken = token
		return nil
	}
}

// WithRateLimiter sets the rate limiter for the hook.
func WithRateLimiter(rl RateLimiter) AlertOption {
	return func(o *alertOption) error {
		o.ratelimiter = rl
		return nil
	}
}

// alertMsg represents a message to be sent as an alert.
type alertMsg struct {
	alertType, pushToken, msg string
}

// NewAlert creates a new Alert hook.
//
// It's better to set an ratelimiter by WithRateLimiter
// to avoid sending too many alerts.
func NewAlert(ctx context.Context, pushAPI string,
	opts ...AlertOption) (a *Alert, err error) {
	Shared.Debug("create new Alert")
	if pushAPI == "" {
		return nil, errors.Errorf("pushAPI should not be empty")
	}
	if ctx == nil {
		return nil, errors.Errorf("ctx should not be nil")
	}

	opt, err := new(alertOption).applyOpts(opts...)
	if err != nil {
		return nil, err
	}

	a = &Alert{
		alertOption: opt,
		stopChan:    make(chan struct{}),
		senderChan:  make(chan *alertMsg, defaultAlertPusherBufSize),
		pushAPI:     pushAPI,
	}

	a.cli = graphql.NewClient(a.pushAPI, &http.Client{
		Timeout: a.timeout,
	})

	go a.runSender(ctx)
	return a, nil
}

// Close closes the Alert hook.
func (a *Alert) Close() {
	close(a.stopChan) // should close stopChan first
	close(a.senderChan)
}

// SendWithType sends an alert with the specified type, token, and message.
func (a *Alert) SendWithType(alertType, pushToken, msg string) (err error) {
	if alertType == "" || pushToken == "" || msg == "" {
		return errors.Errorf("alertType, pushToken and msg should not be empty")
	}

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

// runSender runs the alert sender goroutine.
func (a *Alert) runSender(ctx context.Context) {
	var (
		ok      bool
		payload *alertMsg
		err     error
		query   = new(alertMutation)
		vars    = map[string]any{}
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

		// check ratelimiter
		if a.alertOption.ratelimiter != nil && !a.alertOption.ratelimiter.Allow() {
			Shared.Debug("exceed rate limit, skip alert",
				zap.String("alert", payload.alertType),
				zap.String("msg", payload.msg))
			continue
		}

		vars["type"] = graphql.String(payload.alertType)
		vars["token"] = graphql.String(payload.pushToken)
		vars["msg"] = graphql.String(payload.msg)

		ctxReq, cancel := context.WithTimeout(ctx, time.Second*30)
		if err = a.cli.Mutate(ctxReq, query, vars); err != nil {
			Shared.Warn("send alert mutation failed",
				zap.String("api", a.pushAPI),
				zap.String("type", payload.alertType),
				zap.Error(err))
			cancel()
			continue
		}
		cancel()

		Shared.Debug("send telegram msg",
			zap.String("alert", payload.alertType),
			zap.String("msg", payload.msg))
	}
}

// Send sends an alert with the default alertType and pushToken.
func (a *Alert) Send(msg string) (err error) {
	return a.SendWithType(a.alertType, a.alertToken, msg)
}

// GetZapHook returns a Zap hook that sends alerts for log entries.
func (a *Alert) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
	return func(e zapcore.Entry, fs []zapcore.Field) (err error) {
		if !a.level.Enabled(e.Level) {
			return nil
		}

		var bb *buffer.Buffer
		enci := a.encPool.Get()
		enc, ok := enci.(zapcore.Encoder)
		if !ok {
			return errors.Errorf("unknown type for encoder %T", enci)
		}

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
