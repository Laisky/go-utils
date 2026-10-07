package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSortJSONFile verifies that sortJSONFile rewrites a file with keys sorted ascending or descending at every
// nesting level using the requested indent width, that dry mode leaves the file untouched, and that
// sortJSONPath in recursive mode sorts files matching any of the given extensions inside subdirectories.
func TestSortJSONFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("asc", func(t *testing.T) {
		fpath := filepath.Join(dir, "asc.json")
		raw := `{"b": 2, "a": 1, "c": {"z": 26, "y": 25}}`
		err := os.WriteFile(fpath, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONFile(fpath, false, false, 2, false)
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

		err = sortJSONFile(fpath, false, true, 2, false)
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

		err = sortJSONPath(dir, []string{".json", ".jsonx"}, true, false, false, 2, false)
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

		err = sortJSONFile(fpath, true, false, 2, false)
		require.NoError(t, err)

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		require.Equal(t, raw, string(got), "file should not be changed in dry run")
	})
	t.Run("indent", func(t *testing.T) {
		fpath := filepath.Join(dir, "indent.json")
		raw := `{"b": 2, "a": 1}`
		err := os.WriteFile(fpath, []byte(raw), 0644)
		require.NoError(t, err)

		err = sortJSONFile(fpath, false, false, 4, false)
		require.NoError(t, err)

		got, err := os.ReadFile(fpath)
		require.NoError(t, err)
		expected := `{
    "a": 1,
    "b": 2
}
`
		require.Equal(t, expected, string(got))
	})
}

// TestSortRecursive verifies that sortRecursive wraps a top-level map, whose values include a slice holding a
// nested map, into a sortedMap carrying exactly the original keys in both ascending and descending modes. The
// test re-sorts the keys itself, so it checks the key set rather than the emitted order.
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

		// sortedMap orders keys when it is marshaled, so assert the emitted
		// JSON instead of re-sorting the keys in the test, which would hide an
		// ordering bug.
		asc, err := json.Marshal(sortRecursive(data, false, false))
		require.NoError(t, err)
		require.Equal(t, `{"a":1,"b":2,"c":[{"x":1,"y":2},3]}`, string(asc))

		desc, err := json.Marshal(sortRecursive(data, true, false))
		require.NoError(t, err)
		require.Equal(t, `{"c":[{"y":2,"x":1},3],"b":2,"a":1}`, string(desc))
	})
}

// TestSortJSONKeysOrder verifies the key order sortJSONFile writes for edge cases: byte-wise ordering of
// digits, upper-case and lower-case letters in both directions, special characters, empty keys, UTF-8 keys
// and nested objects, plus the case-insensitive mode that groups keys such as "A" and "a" together and keeps
// "ServerAddress" before the SMTP-prefixed keys.
func TestSortJSONKeysOrder(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "keys.json")

	tests := []struct {
		name        string
		input       string
		desc        bool
		insensitive bool
		expected    string
	}{
		{
			name:  "mixed case and numbers asc",
			input: `{"b": 1, "A": 1, "1": 1, "a": 1, "B": 1, "2": 1}`,
			desc:  false,
			expected: `{
  "1": 1,
  "2": 1,
  "A": 1,
  "B": 1,
  "a": 1,
  "b": 1
}
`,
		},
		{
			name:  "mixed case and numbers desc",
			input: `{"b": 1, "A": 1, "1": 1, "a": 1, "B": 1, "2": 1}`,
			desc:  true,
			expected: `{
  "b": 1,
  "a": 1,
  "B": 1,
  "A": 1,
  "2": 1,
  "1": 1
}
`,
		},
		{
			name:  "same letter different case asc",
			input: `{"a": 1, "A": 1}`,
			desc:  false,
			expected: `{
  "A": 1,
  "a": 1
}
`,
		},
		{
			name:  "different letters different case asc",
			input: `{"b": 1, "A": 1}`,
			desc:  false,
			expected: `{
  "A": 1,
  "b": 1
}
`,
		},
		{
			name:  "numbers and letters asc",
			input: `{"a": 1, "1": 1, "A": 1}`,
			desc:  false,
			expected: `{
  "1": 1,
  "A": 1,
  "a": 1
}
`,
		},
		{
			name:  "special characters asc",
			input: `{"_": 1, "-": 1, "@": 1, " ": 1}`,
			desc:  false,
			expected: `{
  " ": 1,
  "-": 1,
  "@": 1,
  "_": 1
}
`,
		},
		{
			name:  "deeply nested mixed cases asc",
			input: `{"v": {"B": 2, "a": 1}, "V": {"b": 2, "A": 1}}`,
			desc:  false,
			expected: `{
  "V": {
    "A": 1,
    "b": 2
  },
  "v": {
    "B": 2,
    "a": 1
  }
}
`,
		},
		{
			name:  "empty key asc",
			input: `{"a": 1, "": 2}`,
			desc:  false,
			expected: `{
  "": 2,
  "a": 1
}
`,
		},
		{
			name:  "utf8 keys asc",
			input: `{"你好": 1, "世界": 2, "a": 3, "1": 4}`,
			desc:  false,
			expected: `{
  "1": 4,
  "a": 3,
  "世界": 2,
  "你好": 1
}
`,
		},
		{
			name:        "user reported case insensitive asc",
			insensitive: true,
			input: `{
				"ServerAddress": "",
				"SMTPAccount": "",
				"SMTPFrom": "",
				"SMTPPort": "",
				"SMTPServer": "",
				"SMTPToken": ""
			}`,
			expected: `{
  "ServerAddress": "",
  "SMTPAccount": "",
  "SMTPFrom": "",
  "SMTPPort": "",
  "SMTPServer": "",
  "SMTPToken": ""
}
`,
		},
		{
			name:        "mixed case same letters insensitive asc",
			insensitive: true,
			input:       `{"b": 1, "A": 1, "a": 1, "B": 1}`,
			expected: `{
  "A": 1,
  "a": 1,
  "B": 1,
  "b": 1
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(fpath, []byte(tt.input), 0644)
			require.NoError(t, err)

			err = sortJSONFile(fpath, false, tt.desc, 2, tt.insensitive)
			require.NoError(t, err)

			got, err := os.ReadFile(fpath)
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(got))
		})
	}
}
