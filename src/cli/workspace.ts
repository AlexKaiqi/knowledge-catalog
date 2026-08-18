/**
 * CLI workspace — mounts FileGit member repos. Catalog registry is its own
 * FileGit repo (kr://<namespace>/catalog), not a member of user views.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { FileGitRepository } from "../adapters/file-git/repository.ts";
import { FileIdempotencyStore } from "../api/idempotency.ts";
import { Reader } from "../api/reader.ts";
import { Writer } from "../api/writer.ts";
import { Catalog, type CatalogState } from "../catalog/catalog.ts";
import {
  catalogRegistryId,
  catalogRegistryPath,
  GitCatalogRegistry,
} from "../catalog/git-registry.ts";
import { ControlPlane } from "../control-plane/maintenance.ts";
import { FileControlState, type ControlState } from "../control-plane/state.ts";
import { Store } from "../store.ts";

export interface WorkspaceFile {
  readonly namespace: string;
  readonly repos: readonly { readonly id: string; readonly dir: string }[];
}

export interface OpenWorkspace {
  readonly home: string;
  readonly store: Store;
  readonly writer: Writer;
  readonly reader: Reader;
  readonly catalog: Catalog;
  readonly catalogRegistry: GitCatalogRegistry;
  readonly controlPlane: ControlPlane;
  readonly controlStore: FileControlState;
  file: WorkspaceFile;
  control: ControlState;
}

const DEFAULT_NAMESPACE = "local";

export function workspacePath(home: string): string {
  return path.join(home, "workspace.json");
}

export function encodeRepoDir(repositoryId: string): string {
  return repositoryId.replace(/[^A-Za-z0-9._-]+/g, "_");
}

export function initWorkspace(home: string, namespace = DEFAULT_NAMESPACE): WorkspaceFile {
  mkdirSync(path.join(home, "repos"), { recursive: true });
  const empty: WorkspaceFile = { namespace, repos: [] };
  if (!existsSync(workspacePath(home))) {
    writeWorkspace(home, empty);
  }
  return readWorkspace(home);
}

export function readWorkspace(home: string): WorkspaceFile {
  const file = workspacePath(home);
  if (!existsSync(file)) {
    throw new Error(`no workspace at ${home}; run: kc init --home ${home}`);
  }
  const raw = JSON.parse(readFileSync(file, "utf8")) as Partial<WorkspaceFile>;
  return { namespace: raw.namespace ?? DEFAULT_NAMESPACE, repos: raw.repos ?? [] };
}

export function writeWorkspace(home: string, file: WorkspaceFile): void {
  mkdirSync(home, { recursive: true });
  writeFileSync(
    workspacePath(home),
    `${JSON.stringify({ namespace: file.namespace, repos: file.repos }, null, 2)}\n`,
    "utf8",
  );
}

export function openWorkspace(home: string): OpenWorkspace {
  migrateLegacyJsonCatalog(home);
  const file = readWorkspace(home);
  const store = new Store();
  for (const repo of file.repos) {
    store.addRepository(new FileGitRepository(path.join(home, repo.dir), repo.id));
  }
  const catalogRegistry = new GitCatalogRegistry(
    catalogRegistryPath(home),
    catalogRegistryId(file.namespace),
  );
  const writer = new Writer(store, new FileIdempotencyStore(path.join(home, "writer.json")));
  const catalog = new Catalog(store, catalogRegistry);
  const controlStore = new FileControlState(path.join(home, "control.json"));
  return {
    home,
    store,
    writer,
    reader: new Reader(store),
    catalog,
    catalogRegistry,
    controlPlane: new ControlPlane(store, writer, catalog),
    controlStore,
    file,
    control: controlStore.load(),
  };
}

export function persistControl(ws: OpenWorkspace): void {
  ws.controlStore.save(ws.control);
}

export function addRepository(ws: OpenWorkspace, repositoryId: string): string {
  if (repositoryId === catalogRegistryId(ws.file.namespace)) {
    throw new Error(`${repositoryId} is reserved for the Catalog registry`);
  }
  if (ws.file.repos.some((r) => r.id === repositoryId)) {
    throw new Error(`repository already registered: ${repositoryId}`);
  }
  const dir = path.posix.join("repos", encodeRepoDir(repositoryId));
  const repo = new FileGitRepository(path.join(ws.home, dir), repositoryId);
  ws.store.addRepository(repo);
  ws.file = { ...ws.file, repos: [...ws.file.repos, { id: repositoryId, dir }] };
  writeWorkspace(ws.home, ws.file);
  return repo.head("refs/heads/main");
}

/** Import a leftover catalog.json into the git registry once. */
function migrateLegacyJsonCatalog(home: string): void {
  const jsonFile = path.join(home, "catalog.json");
  if (!existsSync(jsonFile)) return;
  const raw = JSON.parse(readFileSync(jsonFile, "utf8")) as Partial<CatalogState>;
  const state: CatalogState = {
    views: raw.views ?? [],
    generations: raw.generations ?? [],
    releases: raw.releases ?? {},
  };
  if (state.views.length === 0 && state.generations.length === 0 && Object.keys(state.releases).length === 0) {
    return;
  }
  const namespace = existsSync(workspacePath(home)) ? readWorkspace(home).namespace : DEFAULT_NAMESPACE;
  const git = new GitCatalogRegistry(catalogRegistryPath(home), catalogRegistryId(namespace));
  if (git.repo.list(git.repo.head()).length === 0) {
    git.save(state, "catalog: import catalog.json");
  }
}
