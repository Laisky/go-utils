package utils

import (
	"context"
	"fmt"
	"github.com/Laisky/zap"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Laisky/graphql"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

const (
	defaultLaiskyRemoteLockTokenUserKey    = "uid"
	defaultLaiskyRemoteLockAuthCookieName  = "general"
	defaultLaiskyRemoteLockTimeout         = 5 * time.Second
	defaultLaiskyRemoteLockRenewalDuration = 10 * time.Second
	defaultLaiskyRemoteLockRenewalInterval = 1 * time.Second
	defaultLaiskyRemoteLockIsRenewal       = false
	defaultLaiskyRemoteLockMaxRetry        = 3
)

// Mutex mutex that support unblocking lock
type Mutex struct {
	l uint32
}

// NewMutex create new mutex
func NewMutex() *Mutex {
	return &Mutex{
		l: 0,
	}
}

// TryLock return true if succeed locked
func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.l, 0, 1)
}

// IsLocked return true if is locked
func (m *Mutex) IsLocked() bool {
	return atomic.LoadUint32(&m.l) == 1
}

// TryRelease return true if succeed release
func (m *Mutex) TryRelease() bool {
	return atomic.CompareAndSwapUint32(&m.l, 1, 0)
}

// ForceRelease force release lock
func (m *Mutex) ForceRelease() {
	atomic.StoreUint32(&m.l, 0)
}

// SpinLock block until succee acquired lock
func (m *Mutex) SpinLock(step, timeout time.Duration) {
	start := Clock.GetUTCNow()
	for {
		if m.TryLock() || Clock.GetUTCNow().Sub(start) > timeout {
			return
		}
		time.Sleep(step)
	}
}

// LaiskyRemoteLock acquire lock from Laisky's GraphQL API
type LaiskyRemoteLock struct {
	cli *graphql.Client
	token,
	tokenCookieName,
	userID string

	timeout time.Duration
}

// LaiskyRemoteLockOptFunc laisky's lock option
type LaiskyRemoteLockOptFunc func(*LaiskyRemoteLock)

// WithLaiskyRemoteLockTimeout set http client timeout
func WithLaiskyRemoteLockTimeout(timeout time.Duration) LaiskyRemoteLockOptFunc {
	return func(opt *LaiskyRemoteLock) {
		opt.timeout = timeout
	}
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

// NewLaiskyRemoteLock create remote lock
func NewLaiskyRemoteLock(api, token string, opts ...LaiskyRemoteLockOptFunc) (l *LaiskyRemoteLock, err error) {
	var payload jwt.MapClaims
	if payload, err = ParseJWTTokenWithoutValidate(token); err != nil {
		return nil, errors.Wrap(err, "token invalidate")
	}

	l = &LaiskyRemoteLock{
		token:           token,
		tokenCookieName: defaultLaiskyRemoteLockAuthCookieName,
		timeout:         defaultLaiskyRemoteLockTimeout,
	}
	var ok bool
	if l.userID, ok = payload[defaultLaiskyRemoteLockTokenUserKey].(string); !ok {
		return nil, fmt.Errorf("unknown typo of %v, should be string", defaultLaiskyRemoteLockTokenUserKey)
	}
	for _, optf := range opts {
		optf(l)
	}

	l.cli = graphql.NewClient(
		api,
		&http.Client{
			Timeout: l.timeout,
		},
		graphql.WithCookie(l.tokenCookieName, l.token),
	)

	return l, nil
}

type acquireLockMutation struct {
	AcquireLock bool `graphql:"AcquireLock(lock_name: $lock_name, is_renewal: $is_renewal, duration_sec: $duration_sec)"`
}

type acquireLockOption struct {
	renewalInterval,
	duration time.Duration
	isRenewal bool
	maxRetry  int
}

// AcquireLockOptFunc options for acquire lock
type AcquireLockOptFunc func(*acquireLockOption)

// WithAcquireLockDuration set how long to extend lock
func WithAcquireLockDuration(duration time.Duration) AcquireLockOptFunc {
	if duration <= 0 {
		Logger.Panic("duration should greater than 0", zap.Duration("duration", duration))
	}
	return func(opt *acquireLockOption) {
		opt.duration = duration
	}
}

// WithAcquireLockRenewalInterval set how ofter to renewal lock
func WithAcquireLockRenewalInterval(renewalInterval time.Duration) AcquireLockOptFunc {
	if renewalInterval < 100*time.Millisecond {
		Logger.Panic("renewalInterval must greater than 100ms", zap.Duration("renewalInterval", renewalInterval))
	}
	return func(opt *acquireLockOption) {
		opt.renewalInterval = renewalInterval
	}
}

// WithAcquireLockIsRenewal set whether to auto renewal lock
func WithAcquireLockIsRenewal(isRenewal bool) AcquireLockOptFunc {
	return func(opt *acquireLockOption) {
		opt.isRenewal = isRenewal
	}
}

// WithAcquireLockMaxRetry set max retry to acquire lock
func WithAcquireLockMaxRetry(maxRetry int) AcquireLockOptFunc {
	if maxRetry < 0 {
		Logger.Panic("maxRetry must greater than 0", zap.Int("maxRetry", maxRetry))
	}
	return func(opt *acquireLockOption) {
		opt.maxRetry = maxRetry
	}
}

// AcquireLock acquire lock with lockname,
// if `isRenewal=true`, will automate refresh lock's lease until ctx done.
// duration to specify how much time each renewal will extend.
func (l *LaiskyRemoteLock) AcquireLock(ctx context.Context, lockName string, opts ...AcquireLockOptFunc) (ok bool, err error) {
	opt := &acquireLockOption{
		renewalInterval: defaultLaiskyRemoteLockRenewalInterval,
		duration:        defaultLaiskyRemoteLockRenewalDuration,
		isRenewal:       defaultLaiskyRemoteLockIsRenewal,
		maxRetry:        defaultLaiskyRemoteLockMaxRetry,
	}
	for _, optf := range opts {
		optf(opt)
	}

	var (
		query = new(acquireLockMutation)
		vars  = map[string]interface{}{
			"lock_name":    graphql.String(lockName),
			"is_renewal":   graphql.Boolean(opt.isRenewal),
			"duration_sec": graphql.Int(opt.duration.Seconds()),
		}
	)
	if err = l.cli.Mutate(ctx, query, vars); err != nil {
		return ok, errors.Wrap(err, "request graphql mutation")
	}
	if ok = query.AcquireLock; opt.isRenewal && ok {
		go l.renewalLock(ctx, query, vars, opt)
	}
	return ok, nil
}

func (l *LaiskyRemoteLock) renewalLock(ctx context.Context, query *acquireLockMutation, vars map[string]interface{}, opt *acquireLockOption) {
	var (
		nRetry   = 0
		err      error
		ticker   = time.NewTicker(opt.renewalInterval)
		lockName = string(vars["lock_name"].(graphql.String))
	)
	defer ticker.Stop()
	Logger.Debug("start to auto renewal lock", zap.String("lock_name", lockName))
	for nRetry < opt.maxRetry {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err = l.cli.Mutate(ctx, query, vars); err != nil {
			Logger.Error("renewal lock", zap.Error(err), zap.Int("n_retry", nRetry), zap.String("lock_name", lockName))
			time.Sleep(1 * time.Second)
			nRetry++
		}
		nRetry = 0
		Logger.Debug("success renewal lock", zap.String("lock_name", lockName))
	}
}
