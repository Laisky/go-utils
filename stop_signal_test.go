package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStopSignalRejectsEmptyOptions preserves option validation on every platform.
func TestStopSignalRejectsEmptyOptions(t *testing.T) {
	require.Panics(t, func() { _ = WithStopSignalCloseSignals() })
}
