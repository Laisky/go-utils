package shamir

import (
	"testing"

	upstream "github.com/corvus-ch/shamir"
	"github.com/stretchr/testify/require"
)

// TestCombine_WrapsUpstreamError verifies that Combine wraps the error returned
// by the upstream shamir library with context instead of returning it bare.
// Regression found during the documentation-pass error-wrapping sweep.
func TestCombine_WrapsUpstreamError(t *testing.T) {
	t.Parallel()

	parts := map[byte][]byte{1: {1, 2, 3}, 2: {4, 5}}
	_, rawErr := upstream.Combine(parts)
	require.Error(t, rawErr)

	_, err := Combine(parts)
	require.Error(t, err)
	require.NotEqual(t, rawErr.Error(), err.Error())
	require.Contains(t, err.Error(), rawErr.Error())
}
