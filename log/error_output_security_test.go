package log

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSecurity77ExactErrorSinks verifies exact replacement and ownership of the path slice.
func TestSecurity77ExactErrorSinks(t *testing.T) {
	backing := []string{"internal-only.log", "CANARY"}
	paths := backing[:1]
	configured := &option{}
	configured.ErrorOutputPaths = []string{"stderr"}
	require.NoError(t, WithErrorOutputPaths(paths)(configured))
	require.Equal(t, []string{"internal-only.log"}, configured.ErrorOutputPaths)
	require.Equal(t, "CANARY", backing[1])
	paths[0] = "mutated-by-caller"
	require.Equal(t, "internal-only.log", configured.ErrorOutputPaths[0])
	configured.ErrorOutputPaths[0] = "mutated-config"
	require.Equal(t, "mutated-by-caller", paths[0])
	for _, empty := range [][]string{nil, {}} {
		configured.ErrorOutputPaths = []string{"stderr"}
		require.NoError(t, WithErrorOutputPaths(empty)(configured))
		require.Len(t, configured.ErrorOutputPaths, 0)
	}
	a, b := &option{}, &option{}
	set := WithErrorOutputPaths([]string{"one.log"})
	require.NoError(t, set(a))
	require.NoError(t, set(b))
	a.ErrorOutputPaths[0] = "changed"
	require.Equal(t, "one.log", b.ErrorOutputPaths[0])
}
