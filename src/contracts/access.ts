/**
 * Read results — every object result carries repository/commit/object
 * coordinates so trust is preserved end-to-end (K-12).
 */

import type { KnowledgeAddress } from "./address.ts";
import type { CommitIdentity, Digest, KnowledgeRef, ObjectIdentity, RepositoryIdentity } from "./identity.ts";
import type { ProvenanceEnvelope } from "./provenance.ts";

/**
 * Which aspects participate in a read or a search document.
 * Write units stay Address-granular; this is a read/index strategy (docs/ASPECT_ACCESS.md).
 */
export interface AspectSelector {
  readonly include?: readonly string[];
  readonly exclude?: readonly string[];
}

/** Drop or keep assembled aspect keys. Entity blobs (no units) are unchanged. */
export function selectAspects(
  value: unknown,
  units: readonly KnowledgeAddress[] | undefined,
  selector?: AspectSelector,
): unknown {
  if (!selector || !units?.some((u) => u.aspectName)) return value;
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  const include = selector.include;
  const exclude = new Set(selector.exclude ?? []);
  const out: Record<string, unknown> = {};
  for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
    if (include && !include.includes(key)) continue;
    if (exclude.has(key)) continue;
    out[key] = item;
  }
  return out;
}

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
  /** Set when the object is stored as Aspect/Member units. Used to apply AspectSelector. */
  readonly units?: readonly KnowledgeAddress[];
}

/** One step in object history (LOG). Same digest+status collapse to the introducing commit. */
export interface ObjectRevision {
  readonly commit: CommitIdentity;
  readonly status: ResolutionStatus;
  readonly digest?: Digest;
}

/** DIFF of one object between two pinned commits. Missing side means not readable there. */
export interface ObjectDiff {
  readonly objectId: ObjectIdentity;
  readonly fromCommit: CommitIdentity;
  readonly toCommit: CommitIdentity;
  readonly from?: KnowledgeValue;
  readonly to?: KnowledgeValue;
}

/**
 * GET_PROVENANCE result. `chain` is the envelopes attached to this object's
 * units at `commit` — not a walked evidence graph and not git log.
 */
export interface ProvenanceTrace {
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly objectId: ObjectIdentity;
  readonly chain: readonly ProvenanceEnvelope[];
}

/** One durable record already appended to a stream. */
export interface StreamRecord {
  readonly recordId: string;
  readonly eventId: string;
  readonly payload: unknown;
  readonly digest: string;
  readonly recordedAt: string;
  readonly schemaRef?: string;
}

/** READ_STREAM result: current cursor plus records from the start of the stream. */
export interface StreamSlice {
  readonly repository: RepositoryIdentity;
  readonly streamRef: string;
  readonly cursor: string;
  readonly records: readonly StreamRecord[];
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
