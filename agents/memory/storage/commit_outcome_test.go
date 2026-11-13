package storage

import (
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestCommitOutcomeClassification preserves cause identity and distinguishes
// durable commits from uncertainty without treating ordinary errors as aborts.
func TestCommitOutcomeClassification(t *testing.T) {
	cause := errors.New("synthetic post-publication failure")
	for _, outcome := range []CommitOutcome{CommitOutcomeUncertain, CommitOutcomeDurable} {
		err := errors.Wrap(NewCommitError(cause, outcome), "outer")
		require.ErrorIs(t, err, cause)
		actual, ok := PublishedCommit(err)
		require.True(t, ok)
		require.Equal(t, outcome, actual)
		require.NotEmpty(t, err.Error())
	}
	for _, err := range []error{nil, cause} {
		_, ok := PublishedCommit(err)
		require.False(t, ok, "ordinary errors provide no commit outcome knowledge")
	}
	require.NoError(t, NewCommitError(nil, CommitOutcomeDurable))
	require.NotEmpty(t, (&CommitError{}).Error())
}
