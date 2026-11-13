//go:build linux

package transactionlocal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	storage "github.com/Laisky/go-utils/v6/agents/memory/storage"
	"github.com/Laisky/go-utils/v6/agents/memory/storage/local"
)

// BenchmarkTransactionSnapshotScaling compares one-byte reads and tiny writes with bounded histories.
func BenchmarkTransactionSnapshotScaling(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		for _, write := range []bool{false, true} {
			operation := "read"
			if write {
				operation = "write"
			}
			b.Run(fmt.Sprintf("%s/%dMiB", operation, size>>20), func(b *testing.B) {
				root := b.TempDir()
				require.NoError(b, os.Chmod(root, 0o700))
				engine, err := NewEngine(local.Config{RootDir: root})
				require.NoError(b, err)
				ctx := context.Background()
				require.NoError(b, engine.Write(ctx, "p", "/unrelated/history", strings.Repeat("x", size), storage.WriteModeTruncate, 0))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					err = engine.Transaction(ctx, "p", "small", func(view storage.Engine) error {
						if write {
							return view.Write(ctx, "p", "/small/value", "tiny", storage.WriteModeTruncate, 0)
						}
						_, readErr := view.Read(ctx, "p", "/unrelated/history", 0, 1)
						return readErr
					})
					require.NoError(b, err)
				}
			})
		}
	}
}
