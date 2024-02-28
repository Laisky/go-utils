package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/go-utils/v4/algorithm"
	"github.com/Laisky/go-utils/v4/log"
)

// TtlCache cache with ttl
type TtlCache[T any] struct {
	ctx    context.Context
	cancel func()
	sk     algorithm.SkipList[int64]
	kv     sync.Map
}

// NewTtlCache new cache with ttl
func NewTtlCache[T any]() *TtlCache[T] {
	c := &TtlCache[T]{
		sk: algorithm.NewSkiplist[int64](),
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	go c.clean()
	return c
}

// Close close cache
func (c *TtlCache[T]) Close() {
	c.cancel()
}

func (c *TtlCache[T]) clean() {
	now := time.Now()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if c.sk.Len() == 0 {
			time.Sleep(time.Second)
			now = time.Now()
			continue
		}

		ele := c.sk.Front()

		if ele.Key() > now.UnixNano() {
			time.Sleep(min(time.Duration(ele.Key()-now.UnixNano()), time.Second))
			now = time.Now()
			continue
		}

		c.sk.Remove(ele.Key())
	}
}

// Set set data with ttl
func (c *TtlCache[T]) Set(key string, val T, ttl time.Duration) {
	select {
	case <-c.ctx.Done():
		log.Shared.Panic("this cache already closed")
	default:
	}

	exp := time.Now().Add(ttl)
	c.sk.Set(exp.UnixNano(), key)
	c.kv.Store(key, expCacheItem{exp, val})
}

// Get get data
func (c *TtlCache[T]) Get(key string) (val T, ok bool) {
	select {
	case <-c.ctx.Done():
		log.Shared.Panic("this cache already closed")
	default:
	}

	v, ok := c.kv.Load(key)
	if !ok {
		return
	}

	exp := v.(expCacheItem).exp //nolint:forcetypeassert
	if exp.Before(time.Now()) {
		c.kv.Delete(key)
		c.sk.Remove(exp.UnixNano())
		return
	}

	return v.(expCacheItem).data.(T), true //nolint:forcetypeassert
}

// Delete remove key
func (c *TtlCache[T]) Delete(key string) {
	select {
	case <-c.ctx.Done():
		log.Shared.Panic("this cache already closed")
	default:
	}

	vi, ok := c.kv.LoadAndDelete(key)

	if ok {
		c.sk.Remove(vi.(expCacheItem).exp.UnixNano()) //nolint:forcetypeassert
	}
}

// SingleItemExpCache single item with expires
type SingleItemExpCache[T any] struct {
	expiredAt time.Time
	ttl       time.Duration
	data      T
	mu        sync.RWMutex
}

// NewSingleItemExpCache new expcache contains single data
func NewSingleItemExpCache[T any](ttl time.Duration) *SingleItemExpCache[T] {
	return &SingleItemExpCache[T]{
		ttl: ttl,
	}
}

// Set set data and refresh expires
func (c *SingleItemExpCache[T]) Set(data T) {
	c.mu.Lock()
	c.data = data
	c.expiredAt = Clock.GetUTCNow().Add(c.ttl)
	c.mu.Unlock()
}

// Get get data
//
// if data is expired, ok=false
func (c *SingleItemExpCache[T]) Get() (data T, ok bool) {
	c.mu.RLock()
	data = c.data

	ok = Clock.GetUTCNow().Before(c.expiredAt)
	c.mu.RUnlock()

	return
}

// ExpCache cache with expires
//
// can Store/Load like map
type ExpCache[T any] struct {
	data sync.Map
	ttl  time.Duration
}

type expCacheItem struct {
	exp  time.Time
	data any
}

// ExpCacheInterface cache with expire duration
type ExpCacheInterface[T any] interface {
	// Store store new key and val into cache
	Store(key string, val T)
	// Delete remove key
	Delete(key string)
	// LoadAndDelete load and delete val from cache
	LoadAndDelete(key string) (data T, ok bool)
	// Load load val from cache
	Load(key string) (data T, ok bool)
}

// NewExpCache new cache manager
//
// use with generic:
//
//	cc := NewExpCache[string](context.Background(), 100*time.Millisecond)
//	cc.Store("key", "val")
//	val, ok := cc.Load("key")
func NewExpCache[T any](ctx context.Context, ttl time.Duration) *ExpCache[T] {
	c := &ExpCache[T]{
		ttl: ttl,
	}
	go c.runClean(ctx)
	return c
}

