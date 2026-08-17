import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { Catalog } from "../../src/catalog/catalog.ts";
import { Store } from "../../src/store.ts";
import { expectCode, makeRepository } from "./helpers.ts";

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
  it("RESOLVE_VIEW produces a deterministic generation id (G2)", () => {
    const { catalog } = setup();
    const def = catalog.defineView("alice-default", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/groups/payments", selector: "refs/heads/main" },
      { repository: "kr://acme/personals/alice", selector: "refs/heads/main" },
    ]);
    const g1 = catalog.resolveView(def);
    const g2 = catalog.resolveView(def);
    expect(g1.generationId).toBe(g2.generationId);
    expect(Object.keys(g1.repositories)).toHaveLength(3);
  });

  it("rejects a repository appearing twice in one generation (K-10)", () => {
    const { catalog } = setup();
    const def = catalog.defineView("dup", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
    ]);
    expectCode(() => catalog.resolveView(def), "VIEW_GENERATION_INVALID");
  });

  it("federated read preserves every source and never overrides (K-12/K-13)", () => {
    const { catalog } = setup();
    const def = catalog.defineView("v", 1, [
      { repository: "kr://acme/public/core", selector: "refs/heads/main" },
      { repository: "kr://acme/groups/payments", selector: "refs/heads/main" },
    ]);
    const gen = catalog.resolveView(def);
    const results = catalog.readObject(gen, "policy/P-103");
    expect(results).toHaveLength(2);
    const byRepo = new Map(results.map((r) => [r.repository, r.value]));
    expect(byRepo.get("kr://acme/public/core")).toEqual({ statement: "public v1" });
    expect(byRepo.get("kr://acme/groups/payments")).toEqual({ statement: "group qualification" });
    expect(results.every((r) => r.commit)).toBe(true);
  });

  it("promote/rollback are CAS on the channel, never touching repos (K-22)", () => {
    const { catalog, publicRepo } = setup();
    const def = catalog.defineView("v", 1, [{ repository: "kr://acme/public/core", selector: "refs/heads/main" }]);
    const gen = catalog.resolveView(def);
    const headBefore = publicRepo.head("refs/heads/main");

    catalog.promote("stable", undefined, gen.generationId);
    expect(catalog.channel("stable")).toBe(gen.generationId);
    expect(publicRepo.head("refs/heads/main")).toBe(headBefore);

    expectCode(() => catalog.promote("stable", "stale", gen.generationId), "PROMOTION_CAS_FAILED");

    catalog.rollback("stable", gen.generationId, "prior-gen");
    expect(catalog.channel("stable")).toBe("prior-gen");
    expect(publicRepo.head("refs/heads/main")).toBe(headBefore);
  });
});
