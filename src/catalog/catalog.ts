/**
 * Catalog — the multi-repository combination boundary. It owns view recipes,
 * immutable ViewGenerations and named Releases, but never repository content.
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
import type { CatalogRegistry } from "./registry.ts";

export interface FederatedValue {
  readonly repository: RepositoryIdentity;
  readonly commit: CommitIdentity;
  readonly objectId: ObjectIdentity;
  readonly value: unknown;
}

/** Persistable Catalog registry (views + generations + releases). */
export interface CatalogState {
  readonly views: readonly ViewDefinition[];
  readonly generations: readonly ViewGeneration[];
  readonly releases: Readonly<Record<string, string>>;
}

export interface GenerationIssue {
  readonly repository: RepositoryIdentity;
  readonly code: "TEMPORARY_UNAVAILABLE" | "VERSION_UNRESOLVED";
  readonly message: string;
}

export interface GenerationCheck {
  readonly generationId: string;
  readonly outcome: "PASSED" | "FAILED";
  readonly issues: readonly GenerationIssue[];
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
  private readonly views = new Map<string, ViewDefinition>();
  private readonly generations = new Map<string, ViewGeneration>();
  private readonly releases = new Map<string, string>();

  constructor(
    private readonly store: Store,
    private readonly registry?: CatalogRegistry,
  ) {
    if (registry) this.loadState(registry.load());
  }

  dumpState(): CatalogState {
    return {
      views: [...this.views.values()],
      generations: [...this.generations.values()],
      releases: Object.fromEntries(this.releases),
    };
  }

  loadState(state: CatalogState): void {
    this.views.clear();
    this.generations.clear();
    this.releases.clear();
    for (const view of state.views ?? []) {
      this.views.set(view.viewId, view);
    }
    for (const generation of state.generations) {
      const frozen = Object.freeze({
        generationId: generation.generationId,
        definitionRevision: generation.definitionRevision,
        repositories: Object.freeze({ ...generation.repositories }),
      });
      this.generations.set(frozen.generationId, frozen);
    }
    for (const [name, generationId] of Object.entries(state.releases)) {
      this.releases.set(name, generationId);
    }
  }

  private persist(message?: string): void {
    this.registry?.save(this.dumpState(), message);
  }

  defineView(viewId: string, revision: number, sources: ViewDefinition["sources"]): ViewDefinition {
    const def: ViewDefinition = { viewId, revision, sources };
    this.views.set(viewId, def);
    this.persist(`define-view ${viewId}`);
    return def;
  }

  view(viewId: string): ViewDefinition {
    const def = this.views.get(viewId);
    if (!def) throw new IngressError("VIEW_GENERATION_INVALID", `unknown view ${viewId}`);
    return def;
  }

  /** PIN_VIEW: resolve every selector once and register the exact generation. */
  pinView(input: ViewDefinition | string): ViewGeneration {
    const def = typeof input === "string" ? this.view(input) : input;
    if (typeof input !== "string") {
      this.views.set(def.viewId, def);
    }
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
    const generation = this.registerGeneration(def.revision, repositories);
    this.persist(`pin-view ${def.viewId}`);
    return generation;
  }

  /**
   * Pin the named view at this moment, then CAS-move the Release.
   * `expected` defaults to the current pointer (undefined on first publish).
   * pin is idempotent; only the Release pointer is the serving CAS.
   */
  publish(release: string, viewId: string, expected?: string): {
    readonly release: string;
    readonly generationId: string;
    readonly generation: ViewGeneration;
  } {
    const generation = this.pinView(viewId);
    this.promote(release, expected ?? this.releases.get(release), generation.generationId);
    return { release, generationId: generation.generationId, generation };
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
    const generation = this.registerGeneration(base.definitionRevision, repositories);
    this.persist(`create-preview ${generation.generationId}`);
    return generation;
  }

  generation(generationId: string): ViewGeneration {
    const generation = this.generations.get(generationId);
    if (!generation) {
      throw new IngressError("VIEW_GENERATION_INVALID", `unknown generation ${generationId}`);
    }
    return generation;
  }

  /** FEDERATED_READ: skip only an absent object; integrity and backend errors propagate. */
  federatedRead(generation: ViewGeneration, objectId: ObjectIdentity): FederatedValue[] {
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

  /** PROMOTE: CAS move a Release name to an existing immutable generation. */
  promote(release: string, expected: string | undefined, newGenerationId: string): void {
    this.generation(newGenerationId);
    const current = this.releases.get(release);
    if (current !== expected) {
      throw new IngressError("PROMOTION_CAS_FAILED", `expected ${expected} but release is ${current}`);
    }
    this.releases.set(release, newGenerationId);
    this.persist(`promote ${release} -> ${newGenerationId.slice(0, 12)}`);
  }

  rollback(release: string, expected: string, prior: string): void {
    this.promote(release, expected, prior);
  }

  /** Structural check: every pinned member repo is mounted and the commit exists. */
  checkGeneration(generationId: string): GenerationCheck {
    const generation = this.generation(generationId);
    const issues: GenerationIssue[] = [];
    for (const [repositoryId, commit] of Object.entries(generation.repositories)) {
      const repo = this.store.repos.get(repositoryId);
      if (!repo) {
        issues.push({
          repository: repositoryId,
          code: "TEMPORARY_UNAVAILABLE",
          message: `${repositoryId} is not mounted`,
        });
        continue;
      }
      if (!repo.hasCommit(commit)) {
        issues.push({
          repository: repositoryId,
          code: "VERSION_UNRESOLVED",
          message: `commit ${commit} is unresolved in ${repositoryId}`,
        });
      }
    }
    return {
      generationId,
      outcome: issues.length === 0 ? "PASSED" : "FAILED",
      issues,
    };
  }

  release(name: string): string | undefined {
    return this.releases.get(name);
  }

  /** READ_RELEASE: federated read of an object on the generation currently named by the Release. */
  readRelease(name: string, objectId: ObjectIdentity): FederatedValue[] {
    const generationId = this.releases.get(name);
    if (!generationId) {
      throw new IngressError("VIEW_GENERATION_INVALID", `unknown release ${name}`);
    }
    return this.federatedRead(this.generation(generationId), objectId);
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
