/**
 * Identity & References — the first of the two things git does NOT give us.
 *
 * Invariants (K-04, ADR-008):
 *  - ObjectIdentity is independent of path; path is only a `pathHint`.
 *  - KnowledgeRef = RepositoryIdentity + ObjectIdentity (no path).
 *  - PinnedKnowledgeRef adds CommitIdentity (reproducible).
 *  - FileRef adds Path + optional Digest (locate raw file only).
 */

/** `kr://<org>/<scope>/<name>` — stable global repository identity. */
export type RepositoryIdentity = string;

/** Stable object identity within one repository; independent of path. */
export type ObjectIdentity = string;

/** Immutable commit identity (content-addressed hash). */
export type CommitIdentity = string;

/** Content digest (sha256) of a value. */
export type Digest = string;

/** `kc://<repo>/<object-id>` — long-term, path-independent reference. */
export interface KnowledgeRef {
  readonly repository: RepositoryIdentity;
  readonly object: ObjectIdentity;
}

/** `kc://<repo>@<commit>/<object-id>` — reproducible, pinned reference. */
export interface PinnedKnowledgeRef extends KnowledgeRef {
  readonly commit: CommitIdentity;
}

/** `file://<repo>@<commit>/<path>#<digest>` — raw file reference only. */
export interface FileRef {
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly path: string;
  readonly digest?: Digest;
}

/** Formatting helpers (kc:// / kr:// syntax per refinements-p0 P0-3). */
export function knowledgeRef(repository: RepositoryIdentity, object: ObjectIdentity): string {
  return `kc://${repository}/${object}`;
}

export function pinnedKnowledgeRef(
  repository: RepositoryIdentity,
  commit: CommitIdentity,
  object: ObjectIdentity,
): string {
  return `kc://${repository}@${commit}/${object}`;
}
