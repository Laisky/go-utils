package memory

import (
	"context"
	"github.com/Laisky/errors/v2"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
)

// TestArchiveFailureDoesNotBlockRetention reproduces binary MCP corruption while
// checking independent physical retention and the successful portable control.
func TestArchiveFailureDoesNotBlockRetention(t *testing.T) {
	for _, format := range []ArchiveFormat{ArchiveFormatZstd, ArchiveFormatZstdBase64V1} {
		t.Run(map[ArchiveFormat]string{ArchiveFormatZstd: "binary", ArchiveFormatZstdBase64V1: "portable"}[format], func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			backend := newMemoryStorageMock()
			storage := newArchiveMCPStorage(t, &archiveJSONTransport{storage: backend})
			engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }, ArchiveFormat: format})
			require.NoError(t, err)
			const session = "isolation"
			require.NoError(t, engine.ensureSessionScaffold(ctx, "demo", session))
			before, err := engine.loadMeta(ctx, "demo", session)
			require.NoError(t, err)
			old := rawLogShardPath(session, now.AddDate(0, 0, -2))
			const raw = "{\"id\":\"old-event\",\"summary\":\"Unicode 世界\"}\n"
			require.NoError(t, storage.Write(ctx, "demo", old, raw, files.WriteModeTruncate, 0))
			for _, tier := range []string{memoryTierL1, memoryTierL2} {
				body, marshalErr := marshalFacts([]MemoryFact{
					{ID: "expired", FactID: "expired", Type: "fact_upsert", Tier: tier, ExpiresAt: now.Add(-time.Hour).Format(time.RFC3339)},
					{ID: "active", FactID: "active", Type: "fact_upsert", Tier: tier, ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)},
				})
				require.NoError(t, marshalErr)
				require.NoError(t, storage.Write(ctx, "demo", tierFactsShardPath(session, tier, now), body, files.WriteModeTruncate, 0))
			}
			// A refreshed summary must describe current state, not retain this sentinel.
			require.NoError(t, storage.Write(ctx, "demo", sessionBasePath(session)+"/.overview", "stale", files.WriteModeTruncate, 0))
			err = engine.RunMaintenance(ctx, "demo", session)
			if format == ArchiveFormatZstd {
				require.ErrorContains(t, err, "archive raw shards")
				retained, readErr := storage.Read(ctx, "demo", old, 0, -1)
				require.NoError(t, readErr)
				require.Equal(t, raw, retained)
			} else {
				require.NoError(t, err)
				info, statErr := storage.Stat(ctx, "demo", old)
				require.NoError(t, statErr)
				require.False(t, info.Exists)
			}
			for _, tier := range []string{memoryTierL1, memoryTierL2} {
				body, readErr := storage.Read(ctx, "demo", tierFactsShardPath(session, tier, now), 0, -1)
				require.NoError(t, readErr)
				require.Contains(t, body, "\"id\":\"active\"")
				require.NotContains(t, body, "\"id\":\"expired\"")
			}
			summary, err := storage.Read(ctx, "demo", sessionBasePath(session)+"/.overview", 0, -1)
			require.NoError(t, err)
			require.NotEqual(t, "stale", summary)
			after, err := engine.loadMeta(ctx, "demo", session)
			require.NoError(t, err)
			if format == ArchiveFormatZstd {
				require.Equal(t, before.LastMaintenanceAt, after.LastMaintenanceAt)
			} else {
				require.Equal(t, now.Format(time.RFC3339), after.LastMaintenanceAt)
			}
		})
	}
}

