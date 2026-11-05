package utils

import (
	"container/heap"
	"reflect"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
)

const keyedTokenUnit int64 = 1_000_000_000
const keyedMaxParameter int64 = 1_000_000_000

// KeyedRateLimiterConfig sets independent shared and per-key token buckets.
// Rates are whole tokens per second. Each rate and burst must be between 1 and
// 1,000,000,000 inclusive. MaxKeys is the maximum number of retained keys and
// must be positive. Both budgets start full.
type KeyedRateLimiterConfig struct {
	TotalPerSecond, TotalBurst int64
	KeyPerSecond, KeyBurst     int64
	MaxKeys                    int
}

// KeyedRateLimiter atomically spends one shared token and one token for a key.
// It retains at most MaxKeys entries. An unseen key is refused at capacity
// unless an existing key's bucket has completely refilled; only a full bucket
// may be evicted. Idle entries remain until space is needed. The key count does
// not bound key byte size; validate untrusted keys and authenticate upstream.
//
// It is safe for concurrent use, has no background goroutine or Close method,
// and supports neither reservations nor refunds. It is process-local and must
// not be copied after first use. Its bounded integer rates and nanosecond clock
// give exact fractional refill without floating-point token rounding.
type KeyedRateLimiter[K comparable] struct {
	mu              sync.Mutex
	cfg             KeyedRateLimiterConfig
	total           keyedBucket
	entries         map[K]*keyedEntry[K]
	expiry          keyedExpiryHeap[K]
	checkDynamicKey bool
}

type keyedBucket struct {
	credit      int64
	last        time.Time
	initialized bool
}
type keyedEntry[K comparable] struct {
	key    K
	bucket keyedBucket
	fullAt time.Time
	index  int
}
type keyedExpiryHeap[K comparable] []*keyedEntry[K]

// NewKeyedRateLimiter validates cfg and returns a new independent limiter.
func NewKeyedRateLimiter[K comparable](cfg KeyedRateLimiterConfig) (*KeyedRateLimiter[K], error) {
	if cfg.TotalPerSecond < 1 || cfg.TotalPerSecond > keyedMaxParameter ||
		cfg.TotalBurst < 1 || cfg.TotalBurst > keyedMaxParameter ||
		cfg.KeyPerSecond < 1 || cfg.KeyPerSecond > keyedMaxParameter ||
		cfg.KeyBurst < 1 || cfg.KeyBurst > keyedMaxParameter || cfg.MaxKeys < 1 {
		return nil, errors.Errorf("rates and bursts must be between 1 and %d, and MaxKeys must be positive",
			keyedMaxParameter)
	}
	return &KeyedRateLimiter[K]{
		cfg: cfg, entries: make(map[K]*keyedEntry[K]),
		checkDynamicKey: typeContainsInterface(reflect.TypeFor[K]()),
	}, nil
}

// Allow atomically admits key when both budgets have a token and key capacity
// is available. It returns false without spending either token otherwise. A key
// whose dynamic value cannot be used safely as a map key, including NaN, is
// refused. Allow uses the process monotonic clock when available.
func (l *KeyedRateLimiter[K]) Allow(key K) bool {
	if !validKey(key, l.checkDynamicKey) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowLocked(key, time.Now())
}

// validKey reports whether key can be compared with itself and used in a map.
func validKey[K comparable](key K, checkDynamic bool) bool {
	// A comparable interface can contain a non-comparable value. NaNs are also
	// rejected because their non-reflexive equality makes map deletion impossible.
	if checkDynamic {
		value := reflect.ValueOf(key)
		if value.IsValid() && !value.Comparable() {
			return false
		}
	}
	return key == key
}

// typeContainsInterface reports whether t or a nested field can hold a dynamic value.
func typeContainsInterface(t reflect.Type) bool {
	switch t.Kind() { //nolint:exhaustive // Only these container kinds can contain interfaces.
	case reflect.Interface:
		return true
	case reflect.Array:
		return typeContainsInterface(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if typeContainsInterface(t.Field(i).Type) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// allowLocked applies admission at now while the caller holds l.mu.
func (l *KeyedRateLimiter[K]) allowLocked(key K, now time.Time) bool {
	entry := l.entries[key]
	if entry != nil {
		entry.bucket.refill(now, l.cfg.KeyPerSecond, l.cfg.KeyBurst)
		if entry.bucket.credit < keyedTokenUnit {
			return false
		}
	}
	l.total.refill(now, l.cfg.TotalPerSecond, l.cfg.TotalBurst)
	if l.total.credit < keyedTokenUnit {
		return false
	}
	if entry == nil {
		if len(l.entries) >= l.cfg.MaxKeys {
			oldest := l.expiry[0]
			if now.Before(oldest.fullAt) {
				return false
			}
			heap.Pop(&l.expiry)
			delete(l.entries, oldest.key)
		}
		entry = &keyedEntry[K]{
			key:    key,
			bucket: keyedBucket{credit: l.cfg.KeyBurst * keyedTokenUnit, last: now, initialized: true},
		}
		l.entries[key] = entry
		heap.Push(&l.expiry, entry)
	}
	l.total.credit -= keyedTokenUnit
	entry.bucket.credit -= keyedTokenUnit
	missing := l.cfg.KeyBurst*keyedTokenUnit - entry.bucket.credit
	wait := missing / l.cfg.KeyPerSecond
	if missing%l.cfg.KeyPerSecond != 0 {
		wait++
	}
	entry.fullAt = entry.bucket.last.Add(time.Duration(wait))
	heap.Fix(&l.expiry, entry.index)
	return true
}

// refill advances b to now, saturating at burst and avoiding overflow.
func (b *keyedBucket) refill(now time.Time, rate, burst int64) {
	capacity := burst * keyedTokenUnit
	if !b.initialized {
		b.initialized = true
		b.credit = capacity
		b.last = now
		return
	}
	if !now.After(b.last) {
		return
	}
	elapsed := now.Sub(b.last)
	missing := capacity - b.credit
	// Compare before multiplying: when this is false elapsed*rate < missing,
	// and missing never exceeds 1e18, so multiplication cannot overflow.
	if int64(elapsed) >= missing/rate && (missing%rate == 0 || int64(elapsed) > missing/rate) {
		b.credit = capacity
	} else {
		b.credit += int64(elapsed) * rate
	}
	b.last = now
}

// Len returns the number of entries in h.
func (h keyedExpiryHeap[K]) Len() int { return len(h) }

// Less reports whether i expires before j.
func (h keyedExpiryHeap[K]) Less(i, j int) bool { return h[i].fullAt.Before(h[j].fullAt) }

// Swap exchanges i and j while preserving their indexes.
func (h keyedExpiryHeap[K]) Swap(i, j int) { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }

// Push appends a keyed entry x to the heap.
func (h *keyedExpiryHeap[K]) Push(x any) {
	entry := x.(*keyedEntry[K])
	entry.index = len(*h)
	*h = append(*h, entry)
}

// Pop removes and returns the last keyed entry from the heap.
func (h *keyedExpiryHeap[K]) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	*h = old[:n-1]
	return entry
}
