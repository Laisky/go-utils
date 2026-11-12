package memory

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/go-utils/v6/agents/files"
	storageengine "github.com/Laisky/go-utils/v6/agents/memory/storage"
	localstorage "github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// TestArchivePortableJSONRoundTrip verifies the opt-in envelope survives real
// JSON transport and remains decodable by a standard zstd reader.
func TestArchivePortableJSONRoundTrip(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "corrupt-readback"}[corrupt], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			backend := newMemoryStorageMock()
			storage := newArchiveMCPStorage(t, &archiveJSONTransport{storage: backend, corrupt: corrupt})
			engine, err := NewEngine(storage, Config{
				TimeNow: func() time.Time { return now }, ArchiveFormat: ArchiveFormatZstdBase64V1,
			})
			require.NoError(t, err)
			day := now.AddDate(0, 0, -2)
			rawPath := rawLogShardPath("portable", day)
			archivePath := archiveShardPath("portable", day, pathBase(rawPath))
			const raw = "{\"id\":\"event\",\"summary\":\"Unicode: 世界\"}\n"
			require.NoError(t, storage.Write(ctx, "demo", rawPath, raw, files.WriteModeTruncate, 0))
			err = engine.RunMaintenance(ctx, "demo", "portable")
			if corrupt {
				require.Error(t, err)
				retained, readErr := storage.Read(ctx, "demo", rawPath, 0, -1)
				require.NoError(t, readErr)
				require.Equal(t, raw, retained)
				return
			}
			require.NoError(t, err)
			info, err := storage.Stat(ctx, "demo", rawPath)
			require.NoError(t, err)
			require.False(t, info.Exists)
			info, err = storage.Stat(ctx, "demo", archivePath)
			require.NoError(t, err)
			require.False(t, info.Exists, "portable format must not masquerade as a legacy .zst")
			stored, err := storage.Read(ctx, "demo", archivePath+".b64.json", 0, -1)
			require.NoError(t, err)
			require.True(t, utf8.ValidString(stored))
			frame, err := DecodeArchive(ArchiveFormatZstdBase64V1, stored)
			require.NoError(t, err)
			assertArchiveDecompresses(t, frame, raw)
		})
	}
}

// assertArchiveDecompresses checks a small frame with the standard zstd decoder.
func assertArchiveDecompresses(t *testing.T, frame []byte, want string) {
	t.Helper()
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer decoder.Close()
	got, err := decoder.DecodeAll(frame, nil)
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

// TestArchiveLocalCompatibility preserves historical binary files and proves
// old-writer/new-reader and new-writer/old-reader behavior on a real local backend.
func TestArchiveLocalCompatibility(t *testing.T) {
	ctx := context.Background()
	storage, err := localstorage.NewEngine(localstorage.Config{RootDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	engine, err := NewEngine(storage, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)
	// Immutable v1.20.1 zstd fixture retained from zstd_compat_test.go.
	const historical = "28b52ffd64f8a6ad0100b40243726f73732d76657273696f6e206d656d6f727920617263686976653a2068656c6c6f2c207a737464210a0154170533533eb9ec911024"
	oldFrame, err := hex.DecodeString(historical)
	require.NoError(t, err)
	const oldPath = "/memory/local/events/archive/2026/01/01/historical.jsonl.zst"
	require.NoError(t, storage.Write(ctx, "demo", oldPath, string(oldFrame), files.WriteModeTruncate, 0))
	stored, err := storage.Read(ctx, "demo", oldPath, 0, -1)
	require.NoError(t, err)
	decoded, err := DecodeArchive(ArchiveFormatZstd, stored)
	require.NoError(t, err)
	require.Equal(t, oldFrame, decoded)
	assertArchiveDecompresses(t, decoded, strings.Repeat("Cross-version memory archive: hello, zstd!\n", 1000))

	day := now.AddDate(0, 0, -2)
	rawPath := rawLogShardPath("local", day)
	const raw = "{\"id\":\"new-event\"}\n"
	require.NoError(t, storage.Write(ctx, "demo", rawPath, raw, files.WriteModeTruncate, 0))
	require.NoError(t, engine.archiveRawShards(ctx, "demo", "local"))
	stored, err = storage.Read(ctx, "demo", archiveShardPath("local", day, pathBase(rawPath)), 0, -1)
	require.NoError(t, err)
	assertArchiveDecompresses(t, []byte(stored), raw) // Historical reader: raw zstd.
	unchanged, err := storage.Read(ctx, "demo", oldPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, string(oldFrame), unchanged)
}

// archiveReadFailure returns a transient verification error, leaving writes intact.
type archiveReadFailure struct{ storageengine.Engine }

// Read fails archive readback and delegates ordinary reads to the underlying engine.
func (storage archiveReadFailure) Read(ctx context.Context, project, filePath string, offset, length int64) (string, error) {
	if strings.Contains(filePath, "/archive/") {
		return "", errors.WithStack(context.DeadlineExceeded)
	}
	return storage.Engine.Read(ctx, project, filePath, offset, length)
}

// TestArchiveReadFailureRetainsRaw verifies failed verification never authorizes deletion.
func TestArchiveReadFailureRetainsRaw(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	storage := newMemoryStorageMock()
	engine, err := NewEngine(archiveReadFailure{storage}, Config{TimeNow: func() time.Time { return now }})
	require.NoError(t, err)
	rawPath := rawLogShardPath("read-failure", now.AddDate(0, 0, -2))
	const raw = "{\"id\":\"retained\"}\n"
	require.NoError(t, storage.Write(ctx, "demo", rawPath, raw, files.WriteModeTruncate, 0))
	require.Error(t, engine.archiveRawShards(ctx, "demo", "read-failure"))
	retained, err := storage.Read(ctx, "demo", rawPath, 0, -1)
	require.NoError(t, err)
	require.Equal(t, raw, retained)
}

// TestArchiveFormatRejectsInvalid verifies explicit formats and strict envelopes.
func TestArchiveFormatRejectsInvalid(t *testing.T) {
	_, err := NewEngine(newMemoryStorageMock(), Config{ArchiveFormat: "unknown"})
	require.Error(t, err)
	for _, content := range []string{
		"not json", `{}`, `{"format":"future","data":"YQ=="}`,
		`{"format":"zstd-base64-v1","data":"!"}`,
	} {
		_, err = DecodeArchive(ArchiveFormatZstdBase64V1, content)
		require.Error(t, err)
	}
}
