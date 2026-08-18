/**
 * Structured errors — minimal Writer/Reader error model (whitepaper §8.3,
 * plus refinements-p1 additions).
 */

export type ErrorCode =
  | "PRECONDITION_FAILED"
  | "NON_FAST_FORWARD"
  | "OBJECT_ID_CONFLICT"
  | "IDEMPOTENCY_CONFLICT"
  | "EVENT_ID_CONFLICT"
  | "POSITION_REGRESSION"
  | "WRITE_TARGET_REQUIRED"
  | "SURFACE_MISMATCH"
  | "SCOPE_DENIED"
  | "SCHEMA_UNSUPPORTED"
  | "SCHEMA_REVISION_UNRESOLVED"
  | "TARGET_REPOSITORY_DENIED"
  | "KNOWLEDGE_REF_UNRESOLVED"
  | "VERSION_UNRESOLVED"
  | "CAPABILITY_UNSATISFIED"
  | "TEMPORARY_UNAVAILABLE"
  | "CANDIDATE_MOVED"
  | "VALIDATION_BASIS_MISMATCH"
  | "PROMOTION_CAS_FAILED"
  | "VIEW_GENERATION_INVALID";

export class IngressError extends Error {
  readonly code: ErrorCode;

  constructor(code: ErrorCode, message: string) {
    super(message);
    this.name = "IngressError";
    this.code = code;
  }
}
