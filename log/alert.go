package log

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/go-utils/v6/internal/netdiag"
	"github.com/Laisky/graphql"
	zap "github.com/Laisky/zap"
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
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}

	// closeOnce guards Close so it is idempotent (closing an already
	// closed channel panics).
	closeOnce sync.Once
	// closed is set once Close has been called. SendWithType checks it
	// before attempting a send to avoid panicking on a closed channel.
	closed atomic.Bool
}

// alertOption holds configuration options for the Alert hook.
type alertOption struct {
	encPool         *sync.Pool
	level           zapcore.LevelEnabler
	timeout         time.Duration
	alertType       string
	alertToken      string
	ratelimiter     RateLimiter
	allowedFields   map[string]struct{}
	maxMessageBytes int
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
	o.maxMessageBytes = defaultAlertMessageBytes

	// apply options
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, errors.Wrap(err, "apply alert option")
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
		if timeout <= 0 {
			return errors.New("alert timeout must be positive")
		}
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
		return nil, errors.Wrap(err, "apply alert options")
	}

	ctx, cancel := context.WithCancel(ctx)
	a = &Alert{
		alertOption: opt,
		stopChan:    make(chan struct{}),
		senderChan:  make(chan *alertMsg, defaultAlertPusherBufSize),
		pushAPI:     pushAPI,
		ctx:         ctx, cancel: cancel, done: make(chan struct{}),
	}

	a.cli = graphql.NewClient(a.pushAPI, &http.Client{
		Timeout: a.timeout,
	})

	go a.runSender(ctx)
	return a, nil
}

// Close closes the Alert hook.
//
// Close is idempotent and safe to call concurrently. It only closes
// stopChan (not senderChan) and cancels in-flight HTTP requests: closing senderChan while SendWithType may be
// racing a send would panic with "send on closed channel", and a double
// Close would panic with "close of closed channel". The sender goroutine
// returns on stopChan, and the unsent senderChan is reclaimed by the GC.
func (a *Alert) Close() {
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		close(a.stopChan)
		a.cancel()
	})
}

// SendWithType sends an alert with the specified type, token, and message.
func (a *Alert) SendWithType(alertType, pushToken, msg string) (err error) {
	if alertType == "" || pushToken == "" || msg == "" {
		return errors.Errorf("alertType, pushToken and msg should not be empty")
	}

	// Reject sends after Close to avoid panicking on a closed channel; a log
	// triggering the alert hook after shutdown must not crash the process.
	if a.closed.Load() || a.ctx.Err() != nil {
		return errors.Errorf("alert is closed")
	}

	if len(msg) > a.maxMessageBytes {
		return errors.WithStack(ErrAlertMessageTooLarge)
	}
	select {
	case <-a.ctx.Done():
		return errors.New("alert is closed")
	case a.senderChan <- &alertMsg{
		alertType: alertType,
		pushToken: pushToken,
		msg:       msg,
	}:
	case <-a.stopChan:
		// Close raced with this send; abort instead of risking a send on a
		// channel whose sole reader (runSender) has already returned.
		return errors.Errorf("alert is closed")
	default:
		return errors.Errorf("send channel overflow")
	}

	return nil
}

// runSender runs the alert sender goroutine.
func (a *Alert) runSender(ctx context.Context) {
	defer close(a.done)
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
		if a.ratelimiter != nil && !a.ratelimiter.Allow() {
			Shared.Debug("alert dropped by rate limit")
			continue
		}

		vars["type"] = graphql.String(payload.alertType)
		vars["token"] = graphql.String(payload.pushToken)
		vars["msg"] = graphql.String(payload.msg)

		ctxReq, cancel := context.WithTimeout(ctx, a.timeout)
		if err = a.cli.Mutate(ctxReq, query, vars); err != nil {
			Shared.Debug("alert delivery failed", zap.String("endpoint", netdiag.Endpoint(a.pushAPI)))
			cancel()
			continue
		}
		cancel()

		Shared.Debug("alert delivered")
	}
}

// Send sends an alert with the default alertType and pushToken.
func (a *Alert) Send(msg string) (err error) {
	return a.SendWithType(a.alertType, a.alertToken, msg)
}
