package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortJSONFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("asc", func(t *testing.T) {
		fpath := filepath.Join(dir, "asc.json")
		raw := `{"b": 2, "a": 1, "c": {"z": 26, "y": 25}}`
		err := os.WriteFile(fpath, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONFile(fpath, false, false)
		require.NoError(t, err)

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		expected := `{
  "a": 1,
  "b": 2,
  "c": {
    "y": 25,
    "z": 26
  }
}
`
		require.Equal(t, expected, string(got))
	})

	t.Run("desc", func(t *testing.T) {
		fpath := filepath.Join(dir, "desc.json")
		raw := `{"a": 1, "b": 2, "c": {"y": 25, "z": 26}}`
		err := os.WriteFile(fpath, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONFile(fpath, false, true)
		require.NoError(t, err)

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		expected := `{
  "c": {
    "z": 26,
    "y": 25
  },
  "b": 2,
  "a": 1
}
`
		require.Equal(t, expected, string(got))
	})

	t.Run("recursive", func(t *testing.T) {
		subDir := filepath.Join(dir, "sub")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		fpath1 := filepath.Join(dir, "1.json")
		fpath2 := filepath.Join(subDir, "2.jsonx")
		raw := `{"b": 2, "a": 1}`
		err = os.WriteFile(fpath1, []byte(raw), 0644)
		require.NoError(t, err)
		err = os.WriteFile(fpath2, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONPath(dir, []string{".json", ".jsonx"}, true, false, false)
		require.NoError(t, err)

		for _, f := range []string{fpath1, fpath2} {
			got, err := os.ReadFile(f)
			require.NoError(t, err)
			expected := `{
  "a": 1,
  "b": 2
}
`
			require.Equal(t, expected, string(got))
		}
	})

	t.Run("dry run", func(t *testing.T) {
		fpath := filepath.Join(dir, "dry.json")
		raw := `{"b": 2, "a": 1}`
		err := os.WriteFile(fpath, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONFile(fpath, true, false)
		require.NoError(t, err)

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, raw, string(got), "file should not be changed in dry run")
	})
}

func TestSortRecursive(t *testing.T) {
	t.Run("ordered keys", func(t *testing.T) {
		data := map[string]interface{}{
			"b": 2,
			"a": 1,
			"c": []interface{}{
				map[string]interface{}{"y": 2, "x": 1},
				3,
			},
		}

		// test asc
		sorted := sortRecursive(data, false)
		sm := sorted.(sortedMap)
		sort.Strings(sm.keys)
		require.Equal(t, []string{"a", "b", "c"}, sm.keys)

		// test desc
		sortedDesc := sortRecursive(data, true)
		smDesc := sortedDesc.(sortedMap)
		sort.Slice(smDesc.keys, func(i, j int) bool {
			return smDesc.keys[i] > smDesc.keys[j]
		})
		require.Equal(t, []string{"c", "b", "a"}, smDesc.keys)
	})
}
