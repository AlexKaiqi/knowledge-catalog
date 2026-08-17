/**
 * Catalog — the multi-repository combination boundary (O19 / whitepaper §17A).
 * It registers independent repositories, resolves ViewDefinition into an
 * immutable ViewGeneration, and moves channels. It NEVER writes repository
 * content (K-22, no cross-repo transaction; Promotion is a separate CAS).
 *
 * Invariants enforced:
 *  - K-10: a repository appears at most once per generation.
 *  - G2: generation_id is a deterministic content hash (idempotent, replayable).
 *  - K-12/K-13: federated reads preserve source repo/commit and never override.
 */

import { createHash } from "node:crypto";
import type {
  CommitIdentity,
  ObjectIdentity,
  RepositoryIdentity,
  ViewDefinition,
  ViewGeneration,
} from "../contracts/index.ts";
import { IngressError } from "../contracts/errors.ts";
import type { Store } from "../store.ts";

export interface FederatedValue {
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly objectId: ObjectIdentity;
  readonly value: unknown;
}

function computeGenerationId(revision: number, repositories: Readonly<Record<RepositoryIdentity, CommitIdentity>>): string {
  const sorted = Object.keys(repositories)
    .sort()
    .map((k) => `${k}=${repositories[k]}`)
    .join(",");
  return createHash("sha256").update(`${revision}:${sorted}`).digest("hex");
}

export class Catalog {
  private readonly channels = new Map<string, string>();

  constructor(private readonly store: Store) {}

  defineView(viewId: string, revision: number, sources: ViewDefinition["sources"]): ViewDefinition {
    return { viewId, revision, sources };
  }

  /** RESOLVE_VIEW: resolve symbolic selectors to exact commits; deterministic id (G2). */
  resolveView(def: ViewDefinition): ViewGeneration {
    const seen = new Set<RepositoryIdentity>();
    const repositories: Record<RepositoryIdentity, CommitIdentity> = {};
    for (const src of def.sources) {
      if (seen.has(src.repository)) {
        throw new IngressError("VIEW_GENERATION_INVALID", `repository ${src.repository} appears twice`);
      }
      seen.add(src.repository);
      const repo = this.store.repos.get(src.repository);
      if (!repo) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown repository ${src.repository}`);
      repositories[src.repository] = repo.head(src.selector);
    }
    return {
      generationId: computeGenerationId(def.revision, repositories),
      repositories,
    };
  }

  /** Federated read: return EVERY source that holds the object (K-12/K-13, no override). */
  readObject(generation: ViewGeneration, objectId: ObjectIdentity): FederatedValue[] {
    const out: FederatedValue[] = [];
    for (const [repositoryId, commit] of Object.entries(generation.repositories)) {
      const repo = this.store.repos.get(repositoryId);
      if (!repo) continue;
      try {
        out.push({ repository: repositoryId, commit, objectId, value: repo.read(objectId, commit).value });
      } catch {
        // object absent in this repo: skip, do not override other sources
      }
    }
    return out;
  }

  /** PROMOTE_GENERATION: CAS move a channel (Catalog pointer only, never a repo). */
  promote(channel: string, expected: string | undefined, newGenerationId: string): void {
    const current = this.channels.get(channel);
    if (current !== expected) {
      throw new IngressError("PROMOTION_CAS_FAILED", `expected ${expected} but channel is ${current}`);
    }
    this.channels.set(channel, newGenerationId);
  }

  /** ROLLBACK_PROMOTION: CAS move a channel back to a prior generation. */
  rollback(channel: string, expected: string, prior: string): void {
    this.promote(channel, expected, prior);
  }

  channel(channel: string): string | undefined {
    return this.channels.get(channel);
  }
}
