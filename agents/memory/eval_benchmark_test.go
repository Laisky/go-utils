package memory

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// BenchmarkMemoryEngineBeforeTurn reports hot-path recall cost for future V1-vs-V2 comparison.
func BenchmarkMemoryEngineBeforeTurn(b *testing.B) {
	clock := newEvalClock(time.Date(2026, 3, 8, 15, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{
		RecentContextItems: 10,
		RecallFactsLimit:   10,
		SearchLimit:        5,
		TimeNow:            clock.Now,
	})
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}

	for idx := 0; idx < 20; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "bench",
			SessionID: "before-turn",
			TurnID:    benchmarkTurnID(idx),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "My name is Alice. I prefer concise answers. Today I need close issue backlog.",
				}},
			}},
		})
		if err != nil {
			b.Fatalf("seed after turn: %v", err)
		}
		clock.Advance(time.Minute)
	}

	input := BeforeTurnInput{
		Project:   "bench",
		SessionID: "before-turn",
		CurrentInput: []ResponseItem{{
			Type: "message",
			Role: "user",
			Content: []ResponseContentPart{{Type: "input_text", Text: "What is my preference?"}},
		}},
		MaxInputTok: 120000,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for idx := 0; idx < b.N; idx++ {
		input.TurnID = benchmarkTurnID(idx)
		if _, err = engine.BeforeTurn(context.Background(), input); err != nil {
			b.Fatalf("before turn: %v", err)
		}
	}
}

// BenchmarkMemoryEngineAfterTurn reports write-path cost for future V1-vs-V2 comparison.
func BenchmarkMemoryEngineAfterTurn(b *testing.B) {
	clock := newEvalClock(time.Date(2026, 3, 8, 16, 0, 0, 0, time.UTC))
	storage := newMemoryStorageMock()
	engine, err := NewEngine(storage, Config{
		RecentContextItems: 10,
		RecallFactsLimit:   10,
		SearchLimit:        5,
		TimeNow:            clock.Now,
	})
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for idx := 0; idx < b.N; idx++ {
		err = engine.AfterTurn(context.Background(), AfterTurnInput{
			Project:   "bench",
			SessionID: "after-turn",
			TurnID:    benchmarkTurnID(idx),
			InputItems: []ResponseItem{{
				Type: "message",
				Role: "user",
				Content: []ResponseContentPart{{
					Type: "input_text",
					Text: "My name is Alice. I prefer concise answers. Today I need close issue backlog.",
				}},
			}},
			OutputItems: []ResponseItem{{
				Type: "message",
				Role: "assistant",
				Content: []ResponseContentPart{{Type: "output_text", Text: "Noted."}},
			}},
		})
		if err != nil {
			b.Fatalf("after turn: %v", err)
		}
		clock.Advance(time.Millisecond)
	}
}

// benchmarkTurnID builds one deterministic benchmark turn identifier.
func benchmarkTurnID(idx int) string {
	return fmt.Sprintf("bench-turn-%08d", idx)
}
