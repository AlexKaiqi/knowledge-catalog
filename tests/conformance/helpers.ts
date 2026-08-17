import { afterEach, expect } from "vitest";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { Access, FileGitRepository, Ingress, Store } from "../../src/index.ts";

const tmpDirs: string[] = [];

/** Create one real files+git repository in a managed temp directory. */
export function makeRepository(repositoryId = "kr://acme/public/core"): FileGitRepository {
  const dir = mkdtempSync(path.join(tmpdir(), "kc-"));
  tmpDirs.push(dir);
  return new FileGitRepository(dir, repositoryId);
}

/** Setup: ONE store implementation — real files + git (git IS the version kernel). */
export function setup(repositoryId = "kr://acme/public/core") {
  const repo = makeRepository(repositoryId);
  const store = new Store();
  store.addRepository(repo);
  const ingress = new Ingress(store);
  const access = new Access(store);
  return { repo, store, ingress, access, repositoryId, rootCommitId: repo.head() };
}

afterEach(() => {
  for (const d of tmpDirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

/** Assert that fn throws an error with the given code. */
export function expectCode(fn: () => unknown, code: string): void {
  try {
    fn();
  } catch (e) {
    expect((e as { code?: string }).code).toBe(code);
    return;
  }
  throw new Error(`expected error code ${code} but nothing was thrown`);
}
