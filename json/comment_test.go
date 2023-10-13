package json

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmarshalCommentFromString(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		raw := []byte(`// comment
		{
			// comment
			"key": "value" // comment
		}`)
		m := map[string]interface{}{}

		require.Contains(t, string(raw), "// comment")
		err := UnmarshalComment(raw, &m)
		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{
			"key": "value",
		}, m)

		// UnmarshalComment will remove all comments in raw
		require.NotContains(t, string(raw), "// comment")
	})

	t.Run("not support comment", func(t *testing.T) {
		raw := []byte(`// comment
		{
			// comment
			"key": "value" // comment
		}`)
		m := map[string]interface{}{}

		err := Unmarshal(raw, &m)
		require.ErrorContains(t, err, "invalid character '/' looking for beginning of value")
		require.Empty(t, m)
	})
}
