package log

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/graphql"
	zap "github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"

	"github.com/Laisky/go-utils/v6/internal/netdiag"
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
	// admission orders enqueueing with Close and worker termination.
	admission sync.Mutex
	// closed records explicit shutdown or worker termination.
	// The hook can reject before encoding; admission checks again under its lock.
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

// Close cancels delivery and prevents later queue admissions; it does not flush.
// It is idempotent and safe to call concurrently, including from a rate limiter.
// Done closes only after the worker releases its queued payload references.
func (a *Alert) Close() {
	a.closeOnce.Do(func() {
		a.admission.Lock()
		defer a.admission.Unlock()
		a.closed.Store(true)
		close(a.stopChan)
		a.cancel()
	})
}

// finishSender rejects further admissions and releases pending messages before Done.
// No caller callback or network operation is invoked while admission is locked.
func (a *Alert) finishSender() {
	a.admission.Lock()
	defer a.admission.Unlock()
	a.closed.Store(true)
	for {
		select {
		case _, ok := <-a.senderChan:
			if !ok {
				close(a.done)
				return
			}
		default:
			close(a.done)
			return
		}
	}
}

// SendWithType sends an alert with the specified type, token, and message.
func (a *Alert) SendWithType(alertType, pushToken, msg string) (err error) {
	if alertType == "" || pushToken == "" || msg == "" {
		return errors.Errorf("alertType, pushToken and msg should not be empty")
	}

	if len(msg) > a.maxMessageBytes {
		return errors.WithStack(ErrAlertMessageTooLarge)
	}
	// The lifecycle check and nonblocking admission share the shutdown lock.
	// A canceled context may race a send ordered before worker termination,
	// but neither Close nor Done can complete before this admission finishes.
	a.admission.Lock()
	defer a.admission.Unlock()
	if a.closed.Load() || a.ctx.Err() != nil {
		return errors.New("alert is closed")
	}
	select {
	case a.senderChan <- &alertMsg{alertType: alertType, pushToken: pushToken, msg: msg}:
	default:
		return errors.New("send channel overflow")
	}

	return nil
}

// runSender runs the alert sender goroutine.
func (a *Alert) runSender(ctx context.Context) {
	defer a.finishSender()
	var (
		ok      bool
		payload *alertMsg
		err     error
		query   = new(alertMutation)
		vars    = map[string]any{}
	)
	for {
		if ctx.Err() != nil || a.closed.Load() {
			return
		}
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

		if ctx.Err() != nil || a.closed.Load() {
			return
		}

		// check ratelimiter
		if a.ratelimiter != nil && !a.ratelimiter.Allow() {
			Shared.Debug("alert dropped by rate limit")
			continue
		}

		if ctx.Err() != nil || a.closed.Load() {
			return
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
