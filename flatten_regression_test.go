package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFlattenMapSafeNilInputContract pins the nil-input contract preserved by
// the nilnil lint cleanup: a nil map yields a nil (empty, read-only) result and
// no error, an empty map yields a non-nil empty result, and delimiter
// validation still runs before the nil check so an empty delimiter fails.
func TestFlattenMapSafeNilInputContract(t *testing.T) {
	t.Parallel()
	var nilMap map[string]any

	flat, err := FlattenMapSafe(nilMap, ".")
	require.NoError(t, err)
	require.Nil(t, flat)
	require.Empty(t, flat)

	flat, err = FlattenMapSafe(map[string]any{}, ".")
	require.NoError(t, err)
	require.NotNil(t, flat)
	require.Empty(t, flat)
	flat["writable"] = true
	require.Len(t, flat, 1)

	flat, err = FlattenMapSafe(nilMap, "")
	require.Error(t, err)
	require.Nil(t, flat)

	require.NotPanics(t, func() { FlattenMap(nilMap, ".") })
	require.NotPanics(t, func() { FlattenMap(nilMap, "") })
	require.Nil(t, nilMap)
}
