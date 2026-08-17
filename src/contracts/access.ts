/**
 * Access results — typed read results. Every result carries repository/commit/
 * object provenance so trust is preserved end-to-end (K-12).
 */

import type { KnowledgeAddress } from "./address.ts";
import type { CommitIdentity, Digest, KnowledgeRef, ObjectIdentity, RepositoryIdentity } from "./identity.ts";
import type { ProvenanceEnvelope } from "./provenance.ts";

export type ResolutionStatus = "RESOLVED" | "REMOVED" | "UNRESOLVED" | "FORBIDDEN";

export interface Resolution {
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly objectId: ObjectIdentity;
  readonly address: KnowledgeAddress;
  readonly pathHint: string;
  readonly digest?: Digest;
  readonly schemaRef?: string;
  readonly status: ResolutionStatus;
}

export interface KnowledgeValue {
  readonly knowledgeRef: KnowledgeRef;
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly address: KnowledgeAddress;
  readonly value: unknown;
  readonly provenance?: ProvenanceEnvelope;
}

export interface ProvenanceTrace {
  readonly value: unknown;
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly objectId: ObjectIdentity;
  /** Near-to-far chain of provenance envelopes. */
  readonly chain: readonly ProvenanceEnvelope[];
}

/**
 * GroundingCitation — the agreed projection that lets an AI's factual claims
 * carry a PinnedKnowledgeRef + provenance to the UI (ingestion-and-grounding B).
 */
export interface GroundingCitation {
  readonly knowledgeRef: KnowledgeRef;
  readonly pinnedRef: string;
  readonly digest?: Digest;
  readonly fragment?: string;
  readonly provenanceSummary?: {
    readonly actorRef?: string;
    readonly sourceRefs?: readonly string[];
    readonly originKind?: string;
  };
}
