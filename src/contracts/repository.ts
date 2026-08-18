/**
 * Repository — the unified store contract. ONE semantic layer, MANY store
 * implementations (git today; Dolt/PostgreSQL by scale later). The protocol
 * layer (Ingress/Access/ControlPlane/Catalog) depends ONLY on this interface,
 * never on a concrete adapter (K-23: logic/physical separation).
 */

import type { AppendEntry, CommitChangeSet } from "./surface.ts";
import type { CommitIdentity, ObjectIdentity, RepositoryIdentity } from "./identity.ts";
import type { KnowledgeValue, ProvenanceTrace, Resolution } from "./access.ts";

export interface Repository {
  readonly repositoryId: RepositoryIdentity;

  /** Current commit of a ref (default HEAD). */
  head(ref?: string): CommitIdentity;

  /** Read a ref's commit (undefined if the ref does not exist). */
  getRef(ref: string): CommitIdentity | undefined;

  /** Check that an immutable version exists in this repository. */
  hasCommit(commitId: CommitIdentity): boolean;

  /** Create a ref (branch/tag) at an exact commit; no silent overwrite. */
  createRef(ref: string, commitId: CommitIdentity): void;

  /** CAS fast-forward a target ref to a candidate commit (K-06). */
  merge(targetRef: string, candidateCommit: CommitIdentity, expectedTargetCommit: CommitIdentity): CommitIdentity;

  /** Apply a COMMIT ChangeSet atomically; returns the new commit id. */
  applyCommit(cs: CommitChangeSet): CommitIdentity;

  /** Append entries to a stream (append-only; event-id idempotent); returns record ids. */
  append(streamRef: string, entries: readonly AppendEntry[], expectedCursor?: string): readonly string[];

  /** Current cursor (record count) of a stream. */
  streamCursor(streamRef: string): string;

  resolve(objectId: ObjectIdentity, commitId: CommitIdentity): Resolution;
  read(objectId: ObjectIdentity, commitId: CommitIdentity): KnowledgeValue;
  origin(objectId: ObjectIdentity, commitId: CommitIdentity): ProvenanceTrace;
  search(query: string, commitId: CommitIdentity): KnowledgeValue[];
  list(commitId: CommitIdentity): KnowledgeValue[];
}
