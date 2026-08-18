/**
 * GitCatalogRegistry — Catalog views / generations / releases as FileGit objects.
 * Promote history is git log on release/<name>, not a sidecar JSON blob.
 */

import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { FileGitRepository } from "../adapters/file-git/repository.ts";
import { canonicalDigest } from "../digest.ts";
import type { Operation, ViewDefinition, ViewGeneration } from "../contracts/index.ts";
import { EMPTY_CATALOG_STATE, type CatalogRegistry } from "./registry.ts";
import type { CatalogState } from "./catalog.ts";

export const CATALOG_OBJECT = {
  view: (id: string) => `view/${id}`,
  generation: (id: string) => `generation/${id}`,
  release: (name: string) => `release/${name}`,
} as const;

export interface CatalogCommit {
  readonly commit: string;
  readonly message: string;
}

export class GitCatalogRegistry implements CatalogRegistry {
  readonly repo: FileGitRepository;

  constructor(rootDir: string, repositoryId: string) {
    this.repo = new FileGitRepository(rootDir, repositoryId);
  }

  load(): CatalogState {
    const head = this.repo.head("refs/heads/main");
    const listed = this.repo.list(head);
    if (!listed.length) return EMPTY_CATALOG_STATE;

    const views: ViewDefinition[] = [];
    const generations: ViewGeneration[] = [];
    const releases: Record<string, string> = {};
    for (const item of listed) {
      const id = item.address.objectId;
      if (id.startsWith("view/")) views.push(item.value as ViewDefinition);
      else if (id.startsWith("generation/")) generations.push(item.value as ViewGeneration);
      else if (id.startsWith("release/")) {
        const body = item.value as { name: string; generationId: string };
        releases[body.name] = body.generationId;
      }
    }
    return normalizeCatalogState({ views, generations, releases });
  }

  save(state: CatalogState, message = "catalog: persist"): void {
    const current = this.load();
    const next = normalizeCatalogState(state);
    if (canonicalDigest(current) === canonicalDigest(next)) return;
    state = next;

    const desired = new Map<string, unknown>();
    for (const view of state.views) desired.set(CATALOG_OBJECT.view(view.viewId), view);
    for (const generation of state.generations) {
      desired.set(CATALOG_OBJECT.generation(generation.generationId), generation);
    }
    for (const [name, generationId] of Object.entries(state.releases)) {
      desired.set(CATALOG_OBJECT.release(name), { name, generationId });
    }

    const head = this.repo.head("refs/heads/main");
    const existing = new Map(this.repo.list(head).map((item) => [item.address.objectId, item]));
    const operations: Operation[] = [];
    for (const [objectId, value] of desired) {
      const prev = existing.get(objectId);
      if (prev && canonicalDigest(prev.value) === canonicalDigest(value)) continue;
      operations.push({
        op: "PUT",
        address: { kind: "Entity", objectId },
        value,
        pathHint: `${objectId}.json`,
      });
    }
    for (const objectId of existing.keys()) {
      if (!desired.has(objectId)) {
        operations.push({ op: "REMOVE", address: { kind: "Entity", objectId } });
      }
    }
    if (!operations.length) return;

    this.repo.applyCommit({
      targetRepository: this.repo.repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: head,
      expectedTargetCommit: head,
      operations,
      message,
      provenance: { originKind: "DEFINITION", activityRef: "catalog-registry" },
    });
  }

  history(limit = 20, objectId?: string): readonly CatalogCommit[] {
    const args = ["log", `-${limit}`, "--format=%H\t%s"];
    if (objectId) args.push("--", `${objectId}.json`);
    const raw = execFileSync("git", args, {
      cwd: this.repo.rootDir,
      encoding: "utf8",
    }).trim();
    if (!raw) return [];
    return raw.split("\n").map((line) => {
      const tab = line.indexOf("\t");
      return { commit: line.slice(0, tab), message: line.slice(tab + 1) };
    });
  }
}

export function catalogRegistryPath(home: string): string {
  return path.join(home, "repos", "_catalog");
}

export function catalogRegistryId(namespace: string): string {
  return `kr://${namespace}/catalog`;
}

export function catalogRegistryExists(home: string): boolean {
  return existsSync(path.join(catalogRegistryPath(home), ".git"));
}

function normalizeCatalogState(state: CatalogState): CatalogState {
  return {
    views: [...state.views].sort((a, b) => a.viewId.localeCompare(b.viewId)),
    generations: [...state.generations].sort((a, b) => a.generationId.localeCompare(b.generationId)),
    releases: state.releases,
  };
}
