package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"golang.org/x/sync/errgroup"

	"github.com/Laisky/go-utils/v4/log"
)

// RaceErr return when any goroutine returned
func RaceErr(gs ...func() error) (err error) {
	var once sync.Once
	cond := sync.NewCond(&sync.Mutex{})
	for _, g := range gs {
		g := g
		go func() {
			ierr := g()
			cond.L.Lock()
			once.Do(func() { err = ierr })
			cond.Signal()
			cond.L.Unlock()
		}()
	}

	cond.L.Lock()
	cond.Wait()
	cond.L.Unlock()

	return err
}

// RaceErrWithCtx return when any goroutine returned or ctx canceled
func RaceErrWithCtx(ctx context.Context, gs ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	resultCh := make(chan error, 1)
	for _, g := range gs {
		g := g
		go func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := g(ctx)
			select {
			case resultCh <- err:
			default:
			}

			cancel()
		}()
	}

	return <-resultCh
}

// RunWithTimeout run func with timeout
func RunWithTimeout(timeout time.Duration, f func() error) error {
	return RaceErr(f, func() error {
		time.Sleep(timeout)
		return nil
	})
}

// WaitComplete wait all goroutines complete or ctx canceled,
// returns the first non-nil error (if any) from them,
// or return ctx.Err() if ctx canceled.
func WaitComplete(ctx context.Context, goros ...func(ctx context.Context) error) (err error) {
	var pool errgroup.Group
	for _, g := range goros {
		g := g
		pool.Go(func() error {
			return g(ctx)
		})
	}

	alldone := make(chan struct{})
	go func() {
		defer close(alldone)
		err = pool.Wait()
	}()

	select {
	case <-alldone:
		return err
	case <-ctx.Done():
		// if alldone and ctx finished at the same time,
		// the select will random choose one.
		// double check if alldone is finished.
		select {
		case <-alldone:
			return err
		default:
			return ctx.Err()
		}
	}
}

// const (
// 	defaultLaiskyRemoteLockTokenUserKey    = "uid"
// 	defaultLaiskyRemoteLockAuthCookieName  = "general"
// 	defaultLaiskyRemoteLockTimeout         = 5 * time.Second
// 	defaultLaiskyRemoteLockRenewalDuration = 10 * time.Second
// 	defaultLaiskyRemoteLockRenewalInterval = 1 * time.Second
// 	defaultLaiskyRemoteLockIsRenewal       = false
// 	defaultLaiskyRemoteLockMaxRetry        = 3
// )

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

// // LaiskyRemoteLock acquire lock from Laisky's GraphQL API
// type LaiskyRemoteLock struct {
// 	cli *graphql.Client
// 	token,
// 	tokenCookieName,
// 	userID string

// 	timeout time.Duration
// }

// // LaiskyRemoteLockOptFunc laisky's lock option
// type LaiskyRemoteLockOptFunc func(*LaiskyRemoteLock) error

// // WithLaiskyRemoteLockTimeout set http client timeout
// func WithLaiskyRemoteLockTimeout(timeout time.Duration) LaiskyRemoteLockOptFunc {
// 	return func(opt *LaiskyRemoteLock) error {
// 		opt.timeout = timeout
// 		return nil
// 	}
// }

// type headerTransport struct {
// 	base    http.RoundTripper
// 	headers map[string]string
// }

// // NewLaiskyRemoteLock create remote lock
// func NewLaiskyRemoteLock(api, token string, opts ...LaiskyRemoteLockOptFunc) (l *LaiskyRemoteLock, err error) {
// 	var payload jwt.MapClaims
// 	if payload, err = ParseJWTTokenWithoutValidate(token); err != nil {
// 		return nil, errors.Wrap(err, "token invalidate")
// 	}

// 	l = &LaiskyRemoteLock{
// 		token:           token,
// 		tokenCookieName: defaultLaiskyRemoteLockAuthCookieName,
// 		timeout:         defaultLaiskyRemoteLockTimeout,
// 	}
// 	var ok bool
// 	if l.userID, ok = payload[defaultLaiskyRemoteLockTokenUserKey].(string); !ok {
// 		return nil, errors.Errorf("unknown typo of %s, should be string", defaultLaiskyRemoteLockTokenUserKey)
// 	}
// 	for _, optf := range opts {
// 		if err = optf(l); err != nil {
// 			return nil, errors.Wrap(err, "set option")
// 		}
// 	}

// 	l.cli = graphql.NewClient(
// 		api,
// 		&http.Client{
// 			Timeout: l.timeout,
// 		},
// 		graphql.WithCookie(l.tokenCookieName, l.token),
// 	)

// 	return l, nil
// }

// type acquireLockMutation struct {
// 	AcquireLock bool `graphql:"AcquireLock(
// lock_name: $lock_name,
//  is_renewal: $is_renewal,
//  duration_sec: $duration_sec)"`
// }

// type acquireLockOption struct {
// 	renewalInterval,
// 	duration time.Duration
// 	isRenewal bool
// 	maxRetry  int
// }

// // AcquireLockOptFunc options for acquire lock
// type AcquireLockOptFunc func(*acquireLockOption) error

// // WithAcquireLockDuration set how long to extend lock
// func WithAcquireLockDuration(duration time.Duration) AcquireLockOptFunc {
// 	return func(opt *acquireLockOption) error {
// 		if duration <= 0 {
// 			return errors.Errorf("duration should greater than 0, got %d", duration)
// 		}

// 		opt.duration = duration
// 		return nil
// 	}
// }

