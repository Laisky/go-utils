package algorithm

import (
	"github.com/Laisky/errors/v2"
	"github.com/gammazero/deque"
)

// Deque is a generic double-ended queue that supports pushing, popping, and peeking at both
// ends; it is backed by gammazero/deque.
//
// https://pkg.go.dev/github.com/gammazero/deque#Deque
type Deque[T any] interface {
	PushBack(T)
	PushFront(T)
	PopFront() T
	PopBack() T
	Len() int
	Front() T
	Back() T
}

// dequeOpt holds the capacity settings that DequeOptFunc options configure for NewDeque.
type dequeOpt struct {
	currentCapacity,
	minimalCapacity int
}

// applyFuncs applies each option in optfs to o in order. It returns o after all options succeed,
// or a wrapped error from the first option that rejects its argument.
func (o *dequeOpt) applyFuncs(optfs ...DequeOptFunc) (*dequeOpt, error) {
	for _, optf := range optfs {
		if err := optf(o); err != nil {
			return nil, errors.Wrap(err, "apply deque option")
		}
	}

	return o, nil
}

// DequeOptFunc optional arguments for deque
type DequeOptFunc func(*dequeOpt) error

// WithDequeCurrentCapacity preallocate memory for deque
func WithDequeCurrentCapacity(size int) DequeOptFunc {
	return func(opt *dequeOpt) error {
		if size < 0 {
			return errors.Errorf("size must greater than 0")
		}

		opt.currentCapacity = size
		return nil
	}
}

// WithDequeMinimalCapacity set deque minimal capacity
func WithDequeMinimalCapacity(size int) DequeOptFunc {
	return func(opt *dequeOpt) error {
		if size < 0 {
			return errors.Errorf("size must greater than 0")
		}

		opt.minimalCapacity = size
		return nil
	}
}

// NewDeque new deque
func NewDeque[T any](optfs ...DequeOptFunc) (Deque[T], error) {
	opt, err := new(dequeOpt).applyFuncs(optfs...)
	if err != nil {
		return nil, errors.Wrap(err, "apply deque options")
	}

	q := new(deque.Deque[T])
	q.SetBaseCap(opt.minimalCapacity)
	return q, nil
}
