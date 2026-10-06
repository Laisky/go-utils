package log

import (
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"
)

const defaultPusherSendTimeout = 30 * time.Second

var (
	// ErrPusherQueueFull reports an entry dropped without waiting for queue space.
	ErrPusherQueueFull = errors.New("log pusher queue is full")
	// ErrPusherClosed reports an entry rejected after lifecycle cancellation.
	ErrPusherClosed = errors.New("log pusher is closed")
	// ErrPusherMessageTooLarge reports a formatted entry exceeding its byte budget.
	ErrPusherMessageTooLarge = errors.New("log pusher message exceeds byte limit")
)

// PusherStats is a race-safe snapshot. Dropped counts enqueue rejections, not
// entries abandoned at shutdown. Enqueued includes queued and in-flight entries.
type PusherStats struct{ Enqueued, Delivered, Failed, Dropped uint64 }

// pusherCounters owns the worker and hook's independent atomic counters.
type pusherCounters struct{ enqueued, delivered, failed, dropped atomic.Uint64 }

// Stats returns delivery counters without recursively writing to any logger.
func (p *Pusher) Stats() PusherStats {
	return PusherStats{p.counters.enqueued.Load(), p.counters.delivered.Load(), p.counters.failed.Load(), p.counters.dropped.Load()}
}

// Close cancels the worker and in-flight send. It is idempotent and does not flush.
func (p *Pusher) Close() { p.cancel() }

// Done is closed after the sender returns; custom senders must honor cancellation.
func (p *Pusher) Done() <-chan struct{} { return p.done }

// WithPusherSendTimeout bounds every send attempt, preserving earlier deadlines.
func WithPusherSendTimeout(timeout time.Duration) PusherOption {
	return func(o *pusherOption) error {
		if timeout <= 0 {
			return errors.New("send timeout must be positive")
		}
		o.sendTimeout = timeout
		return nil
	}
}

// WithPusherMaxMessageBytes sets a positive maximum size for each queued entry.
func WithPusherMaxMessageBytes(maxBytes int) PusherOption {
	return func(o *pusherOption) error {
		if maxBytes <= 0 {
			return errors.New("message byte limit must be positive")
		}
		o.maxMessageBytes = maxBytes
		return nil
	}
}
