/**
 * View objects — one Catalog protocol at any source count.
 * Single-source is a ViewGeneration with one member, not a second mode.
 * ViewReadVersion is a contract skeleton: Generation is implemented;
 * append cuts and authorizationDecisionRef are typed but Reader does not assemble them.
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
