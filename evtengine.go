package utils

import (
	"context"
	"strconv"
	"sync"

	"github.com/Laisky/zap"
	"github.com/pkg/errors"
)

const (
	defaultEventEngineNFork         int = 2
	defaultEventEngineMsgBufferSize int = 1
)

// Event evt
type Event struct {
	Topic string
	Meta  map[string]interface{}
}

// EventHandler function to handle event
type EventHandler func(*Event)

type evtHandler struct {
	h    EventHandler
	name string
}

type evtHandlers struct {
	sync.RWMutex
	hs []evtHandler
}

func (e *evtHandlers) Append(handlers ...evtHandler) *evtHandlers {
	e.hs = append(e.hs, handlers...)

	return e
}

func (e *evtHandlers) Remove(name string) *evtHandlers {
	var hs []evtHandler
	for _, h := range e.hs {
		if h.name != name {
			hs = append(hs, h)
		}
	}

	e.hs = hs
	return e
}

func (e *evtHandlers) Len() int {
	return len(e.hs)
}

func (e *evtHandlers) Clone() (handlers []evtHandler) {
	return append(handlers, e.hs...)
}

// EventEngine type of event store
type EventEngine struct {
	*LoggerType
	q chan *Event

	// topic2hs map[topic][]evtHandler
	topic2hs *sync.Map
}

type eventStoreManagerOpt struct {
	msgBufferSize int
	nfork         int
	logger        *LoggerType
}

// EventEngineOptFunc options for EventEngine
type EventEngineOptFunc func(*eventStoreManagerOpt) error

// WithEventEngineNFork set nfork of event store
func WithEventEngineNFork(nfork int) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		if nfork <= 0 {
			return errors.Errorf("nfork must > 0")
		}

		opt.nfork = nfork
		return nil
	}
}

// WithEventEngineChanBuffer set msg buffer size of event store
func WithEventEngineChanBuffer(msgBufferSize int) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		if msgBufferSize < 0 {
			return errors.Errorf("msgBufferSize must >= 0")
		}

		opt.msgBufferSize = defaultEventEngineMsgBufferSize
		return nil
	}
}

// WithEventEngineLogger set event store's logger
func WithEventEngineLogger(logger *LoggerType) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		if logger == nil {
			return errors.Errorf("logger is nil")
		}

		opt.logger = logger
		return nil
	}
}

// NewEventEngine new event store manager
func NewEventEngine(ctx context.Context, opts ...EventEngineOptFunc) (e *EventEngine, err error) {
	opt := &eventStoreManagerOpt{
		msgBufferSize: defaultEventEngineMsgBufferSize,
		nfork:         defaultEventEngineNFork,
		logger:        Logger.Named("evt-store-" + RandomStringWithLength(6)),
	}
	for _, optf := range opts {
		if err = optf(opt); err != nil {
			return nil, err
		}
	}

	e = &EventEngine{
		LoggerType: opt.logger,
		q:          make(chan *Event, opt.msgBufferSize),
		topic2hs:   &sync.Map{},
	}

	e.run(ctx, opt.nfork)
	e.Logger.Debug("new event store",
		zap.Int("nfork", opt.nfork),
		zap.Int("buffer", opt.msgBufferSize))
	return e, nil
}

func runHandlerWithoutPanic(h evtHandler, evt *Event) (err error) {
	defer func() {
		if erri := recover(); erri != nil {
			err = errors.Errorf("run event handler `%s` with evt `%s`: %+v", h.name, evt.Topic, erri)
		}
	}()

	h.h(evt)
	return nil
}

// Run start EventEngine
func (e *EventEngine) run(ctx context.Context, nfork int) {
	for i := 0; i < nfork; i++ {
		logger := e.Logger.Named(strconv.Itoa(i))
		go func() {
			logger.Debug("start event store runner")

			var evt *Event
			for {
				select {
				case <-ctx.Done():
					return
				case evt = <-e.q:
				}

				hsi, ok := e.topic2hs.Load(evt.Topic)
				if !ok || hsi == nil {
					continue
				}

				hsi.(*evtHandlers).RLock()
				hs := hsi.(*evtHandlers).Clone()
				hsi.(*evtHandlers).RUnlock()

				for _, h := range hs {
					// if err := runHandlerWithoutPanic(h, evt); err != nil {
					// 	logger.Error("panic", zap.Error(err))
					// }

					h.h(evt)
				}
			}
		}()
	}
}

// Register register new handler to event store
func (e *EventEngine) Register(topic, handlerName string, handler EventHandler) {
	hs := &evtHandlers{
		hs: []evtHandler{{
			name: handlerName,
			h:    handler,
		}},
	}
	if actual, loaded := e.topic2hs.LoadOrStore(topic, hs); loaded {
		actual.(*evtHandlers).Lock()
		actual.(*evtHandlers).Append(hs.hs...)
		actual.(*evtHandlers).Unlock()
	}
}

// UnRegister delete handler in event store
func (e *EventEngine) UnRegister(topic, handlerName string) {
	if hsi, ok := e.topic2hs.Load(topic); ok {
		hsi.(*evtHandlers).Lock()
		hsi.(*evtHandlers).Remove(handlerName)
		if hsi.(*evtHandlers).Len() == 0 {
			e.topic2hs.Delete(topic)
		}

		hsi.(*evtHandlers).Unlock()
	}
}

// Publish publish new event
func (e *EventEngine) Publish(evt *Event) {
	e.q <- evt
}
