//go:build linux

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTransactionalConsolidationLostAcknowledgementDeduplicates checks stable
// insight IDs suppress a repeated commit and its metrics update after lost acknowledgement.
func TestTransactionalConsolidationLostAcknowledgementDeduplicates(t *testing.T) {
	for _, loseAck := range []bool{false, true} {
		name := "acknowledged"
		if loseAck {
			name = "lost_acknowledgement"
		}
		t.Run(name, func(t *testing.T) {
			backend := newTurnBackend(t)
			now := recoveryTime
			conf := Config{TimeNow: func() time.Time { return now }, ConsolidationMinEvents: 2}
			seed, err := NewTransactionalEngine(backend, conf)
			require.NoError(t, err)
			writePreference := func(turn, text string) {
				t.Helper()
				in := recoveryInput(turn)
				in.InputItems[0].Content[0].Text = text
				require.NoError(t, seed.AfterTurn(t.Context(), in))
			}
			writePreference("one", "I prefer concise answers.")
			writePreference("two", "I prefer concise answers for status updates.")
			faulty := &turnFaultBackend{TransactionalEngine: backend, loseAck: loseAck}
			first, err := NewTransactionalEngine(faulty, conf)
			require.NoError(t, err)
			err = first.RunConsolidation(t.Context(), recoveryProject, recoverySession)
			if loseAck {
				require.ErrorContains(t, err, "lost commit acknowledgement")
			} else {
				require.NoError(t, err)
			}
			reader, err := NewEngine(backend, conf)
			require.NoError(t, err)
			insights, err := reader.loadInsights(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.NotEmpty(t, insights, "the first consolidation must actually publish an insight")
			metrics, err := reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Equal(t, 1, metrics.ConsolidationRunCount)
			committed := turnSnapshot(t, backend)
			now = now.Add(24 * time.Hour)
			retry, err := NewTransactionalEngine(backend, conf)
			require.NoError(t, err)
			for range 3 {
				require.NoError(t, retry.RunConsolidation(t.Context(), recoveryProject, recoverySession))
				require.Equal(t, committed, turnSnapshot(t, backend), "unchanged facts must not append insights or increment counters")
			}
			// A genuine new supporting fact must still produce a new insight and counter increment.
			writePreference("three", "I prefer detailed answers for incident investigations.")
			require.NoError(t, retry.RunConsolidation(t.Context(), recoveryProject, recoverySession))
			newer, err := reader.loadInsights(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Greater(t, len(newer), len(insights), "the deduplication control must not disable consolidation")
			metrics, err = reader.loadMetrics(t.Context(), recoveryProject, recoverySession)
			require.NoError(t, err)
			require.Equal(t, 2, metrics.ConsolidationRunCount)
		})
	}
}
