package log

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors"
	"github.com/Laisky/graphql"
	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/buffer"
	"github.com/Laisky/zap/zapcore"
)

// ================================
// alert pusher hook
// ================================

type alertMutation struct {
	TelegramMonitorAlert struct {
		Name graphql.String
	} `graphql:"TelegramMonitorAlert(type: $type, token: $token, msg: $msg)"`
}

// Alert send alert to laisky's alert API
//
// https://github.com/Laisky/laisky-blog-graphql/tree/master/telegram
type Alert struct {
	*alertOption

	cli        *graphql.Client
	stopChan   chan struct{}
	senderChan chan *alertMsg

	token, alertType,
	pushAPI string
}

type alertOption struct {
	encPool    *sync.Pool
	level      zapcore.LevelEnabler
	timeout    time.Duration
	alertType  string
	alertToken string
}

func (o *alertOption) fillDefault() *alertOption {
	o.encPool = &sync.Pool{
		New: func() any {
			return zapcore.NewJSONEncoder(zapcore.EncoderConfig{})
		},
	}
	o.level = defaultAlertHookLevel
	o.timeout = defaultAlertPusherTimeout
	return o
}

func (o *alertOption) applyOpts(opts ...AlertOption) (*alertOption, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// AlertOption option for create AlertHook
type AlertOption func(*alertOption) error

// WithAlertHookLevel level to trigger AlertHook
func WithAlertHookLevel(level zapcore.Level) AlertOption {
	return func(ao *alertOption) error {
		if level.Enabled(zap.DebugLevel) {
			// because Alert will use `debug` logger,
			// hook with debug will cause infinite recursive
			return errors.Errorf("level should higher than debug")
		}
		if level.Enabled(zap.WarnLevel) {
			Shared.Warn("level is better higher than warn")
		}

		ao.level = level
		return nil
	}
}

// WithAlertPushTimeout set Alert HTTP timeout
func WithAlertPushTimeout(timeout time.Duration) AlertOption {
	return func(a *alertOption) error {
		a.timeout = timeout
		return nil
	}
}

// WithAlertType set type for alert hooker
func WithAlertType(alertType string) AlertOption {
	return func(ao *alertOption) error {
		alertType = strings.TrimSpace(alertType)
		if alertType == "" {
			return errors.Errorf("alertType should not be empty")
		}

		ao.alertType = alertType
		return nil
	}
}

// WithAlertToken set token for alert hooker
func WithAlertToken(token string) AlertOption {
	return func(ao *alertOption) error {
		token = strings.TrimSpace(token)
		if token == "" {
			return errors.Errorf("token should not be empty")
		}

		ao.alertToken = token
		return nil
	}
}

type alertMsg struct {
	alertType,
	pushToken,
	msg string
}

// NewAlert create new Alert
func NewAlert(ctx context.Context,
	pushAPI string,
	opts ...AlertOption,
) (a *Alert, err error) {
	Shared.Debug("create new Alert")
	if pushAPI == "" {
		return nil, errors.Errorf("pushAPI should nout empty")
	}

	opt, err := new(alertOption).fillDefault().applyOpts(opts...)
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

// Close close Alert
func (a *Alert) Close() {
	close(a.stopChan) // should close stopChan first
	close(a.senderChan)
}

// SendWithType send alert with specific type, token and msg
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
func (a *Alert) Send(msg string) (err error) {
	return a.SendWithType(a.alertType, a.token, msg)
}

// GetZapHook get hook for zap logger
func (a *Alert) GetZapHook() func(zapcore.Entry, []zapcore.Field) (err error) {
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
