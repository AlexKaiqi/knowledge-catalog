package storecontract

import "errors"

// WriteNotApplied records a proven failure before authoritative publication.
// Unmarked failures have an unknown outcome and must retain their command claim.
func WriteNotApplied(err error) error {
	if err == nil || IsWriteNotApplied(err) {
		return err
	}
	return &writeNotAppliedError{err: err}
}

type writeNotAppliedError struct{ err error }

func (e *writeNotAppliedError) Error() string { return e.err.Error() }
func (e *writeNotAppliedError) Unwrap() error { return e.err }

func IsWriteNotApplied(err error) bool {
	var outcome *writeNotAppliedError
	return errors.As(err, &outcome)
}
