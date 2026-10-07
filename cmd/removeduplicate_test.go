package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v6"
)

// Test_removeDuplicate runs removeDuplicate in non-dry mode ten times in parallel, each on a fresh temporary
// tree holding four copies of one content and two copies of another spread over a directory and its
// subdirectory, and verifies that exactly one file of each content survives. It guards against the earlier
// racy sync.Map usage that made concurrent scans keep or delete the wrong number of files.
func Test_removeDuplicate(t *testing.T) {
	t.Parallel()
	// glog.Shared.ChangeLevel(glog.LevelDebug)

	// Previously, this test case had a 20% chance of failure
	// due to incorrect usage of sync.Map. The bug has been fixed.
	for i := 0; i < 10; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			dir, err := os.MkdirTemp("", "removeDuplicate*")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			const (
				valA = "123"
				valB = "2134"
			)

			// prepare files in dir
			err = os.WriteFile(filepath.Join(dir, "a.txt"), []byte(valA), 0600)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(dir, "b.txt"), []byte(valA), 0600)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(dir, "c.txt"), []byte(valB), 0600)
			require.NoError(t, err)

			// prepare files in dir/laisky
			dir2 := filepath.Join(dir, "laisky")
			require.NoError(t, os.MkdirAll(dir2, 0777))
			err = os.WriteFile(filepath.Join(dir2, "a.txt"), []byte(valA), 0600)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(dir2, "b.txt"), []byte(valA), 0600)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(dir2, "c.txt"), []byte(valB), 0600)
			require.NoError(t, err)

			// run removeDuplicate
			err = removeDuplicate(false, dir)
			require.NoError(t, err)

			// check result
			aExists0, err := gutils.FileExists(filepath.Join(dir, "a.txt"))
			require.NoError(t, err)
			aExists1, err := gutils.FileExists(filepath.Join(dir, "b.txt"))
			require.NoError(t, err)
			bExists0, err := gutils.FileExists(filepath.Join(dir, "c.txt"))
			require.NoError(t, err)

			aExists2, err := gutils.FileExists(filepath.Join(dir2, "a.txt"))
			require.NoError(t, err)
			aExists3, err := gutils.FileExists(filepath.Join(dir2, "b.txt"))
			require.NoError(t, err)
			bExists1, err := gutils.FileExists(filepath.Join(dir2, "c.txt"))
			require.NoError(t, err)

			var (
				numValAExists, numValBExists int
			)
			for _, val := range []bool{
				aExists0, aExists1, aExists2, aExists3,
			} {
				if val {
					numValAExists++
				}
			}
			require.Equal(t, 1, numValAExists)

			for _, val := range []bool{
				bExists0, bExists1,
			} {
				if val {
					numValBExists++
				}
			}
			require.Equal(t, 1, numValBExists)
		})
	}
}
