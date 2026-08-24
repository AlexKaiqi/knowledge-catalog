package kernel

import "fmt"

// ErrorCode is the stable switch key. Callers branch on Code, not Message.
// Pick the code from the recovery class; Message names the subject, the failed
// condition, and expected vs actual when both exist.
type ErrorCode string

const (
	// ErrUsageInvalid: request/flag/home shape. Missing flags, unknown command,
	// mutually exclusive flags, empty changeset, search/stream face shape,
	// unmounted repo/stream in this process, bare fmt.Errorf after Normalize.
	ErrUsageInvalid ErrorCode = "USAGE_INVALID"

	// ErrPreconditionFailed: object, digest, cursor, or worktree does not match
	// the current HEAD. Caller should READ/DIFF and rebuild. Not missing flags.
	ErrPreconditionFailed ErrorCode = "PRECONDITION_FAILED"

	// ErrNonFastForward: expectedTargetCommit (or merge expected) is not the current ref tip.
	ErrNonFastForward ErrorCode = "NON_FAST_FORWARD"

	// ErrObjectIDConflict: duplicate Address, or entity blob mixed with aspects.
	ErrObjectIDConflict ErrorCode = "OBJECT_ID_CONFLICT"

	// ErrIdempotencyConflict: same command_id reused with a different digest.
	ErrIdempotencyConflict ErrorCode = "IDEMPOTENCY_CONFLICT"

	// ErrEventIDConflict: same eventId reused with a different payload.
	ErrEventIDConflict ErrorCode = "EVENT_ID_CONFLICT"

	// ErrPositionRegression: APPEND producer position moved backwards.
	ErrPositionRegression ErrorCode = "POSITION_REGRESSION"

	// ErrWriteTargetRequired: write did not name a unique repository or ref.
	// Empty operations are ErrUsageInvalid, not this code.
	ErrWriteTargetRequired ErrorCode = "WRITE_TARGET_REQUIRED"

	// ErrSurfaceMismatch: Surface does not match the address. Frozen until thrown.
	ErrSurfaceMismatch ErrorCode = "SURFACE_MISMATCH"

	// ErrScopeDenied: connector Desired is outside Scope.
	ErrScopeDenied ErrorCode = "SCOPE_DENIED"

	// ErrSchemaUnsupported: schema shape is not allowed. Frozen until thrown.
	ErrSchemaUnsupported ErrorCode = "SCHEMA_UNSUPPORTED"

	// ErrSchemaRevisionUnresolved: schema_ref is not a resolvable schema/* in the target repo.
	ErrSchemaRevisionUnresolved ErrorCode = "SCHEMA_REVISION_UNRESOLVED"

	// ErrTargetRepositoryDenied: write target is not this Snapshot, or Catalog id used as --repo.
	ErrTargetRepositoryDenied ErrorCode = "TARGET_REPOSITORY_DENIED"

	// ErrKnowledgeRefUnresolved: version exists; object or event is missing there.
	ErrKnowledgeRefUnresolved ErrorCode = "KNOWLEDGE_REF_UNRESOLVED"

	// ErrVersionUnresolved: commit or ref coordinate does not exist.
	ErrVersionUnresolved ErrorCode = "VERSION_UNRESOLVED"

	// ErrCapabilityUnsatisfied: adapter/index cannot satisfy an already-declared capability.
	ErrCapabilityUnsatisfied ErrorCode = "CAPABILITY_UNSATISFIED"

	// ErrTemporaryUnavailable: transient backend I/O. Same request is safe to retry.
	// Unmounted repos/streams are ErrUsageInvalid.
	ErrTemporaryUnavailable ErrorCode = "TEMPORARY_UNAVAILABLE"

	// ErrCandidateMoved: candidate advanced after validation.
	ErrCandidateMoved ErrorCode = "CANDIDATE_MOVED"

	// ErrValidationBasisMismatch: PASSED report is not bound to this exact Preview.
	ErrValidationBasisMismatch ErrorCode = "VALIDATION_BASIS_MISMATCH"

	// ErrWorkspaceInvalid: Workspace recipe cannot be used (unknown, retired,
	// duplicate source, empty sources, selector has no such ref). Unmounted
	// members are ErrUsageInvalid.
	ErrWorkspaceInvalid ErrorCode = "WORKSPACE_INVALID"

	// ErrForbidden: --as did not hit allow.json.
	ErrForbidden ErrorCode = "FORBIDDEN"

	// ErrUnauthenticated: the HTTP facade could not establish a trusted
	// principal. Authorization is evaluated only after authentication succeeds.
	ErrUnauthenticated ErrorCode = "UNAUTHENTICATED"

	// ErrCatalogArchived: catalog is archived; define-workspace / register are closed.
	ErrCatalogArchived ErrorCode = "CATALOG_ARCHIVED"

	// ErrRepositoryArchived: repository is archived; COMMIT/APPEND/PROPOSE are closed.
	ErrRepositoryArchived ErrorCode = "REPOSITORY_ARCHIVED"

	// ErrGateUnsatisfied: merge evidence checklist is not PASSED on this basis.
	ErrGateUnsatisfied ErrorCode = "GATE_UNSATISFIED"

	// ErrHookDenied: outbound pre hook exited non-zero, timed out, or HTTP denied.
	ErrHookDenied ErrorCode = "HOOK_DENIED"
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

// Normalize turns any error into an IngressError. Bare fmt.Errorf becomes USAGE_INVALID
// (request/flag/home shape). Write CAS stays PRECONDITION_FAILED via Fail.
func Normalize(err error) *IngressError {
	if err == nil {
		return nil
	}
	if e := AsIngress(err); e != nil {
		return e
	}
	return &IngressError{Code: ErrUsageInvalid, Message: err.Error()}
}

// FaultJSON is the single error envelope: {"error":{"code","message"}}.
func FaultJSON(err error) map[string]any {
	return map[string]any{"error": Normalize(err)}
}
