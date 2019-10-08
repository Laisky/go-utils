package journal

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"

	"github.com/RoaringBitmap/roaring"
)

type Int64SetItf interface {
	Add(int)
	AddInt64(int64)
	CheckAndRemove(int64) bool
	GetLen() int
}

// Uint32Set set depends on bitmap.
// only support uint32, so cannot support more than 4294967295 numbers.
type Uint32Set struct {
	sync.Mutex
	padding struct{}
	d       *roaring.Bitmap
}

// NewUint32Set create new Uint32Set
func NewUint32Set() *Uint32Set {
	s := &Uint32Set{
		padding: struct{}{},
		d:       roaring.NewBitmap(),
	}
	return s
}

// AddInt64 add new number
func (s *Uint32Set) AddInt64(i int64) {
	s.Lock()
	s.d.Add(uint32(i % math.MaxUint32))
	s.Unlock()
}

// CheckAndRemoveInt64 return true if exists
func (s *Uint32Set) CheckAndRemoveInt64(i int64) (ok bool) {
	s.Lock()
	ok = s.d.CheckedRemove(uint32(i % math.MaxUint32))
	s.Unlock()
	return ok
}

// AddUint32 add new number
func (s *Uint32Set) AddUint32(i uint32) {
	s.Lock()
	s.d.Add(i)
	s.Unlock()
}

// CheckAndRemoveUint32 return true if exists
func (s *Uint32Set) CheckAndRemoveUint32(i uint32) (ok bool) {
	s.Lock()
	ok = s.d.CheckedRemove(i)
	s.Unlock()
	return ok
}

// GetLen return length
func (s *Uint32Set) GetLen() int {
	return int(s.d.GetCardinality())
}

// Int64Set set depends on sync.Map.
// cost much more memory than bitmap
type Int64Set struct {
	padding struct{}
	n       int64
	d       *sync.Map
}

// NewInt64Set create new Int64Set
func NewInt64Set() *Int64Set {
	return &Int64Set{
		padding: struct{}{},
		d:       &sync.Map{},
	}
}

// Add add new number
func (s *Int64Set) Add(i int) {
	s.AddInt64(int64(i))
}

// AddInt64 add int64
func (s *Int64Set) AddInt64(i int64) {
	atomic.AddInt64(&s.n, 1)
	s.d.Store(i, s.padding)
}

// CheckAndRemove return true if exists
func (s *Int64Set) CheckAndRemove(i int64) (ok bool) {
	if _, ok = s.d.Load(i); ok {
		atomic.AddInt64(&s.n, -1)
	}
	s.d.Delete(i)
	return ok
}

// GetLen return length
func (s *Int64Set) GetLen() int {
	return int(atomic.LoadInt64(&s.n))
}

// Int64SetWithTTL int64 set with TTL
type Int64SetWithTTL struct {
	sync.RWMutex
	chgLock  *sync.Mutex
	stopChan chan struct{}

	ttl      time.Duration
	og, ng   *sync.Map
	ogN, ngN int64
}

const (
	defaultIDSetTTL = 1 * time.Minute
)

// NewInt64SetWithTTL create new int64 set with ttl
func NewInt64SetWithTTL(ctx context.Context, ttl time.Duration) *Int64SetWithTTL {
	if ttl < defaultIDSetTTL {
		utils.Logger.Warn("TTL too small")
	}

	s := &Int64SetWithTTL{
		stopChan: make(chan struct{}),
		chgLock:  &sync.Mutex{},
		ttl:      ttl,
		ng:       &sync.Map{},
	}
	utils.Logger.Debug("NewInt64SetWithTTL",
		zap.Duration("ttl", s.ttl),
	)
	go s.StartRotate(ctx)
	return s
}

// Add add int
func (s *Int64SetWithTTL) Add(id int) {
	s.AddInt64(int64(id))
}

// AddInt64 add int64
func (s *Int64SetWithTTL) AddInt64(id int64) {
	t := utils.Clock.GetUTCNow().Unix()
	s.RLock()
	if _, ok := s.ng.LoadOrStore(id, t); !ok {
		atomic.AddInt64(&s.ngN, 1)
	} else { // already exists
		s.chgLock.Lock()
		if _, ok = s.ng.LoadOrStore(id, t); !ok {
			atomic.AddInt64(&s.ngN, 1)
		} else {
			s.ng.Store(id, t)
		}
		s.chgLock.Unlock()
	}
	s.RUnlock()
}

// CheckAndRemove return true if id committed
func (s *Int64SetWithTTL) CheckAndRemove(id int64) (ok bool) {
	s.RLock()
	defer s.RUnlock()
	var (
		t  = utils.Clock.GetUTCNow().Unix()
		vi interface{}
	)
	if vi, ok = s.ng.Load(id); ok {
		return true
	}

	if s.og != nil {
		if vi, ok = s.og.Load(id); ok {
			if vi.(int64) > t {
				return true
			}

			s.og.Delete(id)
			atomic.AddInt64(&s.ogN, -1)
		}
	}

	return false
}

// GetLen get items number of set
func (s *Int64SetWithTTL) GetLen() (r int) {
	s.RLock()
	r = int(atomic.LoadInt64(&s.ogN) + atomic.LoadInt64(&s.ngN))
	s.RUnlock()
	return r
}

// Close close set, stop rotate
func (s *Int64SetWithTTL) Close() {
	s.stopChan <- struct{}{}
}

// StartRotate start counter rotate
func (s *Int64SetWithTTL) StartRotate(ctx context.Context) {
	defer utils.Logger.Info("StartRotate exit")
	for {
		select {
		case <-s.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		s.Lock()
		s.ogN, s.ngN = s.ngN, 0
		s.og = s.ng
		s.ng = &sync.Map{}
		s.Unlock()
		time.Sleep(s.ttl)
	}
}
