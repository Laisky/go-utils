package utils

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"
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

// EventTopic topic of event
type EventTopic string

func (h EventTopic) String() string {
	return string(h)
}

// HandlerID id(name) of event handler
type HandlerID string

func (h HandlerID) String() string {
	return string(h)
}

// MetaKey key of event's meta
type MetaKey string

func (h MetaKey) String() string {
	return string(h)
}

// EventMeta event meta
type EventMeta map[MetaKey]interface{}

// Event evt
type Event struct {
	Topic EventTopic
	Time  time.Time
	Meta  EventMeta
	Stack string
}

// EventHandler function to handle event
type EventHandler func(*Event) error

// EventEngine event driven engine
//
// Usage
//
// you -> produce event -> trigger multiply handlers
//
//   1. create an engine by `NewEventEngine`
//   2. register handlers with specified event type by `engine.Register`
//   3. produce event to trigger handlers by `engine.Publish`
type EventEngine struct {
	*eventStoreManagerOpt
	q chan *Event

	// topic2hs map[topic]*sync.Map[handlerID]handler
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
//
// default to 2
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
//
// default to 1
func WithEventEngineChanBuffer(msgBufferSize int) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		if msgBufferSize < 0 {
			return errors.Errorf("msgBufferSize must >= 0")
		}

		opt.msgBufferSize = msgBufferSize
		return nil
	}
}

// WithEventEngineLogger set event store's logger
//
// default to gutils' internal logger
func WithEventEngineLogger(logger *LoggerType) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		if logger == nil {
			return errors.Errorf("logger is nil")
		}

		opt.logger = logger
		return nil
	}
}

// WithEventEngineSuppressPanic set whether suppress event handler's panic
//
// default to false
func WithEventEngineSuppressPanic(suppressPanic bool) EventEngineOptFunc {
	return func(opt *eventStoreManagerOpt) error {
		opt.suppressPanic = suppressPanic
		return nil
	}
}

// NewEventEngine new event store manager
//
// Args:
//   * ctx:
//   * WithEventEngineNFork: n goroutines to run handlers in parallel
//   * WithEventEngineChanBuffer: length of channel to receive published event
//   * WithEventEngineLogger: internal logger in event engine
//   * WithEventEngineSuppressPanic: if is true, will not raise panic when running handler
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

func runHandlerWithoutPanic(h EventHandler, evt *Event) (err error) {
	defer func() {
		if erri := recover(); erri != nil {
			err = errors.Errorf("run event handler with evt `%s`: %+v", evt.Topic, erri)
		}
	}()

	err = h(evt)
	return err
}

type eventRunChanItem struct {
	h   EventHandler
	hid HandlerID
	evt *Event
}

func (e *EventEngine) startRunner(ctx context.Context, nfork int, taskChan chan *eventRunChanItem) {
	for i := 0; i < nfork; i++ {
		logger := e.logger.Named(strconv.Itoa(i))
		go func() {
			var err error
			for {
				select {
				case <-ctx.Done():
					return
				case t := <-taskChan:
					logger.Debug("trigger handler",
						zap.String("evt", t.evt.Topic.String()),
						zap.String("source", t.evt.Stack),
						zap.String("handler", t.hid.String()))

					if e.suppressPanic {
						err = runHandlerWithoutPanic(t.h, t.evt)
					} else {
						err = t.h(t.evt)
					}

					if err != nil {
						logger.Error("run evnet handler",
							zap.String("evt", t.evt.Topic.String()),
							zap.String("handler", t.hid.String()),
							zap.String("source", t.evt.Stack),
							zap.Error(err))
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

				hsi.(*sync.Map).Range(func(hid, h interface{}) bool {
					taskChan <- &eventRunChanItem{
						h:   h.(EventHandler),
						hid: hid.(HandlerID),
						evt: evt,
					}

					return true
				})
			}
		}
	}()
}

// Register register new handler to event store
//
// Args:
//   * topic: specific the topic that will trigger the handler
//   * handlerID: the unique ID of the handler, you can unregister the handler by it's id
//                you can use `gutils.GetHandlerID(handler)` to generate handler's id
//   * handler: the func that used to process event
func (e *EventEngine) Register(topic EventTopic, handlerID HandlerID, handler EventHandler) {
	hs := &sync.Map{}
	actual, _ := e.topic2hs.LoadOrStore(topic, hs)
	actual.(*sync.Map).Store(handlerID, handler)

	e.logger.Info("register handler",
		zap.String("topic", topic.String()),
		zap.String("handler", handlerID.String()))
}

// UnRegister remove handler by id
func (e *EventEngine) UnRegister(topic EventTopic, handlerID HandlerID) {
	if hsi, _ := e.topic2hs.Load(topic); hsi != nil {
		hsi.(*sync.Map).Delete(handlerID)
	}

	e.logger.Info("unregister handler",
		zap.String("topic", topic.String()),
		zap.String("handler", handlerID.String()))
}

// RegisterWithHandler register handler
//
// like `Register`, but can calculate handlerID automatically.
func (e *EventEngine) RegisterWithHandler(topic EventTopic, handler EventHandler) {
	e.Register(topic, GetHandlerID(handler), handler)
}

// UnRegisterWithHandler unregister handler
//
// like `UnRegister`, but can calculate handlerID automatically.
func (e *EventEngine) UnRegisterWithHandler(topic EventTopic, handler EventHandler) {
	e.UnRegister(topic, GetHandlerID(handler))
}

// Publish publish new event
func (e *EventEngine) Publish(evt *Event) {
	evt.Time = Clock.GetUTCNow()
	evt.Stack = string(debug.Stack())
	e.q <- evt
	e.logger.Debug("publish event", zap.String("event", evt.Topic.String()))
}

// GetHandlerID calculate handler func's address as id
func GetHandlerID(handler EventHandler) HandlerID {
	return HandlerID(GetFuncAddress(handler))
}

// GetFuncAddress get address of func
func GetFuncAddress(v interface{}) string {
	ele := reflect.ValueOf(v)
	if ele.Kind() != reflect.Func {
		panic("only accept func")
	}

	return fmt.Sprintf("%x", ele.Pointer())
}
