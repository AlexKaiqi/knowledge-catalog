/**
 * Provenance — the second of the two things git does NOT give us.
 * git has commit author/message, but not "where did this knowledge come from,
 * what produced it". This is the minimal envelope collected by GET_PROVENANCE.
 */

export type OriginKind =
  | "SOURCE"
  | "OBSERVATION"
  | "EVIDENCE"
  | "ASSERTION"
  | "DEFINITION"
  | "DERIVATION";

export interface ProvenanceEnvelope {
  readonly originKind: OriginKind;
  /** Who. */
  readonly actorRef?: string;
  /** What activity. */
  readonly activityRef?: string;
  /** Sources. */
  readonly sourceRefs?: readonly string[];
  /** Supporting evidence. */
  readonly evidenceRefs?: readonly string[];
  /** Required for DERIVATION only. */
  readonly inputViewReadVersionRef?: string;
  /** Required for DERIVATION only. */
  readonly algorithm?: {
    readonly derivationSpecRef?: string;
    readonly modelRef?: string;
    readonly codeHash?: string;
  };
  readonly producedAt?: string;
}
