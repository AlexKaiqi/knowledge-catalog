package opensearchimpl

// completedPhysicalError is used only after a definitive provider rejection
// or a fully decoded operation result. A transport timeout, truncated response
// or intermediary 5xx is not proof that admitted work has drained.
type completedPhysicalError struct{ cause error }

func (e *completedPhysicalError) Error() string { return e.cause.Error() }
func (e *completedPhysicalError) Unwrap() error { return e.cause }

func completedPhysical(err error) error {
	if err == nil {
		return nil
	}
	return &completedPhysicalError{cause: err}
}
