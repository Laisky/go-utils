package storage

import "github.com/Laisky/errors/v2"

// CommitOutcome describes a published transaction's known durability, not whether
// arbitrary application operations can safely be repeated.
type CommitOutcome uint8

const (
	// CommitOutcomeUncertain means publication happened but durability could not be
	// confirmed. A crash may restore the preceding or the complete new generation.
	CommitOutcomeUncertain CommitOutcome = iota
	// CommitOutcomeDurable means all mutations committed durably; only subsequent
	// housekeeping or acknowledgement failed. Repeating a non-idempotent operation
	// would apply it again and must not be treated as failure recovery.
	CommitOutcomeDurable
)

// CommitError reports an error after a complete transaction was published. Err
// retains the original diagnostic. Outcome distinguishes confirmed durability
// from uncertainty; neither value is an instruction to retry the callback.
// Backends unable to classify a lost acknowledgement may return ordinary errors.
// Absence of CommitError therefore does not prove a transaction was rolled back.
type CommitError struct {
	Outcome CommitOutcome
	Err     error
}

// Error describes the published transaction's outcome and original failure.
func (err *CommitError) Error() string {
	prefix := "transaction published; durability uncertain"
	if err.Outcome == CommitOutcomeDurable {
		prefix = "transaction committed durably; post-commit failure"
	}
	if err.Err == nil {
		return prefix
	}
	return prefix + ": " + err.Err.Error()
}

// Unwrap exposes the original error for errors.Is and errors.As.
func (err *CommitError) Unwrap() error { return err.Err }

// NewCommitError annotates a non-nil post-publication diagnostic with its outcome.
// It returns nil for a nil error and preserves the underlying failure for callers.
func NewCommitError(err error, outcome CommitOutcome) error {
	if err == nil {
		return nil
	}
	return errors.WithStack(&CommitError{Outcome: outcome, Err: err})
}

// PublishedCommit extracts a classified outcome through wrapped errors. False
// means no classification is available, not that the operation was rolled back.
func PublishedCommit(err error) (CommitOutcome, bool) {
	var commit *CommitError
	if !errors.As(err, &commit) {
		return CommitOutcomeUncertain, false
	}
	return commit.Outcome, true
}
