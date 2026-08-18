/**
 * Catalog — the multi-repository combination boundary. It owns immutable
 * ViewGenerations and serving channels, but never repository content.
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

function computeGenerationId(
  revision: number,
  repositories: Readonly<Record<RepositoryIdentity, CommitIdentity>>,
): string {
  const sorted = Object.keys(repositories)
    .sort()
    .map((key) => `${key}=${repositories[key]}`)
    .join(",");
  return createHash("sha256").update(`${revision}:${sorted}`).digest("hex");
}

export class Catalog {
  private readonly generations = new Map<string, ViewGeneration>();
  private readonly channels = new Map<string, string>();

  constructor(private readonly store: Store) {}

  defineView(viewId: string, revision: number, sources: ViewDefinition["sources"]): ViewDefinition {
    return { viewId, revision, sources };
  }

  /** RESOLVE_VIEW: resolve every selector once and register the exact generation. */
  resolveView(def: ViewDefinition): ViewGeneration {
    if (def.sources.length === 0) {
      throw new IngressError("VIEW_GENERATION_INVALID", "a view must contain at least one repository");
    }

    const seen = new Set<RepositoryIdentity>();
    const repositories: Record<RepositoryIdentity, CommitIdentity> = {};
    for (const src of def.sources) {
      if (seen.has(src.repository)) {
        throw new IngressError("VIEW_GENERATION_INVALID", `repository ${src.repository} appears twice`);
      }
      seen.add(src.repository);
      const repo = this.store.repos.get(src.repository);
      if (!repo) {
        throw new IngressError("VIEW_GENERATION_INVALID", `unknown repository ${src.repository}`);
      }
      const commit = repo.getRef(src.selector);
      if (!commit) {
        throw new IngressError("VIEW_GENERATION_INVALID", `selector ${src.selector} is unresolved in ${src.repository}`);
      }
      repositories[src.repository] = commit;
    }
    return this.registerGeneration(def.revision, repositories);
  }

  /** CREATE_PREVIEW: replace explicit members and retain every other base member. */
  createPreview(
    baseGenerationId: string,
    substitutions: Readonly<Record<RepositoryIdentity, CommitIdentity>>,
  ): ViewGeneration {
    const base = this.generation(baseGenerationId);
    const repositories = { ...base.repositories };

    for (const [repositoryId, commit] of Object.entries(substitutions)) {
      if (!(repositoryId in repositories)) {
        throw new IngressError("VIEW_GENERATION_INVALID", `repository ${repositoryId} is not in the base generation`);
      }
      const repo = this.store.repos.get(repositoryId);
      if (!repo || !repo.hasCommit(commit)) {
        throw new IngressError("VERSION_UNRESOLVED", `commit ${commit} is unresolved in ${repositoryId}`);
      }
      repositories[repositoryId] = commit;
    }
    return this.registerGeneration(base.definitionRevision, repositories);
  }

  generation(generationId: string): ViewGeneration {
    const generation = this.generations.get(generationId);
    if (!generation) {
      throw new IngressError("VIEW_GENERATION_INVALID", `unknown generation ${generationId}`);
    }
    return generation;
  }

  /** Federated read skips only an absent object; integrity and backend errors propagate. */
  readObject(generation: ViewGeneration, objectId: ObjectIdentity): FederatedValue[] {
    const registered = this.generation(generation.generationId);
    const out: FederatedValue[] = [];
    for (const [repositoryId, commit] of Object.entries(registered.repositories)) {
      const repo = this.store.repos.get(repositoryId);
      if (!repo) {
        throw new IngressError("TEMPORARY_UNAVAILABLE", `repository ${repositoryId} is not mounted`);
      }
      try {
        out.push({ repository: repositoryId, commit, objectId, value: repo.read(objectId, commit).value });
      } catch (error) {
        if (error instanceof IngressError && error.code === "KNOWLEDGE_REF_UNRESOLVED") continue;
        throw error;
      }
    }
    return out;
  }

  /** PROMOTE_GENERATION: CAS move a channel to an existing immutable generation. */
  promote(channel: string, expected: string | undefined, newGenerationId: string): void {
    this.generation(newGenerationId);
    const current = this.channels.get(channel);
    if (current !== expected) {
      throw new IngressError("PROMOTION_CAS_FAILED", `expected ${expected} but channel is ${current}`);
    }
    this.channels.set(channel, newGenerationId);
  }

  rollback(channel: string, expected: string, prior: string): void {
    this.promote(channel, expected, prior);
  }

  channel(channel: string): string | undefined {
    return this.channels.get(channel);
  }

  private registerGeneration(
    definitionRevision: number,
    repositories: Readonly<Record<RepositoryIdentity, CommitIdentity>>,
  ): ViewGeneration {
    const immutableRepositories = Object.freeze({ ...repositories });
    const generationId = computeGenerationId(definitionRevision, immutableRepositories);
    const existing = this.generations.get(generationId);
    if (existing) return existing;

    const generation = Object.freeze({
      generationId,
      definitionRevision,
      repositories: immutableRepositories,
    });
    this.generations.set(generationId, generation);
    return generation;
  }
}
