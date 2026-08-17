import { expect } from "vitest";
import { Access, Ingress, MemoryRepository, MemoryStore } from "../../src/index.ts";

export function setup(repositoryId = "kr://acme/public/core", rootCommitId = "P0") {
  const repo = new MemoryRepository(repositoryId, rootCommitId);
  const store = new MemoryStore();
  store.addRepository(repo);
  const ingress = new Ingress(store);
  const access = new Access(store);
  return { repo, store, ingress, access, repositoryId, rootCommitId };
}

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
