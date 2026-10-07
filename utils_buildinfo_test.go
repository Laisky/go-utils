package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPrettyBuildInfo verifies that PrettyBuildInfo without options emits the GoVersion and Main keys with
// "Deps" set to null, and that WithPrettyBuildInfoDeps makes it emit the Deps key as well.
func TestPrettyBuildInfo(t *testing.T) {
	t.Parallel()

	t.Run("no deps", func(t *testing.T) {
		ret := PrettyBuildInfo()
		require.Contains(t, ret, `"GoVersion"`)
		require.Contains(t, ret, `"Main":`)
		require.Contains(t, ret, `"Deps": null`)
	})

	t.Run("with deps", func(t *testing.T) {
		ret := PrettyBuildInfo(
			WithPrettyBuildInfoDeps(),
		)
		require.Contains(t, ret, `"GoVersion"`)
		require.Contains(t, ret, `"Main":`)
		require.Contains(t, ret, `"Deps":`)
	})
}