func (c *ExpCache[T]) runClean(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		now := time.Now()
		c.data.Range(func(k, v any) bool {
			if v.(*expCacheItem).exp.Before(now) { //nolint:forcetypeassert
				// delete expired
				//
				// if new expCacheItem stored just before delete,
				// may delete item that not expired.
				// but this condition is rare, so may just add a little cost.
				c.data.Delete(k)
			}

			return true
		})

		time.Sleep(c.ttl)
	}
}

// Store store new key and val into cache
func (c *ExpCache[T]) Store(key string, val T) {
	c.data.Store(key, &expCacheItem{
		data: val,
		exp:  Clock.GetUTCNow().Add(c.ttl),
	})
}

// Delete remove key
func (c *ExpCache[T]) Delete(key string) {
	c.data.Delete(key)
}

// LoadAndDelete load and delete val from cache
func (c *ExpCache[T]) LoadAndDelete(key string) (data T, ok bool) {
	//nolint:forcetypeassert
	if datai, ok := c.data.LoadAndDelete(key); ok && Clock.GetUTCNow().Before(datai.(*expCacheItem).exp) {
		return datai.(*expCacheItem).data.(T), ok //nolint:forcetypeassert
	}

	return data, false
}

// Load load val from cache
func (c *ExpCache[T]) Load(key string) (data T, ok bool) {
	//nolint:forcetypeassert
	if datai, ok := c.data.Load(key); ok && Clock.GetUTCNow().Before(datai.(*expCacheItem).exp) {
		return datai.(*expCacheItem).data.(T), ok //nolint:forcetypeassert
	} else if ok {
		// delete expired
		c.data.Delete(key)
	}

	return data, false
}

type expiredMapItem[T any] struct {
	sync.RWMutex
	data T
	t    *int64
}

func (e *expiredMapItem[T]) getTime() time.Time {
	return ParseUnix2UTC(atomic.LoadInt64(e.t))
}

func (e *expiredMapItem[T]) refreshTime() {
	atomic.StoreInt64(e.t, Clock.GetUTCNow().Unix())
}

// LRUExpiredMap map with expire time, auto delete expired item.
//
// `Get` will auto refresh item's expires.
// `Get` will auto create new item if key not exists.
type LRUExpiredMap[T any] struct {
	m   sync.Map
	ttl time.Duration
	new func() T
}

// NewLRUExpiredMap new ExpiredMap
func NewLRUExpiredMap[T any](ctx context.Context,
	ttl time.Duration,
	newIns func() T) (el *LRUExpiredMap[T], err error) {
	el = &LRUExpiredMap[T]{
		ttl: ttl,
		new: newIns,
	}

	go el.clean(ctx)
	return el, nil
}

func (e *LRUExpiredMap[T]) clean(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		now := time.Now()
		e.m.Range(func(k, v any) bool {
			//nolint:forcetypeassert
			if v.(*expiredMapItem[T]).getTime().Add(e.ttl).After(now) {
				return true
			}

			// lock is expired
			v.(*expiredMapItem[T]).Lock()         //nolint:forcetypeassert
			defer v.(*expiredMapItem[T]).Unlock() //nolint:forcetypeassert

			//nolint:forcetypeassert
			if v.(*expiredMapItem[T]).getTime().Add(e.ttl).Before(now) {
				// lock still expired
				e.m.Delete(k)
			}

			return true
		})

		time.Sleep(e.ttl / 2)
	}
}

// Get get item
//
// will auto refresh key's ttl
func (e *LRUExpiredMap[T]) Get(key string) T {
	l, _ := e.m.Load(key)
	if l == nil {
		t := Clock.GetUTCNow().Unix()
		l, _ = e.m.LoadOrStore(key, &expiredMapItem[T]{
			t:    &t,
			data: e.new(),
		})
	} else {
		ol := l.(*expiredMapItem[T]) //nolint:forcetypeassert
		ol.RLock()
		ol.refreshTime()
		l, _ = e.m.LoadOrStore(key, ol)
		ol.RUnlock()
	}

	//nolint:forcetypeassert
	return l.(*expiredMapItem[T]).data
}
