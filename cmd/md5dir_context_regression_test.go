package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMd5DirFilesHonorsCanceledContext verifies that md5dir threads its context
// into content hashing: a canceled context stops processing before any file is
// hashed, published or removed, in both move and copy modes. It guards the
// contextcheck lint finding in placeMd5File, which hashed with FileHash and so
// ignored cancellation.
func TestMd5DirFilesHonorsCanceledContext(t *testing.T) {
	saved := md5DirSaveCaption
	t.Cleanup(func() { md5DirSaveCaption = saved })
	md5DirSaveCaption = func(context.Context, string, string) error { return nil }

	for mode, remain := range md5DirModes {
		t.Run(mode, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			src := writeDedupeFile(t, source, "photo.jpg", "CANCELED")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := md5DirFiles(ctx, []string{src}, target, remain)
			require.ErrorIs(t, err, context.Canceled)
			requireDedupeContent(t, src, "CANCELED")
			_, statErr := os.Stat(md5Target(target, "CANCELED", ".jpg"))
			require.ErrorIs(t, statErr, os.ErrNotExist, "nothing may be published after cancellation")
		})
	}
}
