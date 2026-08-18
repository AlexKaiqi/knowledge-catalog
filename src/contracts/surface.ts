/**
 * Write Surfaces — the write boundary. A producer must choose one Surface;
 * the Writer executes it mechanically (semantically thin, strong execution).
 *
 * Minimal semantic layer: COMMIT (git-native) + APPEND (new). PROPOSAL is
 * deferred (single-person profile uses git branch).
 */

import type { KnowledgeAddress } from "./address.ts";
import type { KnowledgeRef, Digest, RepositoryIdentity, CommitIdentity } from "./identity.ts";
import type { ProvenanceEnvelope } from "./provenance.ts";

export type Surface = "COMMIT" | "PROPOSAL" | "APPEND";

export type OpKind = "PUT" | "REMOVE";

export type PreconditionType = "IF_ABSENT" | "IF_OBJECT_EQUALS" | "IF_DIGEST_EQUALS";

export interface Precondition {
  readonly type: PreconditionType;
  /** Expected current digest; for IF_OBJECT_EQUALS / IF_DIGEST_EQUALS. */
  readonly digest?: Digest;
}

export interface PutOp {
  readonly op: "PUT";
  readonly address: KnowledgeAddress;
  readonly value: unknown;
  /** Display/checkout location; movable without changing object identity. */
  readonly pathHint?: string;
  readonly schemaRef?: string;
  readonly precondition?: Precondition;
}

export interface RemoveOp {
  readonly op: "REMOVE";
  readonly address: KnowledgeAddress;
  readonly precondition?: Precondition;
  readonly reason?: string;
  readonly replacement?: KnowledgeRef;
}

export type Operation = PutOp | RemoveOp;

/**
 * A COMMIT is a single atomic ChangeSet against ONE target repository.
 * Invariants: K-01 (unique target), K-06 (expected target commit / CAS),
 * K-22 (no cross-repo transaction).
 */
export interface CommitChangeSet {
  readonly targetRepository: RepositoryIdentity;
  readonly targetRef: string;
  readonly baseCommit: CommitIdentity;
  readonly expectedTargetCommit: CommitIdentity;
  readonly operations: readonly Operation[];
  readonly message?: string;
  readonly provenance?: ProvenanceEnvelope;
}

export interface AppendEntry {
  readonly eventId: string;
  readonly eventType?: string;
  readonly payload: unknown;
  readonly observedAt?: string;
  readonly schemaRef?: string;
}

export interface AppendEntries {
  readonly targetRepository: RepositoryIdentity;
  readonly streamRef: string;
  readonly expectedCursor?: string;
  readonly entries: readonly AppendEntry[];
}