// // WithAcquireLockRenewalInterval set how ofter to renewal lock
// func WithAcquireLockRenewalInterval(renewalInterval time.Duration) AcquireLockOptFunc {
// 	return func(opt *acquireLockOption) error {
// 		if renewalInterval < 100*time.Millisecond {
// 			return errors.Errorf("renewalInterval must greater than 100ms, got %d", renewalInterval)
// 		}

// 		opt.renewalInterval = renewalInterval
// 		return nil
// 	}
// }

// // WithAcquireLockIsRenewal set whether to auto renewal lock
// func WithAcquireLockIsRenewal(isRenewal bool) AcquireLockOptFunc {
// 	return func(opt *acquireLockOption) error {
// 		opt.isRenewal = isRenewal
// 		return nil
// 	}
// }

// // WithAcquireLockMaxRetry set max retry to acquire lock
// func WithAcquireLockMaxRetry(maxRetry int) AcquireLockOptFunc {
// 	return func(opt *acquireLockOption) error {
// 		if maxRetry < 0 {
// 			return errors.Errorf("maxRetry must greater than 0, got %d", maxRetry)
// 		}

// 		opt.maxRetry = maxRetry
// 		return nil
// 	}
// }

// // AcquireLock acquire lock with lockname,
// // if `isRenewal=true`, will automate refresh lock's lease until ctx done.
// // duration to specify how much time each renewal will extend.
// func (l *LaiskyRemoteLock) AcquireLock(ctx context.Context,
// lockName string,
// opts ...AcquireLockOptFunc) (ok bool, err error) {
// 	opt := &acquireLockOption{
// 		renewalInterval: defaultLaiskyRemoteLockRenewalInterval,
// 		duration:        defaultLaiskyRemoteLockRenewalDuration,
// 		isRenewal:       defaultLaiskyRemoteLockIsRenewal,
// 		maxRetry:        defaultLaiskyRemoteLockMaxRetry,
// 	}
// 	for _, optf := range opts {
// 		if err = optf(opt); err != nil {
// 			return ok, errors.Wrap(err, "set option")
// 		}
// 	}

// 	var (
// 		query = new(acquireLockMutation)
// 		vars  = map[string]any{
// 			"lock_name":    graphql.String(lockName),
// 			"is_renewal":   graphql.Boolean(opt.isRenewal),
// 			"duration_sec": graphql.Int(opt.duration.Seconds()),
// 		}
// 	)
// 	if err = l.cli.Mutate(ctx, query, vars); err != nil {
// 		return ok, errors.Wrap(err, "request graphql mutation")
// 	}
// 	if ok = query.AcquireLock; opt.isRenewal && ok {
// 		go l.renewalLock(ctx, query, vars, opt)
// 	}
// 	return ok, nil
// }

// func (l *LaiskyRemoteLock) renewalLock(ctx context.Context,
// query *acquireLockMutation,
//  vars map[string]any, opt *acquireLockOption) {
// 	var (
// 		nRetry   = 0
// 		err      error
// 		ticker   = time.NewTicker(opt.renewalInterval)
// 		lockName = string(vars["lock_name"].(graphql.String))
// 	)
// 	defer ticker.Stop()
// 	Logger.Debug("start to auto renewal lock", zap.String("lock_name", lockName))
// 	for nRetry < opt.maxRetry {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case <-ticker.C:
// 		}

// 		if err = l.cli.Mutate(ctx, query, vars); err != nil {
// 			Logger.Error("renewal lock",
// zap.Error(err),
// zap.Int("n_retry", nRetry), zap.String("lock_name", lockName))
// 			time.Sleep(1 * time.Second)
// 			nRetry++
// 			continue
// 		}
// 		nRetry = 0
// 		Logger.Debug("success renewal lock", zap.String("lock_name", lockName))
// 	}
// }

// ExpiredRLock Lock with expire time
type ExpiredRLock struct {
	m *LRUExpiredMap[*sync.RWMutex]
}

// NewExpiredRLock new ExpiredRLock
func NewExpiredRLock(ctx context.Context, exp time.Duration) (el *ExpiredRLock, err error) {
	el = &ExpiredRLock{}
	el.m, err = NewLRUExpiredMap(ctx, exp, func() *sync.RWMutex {
		return &sync.RWMutex{}
	})
	err = errors.Wrap(err, "new expired rlock")
	return
}

// GetLock get lock
func (e *ExpiredRLock) GetLock(key string) *sync.RWMutex {
	return e.m.Get(key)
}

// RWManagerInterface auto create rwlock if not exists
type RWManagerInterface interface {
	RLock(name string)
	Lock(name string)
	RUnlock(name string)
	Unlock(name string)
}

// RWManager auto create rwlock if not exists
type RWManager struct {
	m sync.Map
}

// RLock rlock lock by name
func (m *RWManager) RLock(name string) {
	mu, _ := m.m.LoadOrStore(name, &sync.RWMutex{})
	mu.(*sync.RWMutex).RLock() //nolint:forcetypeassert
}

// Lock lock by name
func (m *RWManager) Lock(name string) {
	mu, _ := m.m.LoadOrStore(name, &sync.RWMutex{})
	mu.(*sync.RWMutex).Lock() //nolint:forcetypeassert
}

// RUnlock runlock by name
func (m *RWManager) RUnlock(name string) {
	mu, ok := m.m.Load(name)
	if !ok {
		log.Shared.Panic("lock not exists", zap.String("name", name))
	}

	mu.(*sync.RWMutex).RUnlock() //nolint:forcetypeassert
}

// Unlock unlock by name
func (m *RWManager) Unlock(name string) {
	mu, ok := m.m.Load(name)
	if !ok {
		log.Shared.Panic("lock not exists", zap.String("name", name))
	}

	mu.(*sync.RWMutex).Unlock() //nolint:forcetypeassert
}
