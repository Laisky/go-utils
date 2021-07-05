package utils

import (
	"context"
	"fmt"
	"testing"

	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"
)

func ExampleEventEngine() {
	evtstore, err := NewEventEngine(context.Background())
	if err == nil {
		Logger.Panic("new evt engine", zap.Error(err))
	}

	var (
		topic1 EventTopic = "t1"
		topic2 EventTopic = "t2"
	)
	evt1 := &Event{
		Topic: topic1,
		Meta: EventMeta{
			"name": "yo",
		},
	}
	evt2 := &Event{
		Topic: topic2,
		Meta: EventMeta{
			"name": "yo2",
		},
	}

	handler := func(evt *Event) error {
		fmt.Printf("got event %s: %v\n", evt.Topic, evt.Meta)
		return nil
	}

	evtstore.Register(topic1, "handler", handler)
	evtstore.Publish(evt1) // print: got event t1: map[name]yo
	evtstore.Publish(evt2) // print: got event t2: map[name]yo2

	evtstore.UnRegister(topic1, "handler")
	evtstore.Publish(evt1) // nothing print
	evtstore.Publish(evt2) // nothing print

}

func TestNewEventEngine(t *testing.T) {
	evtstore, err := NewEventEngine(context.Background())
	require.NoError(t, err)

	var (
		topic1 EventTopic = "t1"
		topic2 EventTopic = "t2"
	)
	evt1 := &Event{
		Topic: topic1,
		Meta: EventMeta{
			"name": "yo",
		},
	}
	evt2 := &Event{
		Topic: topic2,
		Meta: EventMeta{
			"name": "yo2",
		},
	}

	handler := func(evt *Event) error {
		t.Logf("got event %s: %+v", evt.Topic, evt.Meta)
		return nil
	}

	evtstore.Register(topic1, "handler", handler)
	evtstore.Publish(evt1)
	evtstore.Publish(evt2)

	evtstore.UnRegister(topic1, "handler")
	evtstore.Publish(evt1)
	evtstore.Publish(evt2)

	// t.Error()
}

func BenchmarkNewEventEngine(b *testing.B) {
	evtstore, err := NewEventEngine(context.Background())
	if err != nil {
		b.Fatalf("%+v", err)
	}

	var (
		topic1 EventTopic = "t1"
		topic2 EventTopic = "t2"
	)
	evt1 := &Event{
		Topic: topic1,
		Meta: EventMeta{
			"name": "yo",
		},
	}
	evt2 := &Event{
		Topic: topic2,
		Meta: EventMeta{
			"name": "yo2",
		},
	}

	handler := func(evt *Event) error {
		b.Logf("got event %s: %+v", evt.Topic, evt.Meta)
		return nil
	}

	evtstore.Register(topic1, "handler", handler)

	b.Run("publish", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			evtstore.Publish(evt1)
			evtstore.Publish(evt2)
		}
	})

	// b.Error()
}
