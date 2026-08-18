import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { Catalog } from "../../src/catalog/catalog.ts";
import { GitCatalogRegistry } from "../../src/catalog/git-registry.ts";
import { FileCatalogRegistry } from "../../src/catalog/registry.ts";
import { Store } from "../../src/store.ts";
import { expectCode, makeRepository } from "./helpers.ts";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

function makeRepo(repoId: string, objects: Record<string, unknown>) {
  const repo = makeRepository(repoId);
  let head = repo.head("refs/heads/main");
  for (const [objectId, value] of Object.entries(objects)) {
    const cs: CommitChangeSet = {
      targetRepository: repoId,
      targetRef: "refs/heads/main",
      baseCommit: head,
      expectedTargetCommit: head,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId }, value }],
    };
    head = repo.applyCommit(cs);
  }
  return repo;
}

function setup() {
  const publicRepo = makeRepo("kr://acme/public/core", { "policy/P-103": { statement: "public v1" } });
  const groupRepo = makeRepo("kr://acme/groups/payments", {
    "policy/P-103": { statement: "group qualification" },
    "assertion/A-27": { about: "policy/P-103" },
  });
  const personalRepo = makeRepo("kr://acme/personals/alice", { "note/oncall": { text: "check freeze" } });

  const store = new Store();
  store.addRepository(publicRepo);
  store.addRepository(groupRepo);
  store.addRepository(personalRepo);
  const catalog = new Catalog(store);
  return { catalog, publicRepo, groupRepo, personalRepo, store };
}

