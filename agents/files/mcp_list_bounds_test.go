package files

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

// TestMCPListRequestBounds verifies that client and configured defaults cannot
// pass unbounded positive depth or entry counts to the remote listing service.
func TestMCPListRequestBounds(t *testing.T) {
	for _, useDefaults := range []bool{false, true} {
		caller := newMockToolCaller(nil)
		s, err := NewMCPStorage(MCPStorageConfig{Caller: caller, DefaultDepth: math.MaxInt, DefaultLimit: math.MaxInt})
		require.NoError(t, err)
		depth, limit := math.MaxInt, math.MaxInt
		if useDefaults {
			depth, limit = -1, 0
		}
		_, _, err = s.List(t.Context(), "demo", "/memory/alice", depth, limit)
		require.NoError(t, err)
		require.Len(t, caller.calls, 1)
		require.LessOrEqual(t, caller.calls[0].args["depth"].(int), 32)
		require.LessOrEqual(t, caller.calls[0].args["limit"].(int), 5000)
	}
}
