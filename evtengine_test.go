package utils

import (
	"context"
	"testing"
)

func TestNewEventEngine(t *testing.T) {
	evtstore, err := NewEventEngine(context.Background())
	if err != nil {
		t.Fatalf("%+v", err)
	}

	topic1 := "t1"
	topic2 := "t2"
	evt1 := &Event{
		Topic: topic1,
		Meta: map[string]interface{}{
			"name": "yo",
		},
	}
	evt2 := &Event{
		Topic: topic2,
		Meta: map[string]interface{}{
			"name": "yo2",
		},
	}

	handler := func(evt *Event) {
		t.Logf("got event %s: %+v", evt.Topic, evt.Meta)
	}

	evtstore.Register(topic1, "handler", handler)
	evtstore.Publish(evt1)
	evtstore.Publish(evt2)

	// t.Error()
}

func BenchmarkNewEventEngine(b *testing.B) {
	evtstore, err := NewEventEngine(context.Background())
	if err != nil {
		b.Fatalf("%+v", err)
	}

	topic1 := "t1"
	topic2 := "t2"
	evt1 := &Event{
		Topic: topic1,
		Meta: map[string]interface{}{
			"name": "yo",
		},
	}
	evt2 := &Event{
		Topic: topic2,
		Meta: map[string]interface{}{
			"name": "yo2",
		},
	}

	handler := func(evt *Event) {
		b.Logf("got event %s: %+v", evt.Topic, evt.Meta)
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
