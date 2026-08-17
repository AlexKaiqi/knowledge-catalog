/**
 * View objects — DEFERRED (D-tier). Single-person profile collapses these to
 * "current commit = version coordinate". Kept as minimal types so the
 * multi-repo profile can grow here without changing identity/surface/access.
 */

import type { CommitIdentity, RepositoryIdentity } from "./identity.ts";

/** `{RepositoryIdentity → CommitIdentity}` immutable union snapshot. */
export interface ViewGeneration {
  readonly generationId: string;
  readonly repositories: Readonly<Record<RepositoryIdentity, CommitIdentity>>;
}

export interface ViewReadVersion {
  readonly viewGeneration: ViewGeneration;
  readonly appendCuts?: Readonly<Record<string, string>>;
  readonly authorizationDecisionRef?: string;
}
