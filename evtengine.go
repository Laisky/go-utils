package utils

import (
	"context"
	"strconv"
	"sync"
	"time"

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
	Time  time.Time
	Meta  map[string]interface{}
}

// EventHandler function to handle event
type EventHandler func(*Event)

type evtHandlerItem struct {
	h    EventHandler
	name string
}

type evtHandlers struct {
	sync.RWMutex
	hs []evtHandlerItem
}

func (e *evtHandlers) Append(handlers ...evtHandlerItem) *evtHandlers {
	e.hs = append(e.hs, handlers...)

	return e
}

func (e *evtHandlers) Remove(name string) *evtHandlers {
	var hs []evtHandlerItem
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

func (e *evtHandlers) Clone() (handlers []evtHandlerItem) {
	return append(handlers, e.hs...)
}

// EventEngine type of event store
type EventEngine struct {
	*eventStoreManagerOpt
	q chan *Event

	// topic2hs map[topic][]evtHandlerItem
	topic2hs *sync.Map
}

type eventStoreManagerOpt struct {
	msgBufferSize int
	nfork         int
	logger        *LoggerType
	suppressPanic bool
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

// WithEventEngineSuppressPanic set whether supress event handler's panic
func WithEventEngineSuppressPanic(suppressPanic bool) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		opt.suppressPanic = suppressPanic
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
		eventStoreManagerOpt: opt,
		q:                    make(chan *Event, opt.msgBufferSize),
		topic2hs:             &sync.Map{},
	}

	taskChan := make(chan *eventRunChanItem, opt.msgBufferSize)
	e.startRunner(ctx, opt.nfork, taskChan)
	e.run(ctx, taskChan)
	e.logger.Info("new event store",
		zap.Int("nfork", opt.nfork),
		zap.Int("buffer", opt.msgBufferSize))
	return e, nil
}

func runHandlerWithoutPanic(h evtHandlerItem, evt *Event) (err error) {
	defer func() {
		if erri := recover(); erri != nil {
			err = errors.Errorf("run event handler `%s` with evt `%s`: %+v", h.name, evt.Topic, erri)
		}
	}()

	h.h(evt)
	return nil
}

type eventRunChanItem struct {
	h   evtHandlerItem
	evt *Event
}

func (e *EventEngine) startRunner(ctx context.Context, nfork int, taskChan chan *eventRunChanItem) {
	for i := 0; i < nfork; i++ {
		logger := e.logger.Named(strconv.Itoa(i))
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-taskChan:
					logger.Debug("trigger handler",
						zap.String("evt", t.evt.Topic),
						zap.String("handler", t.h.name))

					if e.suppressPanic {
						if err := runHandlerWithoutPanic(t.h, t.evt); err != nil {
							logger.Error("panic", zap.Error(err))
						}
					} else {
						t.h.h(t.evt)
					}
				}
			}
		}()
	}
}

// Run start EventEngine
func (e *EventEngine) run(ctx context.Context, taskChan chan *eventRunChanItem) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-e.q:
				hsi, ok := e.topic2hs.Load(evt.Topic)
				if !ok || hsi == nil {
					continue
				}

				hsi.(*evtHandlers).RLock()
				hs := hsi.(*evtHandlers).Clone()
				hsi.(*evtHandlers).RUnlock()

				for _, h := range hs {
					taskChan <- &eventRunChanItem{
						h:   h,
						evt: evt,
					}
				}
			}
		}
	}()
}

// Register register new handler to event store
func (e *EventEngine) Register(topic, handlerName string, handler EventHandler) {
	hs := &evtHandlers{
		hs: []evtHandlerItem{{
			name: handlerName,
			h:    handler,
		}},
	}
	if actual, loaded := e.topic2hs.LoadOrStore(topic, hs); loaded {
		actual.(*evtHandlers).Lock()
		actual.(*evtHandlers).Append(hs.hs...)
		actual.(*evtHandlers).Unlock()
	}

	e.logger.Info("register handler",
		zap.String("topic", topic),
		zap.String("handler", handlerName))
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

	e.logger.Info("unregister handler",
		zap.String("topic", topic),
		zap.String("handler", handlerName))
}

// Publish publish new event
func (e *EventEngine) Publish(evt *Event) {
	evt.Time = Clock.GetUTCNow()
	e.q <- evt
	e.logger.Debug("publish event", zap.String("event", evt.Topic))
}