describe("Catalog multi-repo federation (real git)", () => {
  it("PIN_VIEW produces a deterministic generation id (G2)", () => {
    const { catalog } = setup();
    const def = catalog.defineView("alice-default", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/groups/payments", selector: "refs/heads/main" },
      { repository: "kr://acme/personals/alice", selector: "refs/heads/main" },
    ]);
    const g1 = catalog.pinView(def);
    const g2 = catalog.pinView(def);
    expect(g1.generationId).toBe(g2.generationId);
    expect(Object.keys(g1.repositories)).toHaveLength(3);
  });

  it("rejects duplicate repositories and unresolved selectors (K-10/K-11)", () => {
    const { catalog } = setup();
    const duplicate = catalog.defineView("dup", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
    ]);
    expectCode(() => catalog.pinView(duplicate), "VIEW_GENERATION_INVALID");

    const unresolved = catalog.defineView("bad-ref", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/missing" },
    ]);
    expectCode(() => catalog.pinView(unresolved), "VIEW_GENERATION_INVALID");
  });

  it("federated read preserves every source and never overrides (K-12/K-13)", () => {
    const { catalog } = setup();
    const def = catalog.defineView("v", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/groups/payments", selector: "refs/heads/main" },
    ]);
    const gen = catalog.pinView(def);
    const results = catalog.federatedRead(gen, "policy/P-103");
    expect(results).toHaveLength(2);
    const byRepo = new Map(results.map((r) => [r.repository, r.value]));
    expect(byRepo.get("kr://acme/public/core")).toEqual({ statement: "public v1" });
    expect(byRepo.get("kr://acme/groups/payments")).toEqual({ statement: "group qualification" });
    expect(results.every((r) => r.commit)).toBe(true);
  });

  it("propagates generation/backend failures instead of reporting false absence", () => {
    const { catalog, store } = setup();
    const def = catalog.defineView("v", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
    ]);
    const gen = catalog.pinView(def);
    expect(catalog.federatedRead(gen, "absent")).toEqual([]);

    store.repos.delete("kr://acme/public/core");
    expectCode(() => catalog.federatedRead(gen, "absent"), "TEMPORARY_UNAVAILABLE");
  });

  it("promote/rollback require registered generations and never touch repos (K-22)", () => {
    const { catalog, publicRepo } = setup();
    const source = [{ repository: "kr://acme/public/core", selector: "refs/heads/main" }] as const;
    const prior = catalog.pinView(catalog.defineView("v", 0, source));
    const gen = catalog.pinView(catalog.defineView("v", 1, source));
    const headBefore = publicRepo.head("refs/heads/main");

    expectCode(() => catalog.promote("stable", undefined, "unknown-generation"), "VIEW_GENERATION_INVALID");
    catalog.promote("stable", undefined, gen.generationId);
    expect(catalog.release("stable")).toBe(gen.generationId);
    expect(publicRepo.head("refs/heads/main")).toBe(headBefore);

    expectCode(() => catalog.promote("stable", "stale", gen.generationId), "PROMOTION_CAS_FAILED");

    catalog.rollback("stable", gen.generationId, prior.generationId);
    expect(catalog.release("stable")).toBe(prior.generationId);
    expect(publicRepo.head("refs/heads/main")).toBe(headBefore);
  });

  it("readRelease uses the pinned generation, not a live ref", () => {
    const { catalog, publicRepo } = setup();
    const def = catalog.defineView("v", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
    ]);
    const gen = catalog.pinView(def);
    catalog.promote("stable", undefined, gen.generationId);

    const head = publicRepo.head("refs/heads/main");
    publicRepo.applyCommit({
      targetRepository: "kr://acme/public/core",
      targetRef: "refs/heads/main",
      baseCommit: head,
      expectedTargetCommit: head,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "policy/P-103" }, value: { statement: "later" } }],
    });

    expect(catalog.readRelease("stable", "policy/P-103")).toEqual([
      {
        repository: "kr://acme/public/core",
        commit: gen.repositories["kr://acme/public/core"],
        objectId: "policy/P-103",
        value: { statement: "public v1" },
      },
    ]);
    expectCode(() => catalog.readRelease("missing", "policy/P-103"), "VIEW_GENERATION_INVALID");
  });

  it("publish pins then promotes; file registry survives a new Catalog", () => {
    const { store, publicRepo } = setup();
    const dir = mkdtempSync(path.join(tmpdir(), "kc-cat-"));
    try {
      const registry = new FileCatalogRegistry(path.join(dir, "catalog.json"));
      const catalog = new Catalog(store, registry);
      catalog.defineView("v", 1, [{ repository: "kr://acme/public/core", selector: "refs/heads/main" }]);
      const published = catalog.publish("stable", "v");
      expect(catalog.release("stable")).toBe(published.generationId);

      const again = new Catalog(store, registry);
      expect(again.release("stable")).toBe(published.generationId);
      expect(again.readRelease("stable", "policy/P-103")[0]?.value).toEqual({ statement: "public v1" });

      const head = publicRepo.head("refs/heads/main");
      publicRepo.applyCommit({
        targetRepository: "kr://acme/public/core",
        targetRef: "refs/heads/main",
        baseCommit: head,
        expectedTargetCommit: head,
        operations: [{ op: "PUT", address: { kind: "Entity", objectId: "policy/P-103" }, value: { statement: "later" } }],
      });
      const next = again.publish("stable", "v");
      expect(next.generationId).not.toBe(published.generationId);
      expect(again.readRelease("stable", "policy/P-103")[0]?.value).toEqual({ statement: "later" });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("git registry records promote history and checkGeneration sees unmounted members", () => {
    const { store } = setup();
    const dir = mkdtempSync(path.join(tmpdir(), "kc-cat-git-"));
    try {
      const registry = new GitCatalogRegistry(path.join(dir, "catalog"), "kr://acme/catalog");
      const catalog = new Catalog(store, registry);
      catalog.defineView("v", 1, [{ repository: "kr://acme/public/core", selector: "refs/heads/main" }]);
      const published = catalog.publish("stable", "v");
      expect(catalog.checkGeneration(published.generationId).outcome).toBe("PASSED");

      const again = new Catalog(store, registry);
      expect(again.release("stable")).toBe(published.generationId);
      const promoteLog = registry.history(20, "release/stable");
      expect(promoteLog.some((item) => item.message.startsWith("promote stable"))).toBe(true);
      expect(promoteLog.some((item) => item.message.startsWith("define-view"))).toBe(false);

      store.repos.delete("kr://acme/public/core");
      const check = again.checkGeneration(published.generationId);
      expect(check.outcome).toBe("FAILED");
      expect(check.issues[0]?.code).toBe("TEMPORARY_UNAVAILABLE");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
