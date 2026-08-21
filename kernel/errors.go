package kernel

import "fmt"

type ErrorCode string

const (
	ErrPreconditionFailed       ErrorCode = "PRECONDITION_FAILED"
	ErrNonFastForward           ErrorCode = "NON_FAST_FORWARD"
	ErrObjectIDConflict         ErrorCode = "OBJECT_ID_CONFLICT"
	ErrIdempotencyConflict      ErrorCode = "IDEMPOTENCY_CONFLICT"
	ErrEventIDConflict          ErrorCode = "EVENT_ID_CONFLICT"
	ErrPositionRegression       ErrorCode = "POSITION_REGRESSION"
	ErrWriteTargetRequired      ErrorCode = "WRITE_TARGET_REQUIRED"
	ErrSurfaceMismatch          ErrorCode = "SURFACE_MISMATCH"
	ErrScopeDenied              ErrorCode = "SCOPE_DENIED"
	ErrSchemaUnsupported        ErrorCode = "SCHEMA_UNSUPPORTED"
	ErrSchemaRevisionUnresolved ErrorCode = "SCHEMA_REVISION_UNRESOLVED"
	ErrTargetRepositoryDenied   ErrorCode = "TARGET_REPOSITORY_DENIED"
	ErrKnowledgeRefUnresolved   ErrorCode = "KNOWLEDGE_REF_UNRESOLVED"
	ErrVersionUnresolved        ErrorCode = "VERSION_UNRESOLVED"
	ErrCapabilityUnsatisfied    ErrorCode = "CAPABILITY_UNSATISFIED"
	ErrTemporaryUnavailable     ErrorCode = "TEMPORARY_UNAVAILABLE"
	ErrCandidateMoved           ErrorCode = "CANDIDATE_MOVED"
	ErrValidationBasisMismatch  ErrorCode = "VALIDATION_BASIS_MISMATCH"
	ErrPromotionCASFailed       ErrorCode = "PROMOTION_CAS_FAILED"
	ErrViewGenerationInvalid    ErrorCode = "VIEW_GENERATION_INVALID"
	ErrForbidden                ErrorCode = "FORBIDDEN"
	ErrCatalogArchived          ErrorCode = "CATALOG_ARCHIVED"
	ErrRepositoryArchived       ErrorCode = "REPOSITORY_ARCHIVED"
	ErrGateUnsatisfied          ErrorCode = "GATE_UNSATISFIED"
	ErrHookDenied               ErrorCode = "HOOK_DENIED"
)

// IngressError is the protocol error. Callers switch on Code, not strings.
type IngressError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *IngressError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func Fail(code ErrorCode, format string, args ...any) error {
	return &IngressError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func AsIngress(err error) *IngressError {
	if err == nil {
		return nil
	}
	if e, ok := err.(*IngressError); ok {
		return e
	}
	return nil
}

func CodeOf(err error) ErrorCode {
	if e := AsIngress(err); e != nil {
		return e.Code
	}
	return ""
}