// TestMaintenancePartialFailure verifies error identity/aggregation, dependency
// gating, independent L2 progress and absence of storage calls after cancellation.
func TestMaintenancePartialFailure(t *testing.T) {
	for _, scenario := range []string{"success", "archive", "L1", "multiple", "cancel", "already-canceled", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			const session = "partial"
			backend := newMemoryStorageMock()
			engine, err := NewEngine(backend, Config{TimeNow: func() time.Time { return now }})
			require.NoError(t, err)
			require.NoError(t, engine.ensureSessionScaffold(ctx, "demo", session))
			raw := rawLogShardPath(session, now.AddDate(0, 0, -2))
			require.NoError(t, backend.Write(ctx, "demo", raw, "{\"id\":\"old\"}\n", files.WriteModeTruncate, 0))
			for _, tier := range []string{memoryTierL1, memoryTierL2} {
				body, marshalErr := marshalFacts([]MemoryFact{{ID: "expired", ExpiresAt: now.Add(-time.Hour).Format(time.RFC3339)}})
				require.NoError(t, marshalErr)
				require.NoError(t, backend.Write(ctx, "demo", tierFactsShardPath(session, tier, now), body, files.WriteModeTruncate, 0))
			}
			before, err := engine.loadMeta(ctx, "demo", session)
			require.NoError(t, err)
			archiveErr := errors.New("synthetic archive failure")
			tierErr := errors.New("synthetic L1 failure")
			summaryErr := errors.New("synthetic summary failure")
			afterCancel, rawLists := 0, 0
			check := func() {
				if ctx.Err() != nil {
					afterCancel++
				}
			}
			probe := &storageStub{
				statFn: func(c context.Context, p, f string) (files.FileInfo, error) { check(); return backend.Stat(c, p, f) },
				readFn: func(c context.Context, p, f string, o, n int64) (string, error) {
					check()
					if strings.Contains(f, "/archive/") {
						switch scenario {
						case "archive", "multiple":
							return "", archiveErr
						case "cancel":
							cancel()
							return "", archiveErr
						case "deadline":
							return "", context.DeadlineExceeded
						}
					}
					return backend.Read(c, p, f, o, n)
				},
				writeFn: func(c context.Context, p, f, b string, m files.WriteMode, o int64) error {
					check()
					if scenario == "multiple" && f == sessionBasePath(session)+"/.overview" {
						return summaryErr
					}
					return backend.Write(c, p, f, b, m, o)
				},
				listFn: func(c context.Context, p, f string, d, n int) ([]files.FileInfo, bool, error) {
					check()
					if f == eventsRawRootPath(session) && d == 16 {
						rawLists++
					}
					if (scenario == "L1" || scenario == "multiple") && f == tierRootPath(session, memoryTierL1) && d == 16 {
						return nil, false, tierErr
					}
					return backend.List(c, p, f, d, n)
				},
				deleteFn: func(c context.Context, p, f string, r bool) error { check(); return backend.Delete(c, p, f, r) },
				searchFn: func(c context.Context, p, q, f string, n int) ([]files.FileChunk, error) {
					check()
					return backend.Search(c, p, q, f, n)
				},
			}
			engine.storage = probe
			if scenario == "already-canceled" {
				cancel()
			}
			err = engine.RunMaintenance(ctx, "demo", session)
			if scenario == "success" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Zero(t, afterCancel, "no new storage calls after context cancellation")
			if scenario == "archive" || scenario == "multiple" || scenario == "cancel" {
				require.ErrorIs(t, err, archiveErr)
				require.ErrorContains(t, err, "archive raw shards")
			}
			if scenario == "L1" || scenario == "multiple" {
				require.ErrorIs(t, err, tierErr)
				require.ErrorContains(t, err, "sweep expired L1 facts")
			}
			if scenario == "multiple" {
				require.ErrorIs(t, err, summaryErr)
				require.ErrorContains(t, err, "refresh summaries")
			}
			if scenario == "cancel" || scenario == "already-canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if scenario == "deadline" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			expectedRawLists := 1
			if scenario == "success" {
				expectedRawLists = 2
			}
			if scenario == "already-canceled" {
				expectedRawLists = 0
			}
			require.Equal(t, expectedRawLists, rawLists, "consolidation must not read retained raw data after upstream failure")
			l2, statErr := backend.Stat(context.Background(), "demo", tierFactsShardPath(session, memoryTierL2, now))
			require.NoError(t, statErr)
			stopped := scenario == "cancel" || scenario == "already-canceled" || scenario == "deadline"
			require.Equal(t, stopped, l2.Exists, "independent L2 retention proceeds unless cancellation stops the cycle")
			// Read through the backend, not the canceled/failing probe.
			engine.storage = backend
			after, metaErr := engine.loadMeta(context.Background(), "demo", session)
			require.NoError(t, metaErr)
			if scenario == "success" {
				require.Equal(t, now.Format(time.RFC3339), after.LastMaintenanceAt)
			} else {
				require.Equal(t, before.LastMaintenanceAt, after.LastMaintenanceAt)
			}
		})
	}
}
