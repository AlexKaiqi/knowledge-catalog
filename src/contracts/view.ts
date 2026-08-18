/**
 * View objects — DEFERRED (D-tier). Single-person profile collapses these to
 * "current commit = version coordinate". Kept as minimal types so the
 * multi-repo profile can grow here without changing identity/surface/access.
 */

import type { CommitIdentity, RepositoryIdentity } from "./identity.ts";

/** A source in a ViewDefinition: one repository + a symbolic selector (branch/release). */
export interface ViewSource {
  readonly repository: RepositoryIdentity;
  readonly selector: string; // "refs/heads/main" | "release/2026.08"
}

/** Mutable intent: which repos to join. Resolve to an immutable ViewGeneration. */
export interface ViewDefinition {
  readonly viewId: string;
  readonly revision: number;
  readonly sources: readonly ViewSource[];
}

/** `{RepositoryIdentity → CommitIdentity}` immutable union snapshot. */
export interface ViewGeneration {
  readonly generationId: string;
  readonly definitionRevision: number;
  readonly repositories: Readonly<Record<RepositoryIdentity, CommitIdentity>>;
}

export interface ViewReadVersion {
  readonly viewGeneration: ViewGeneration;
  readonly appendCuts?: Readonly<Record<string, string>>;
  readonly authorizationDecisionRef?: string;
}
