import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { Access } from "../../src/api/access.ts";
import { Ingress } from "../../src/api/ingress.ts";
import { MemoryRepository } from "../../src/adapters/memory/repository.ts";
import { ControlPlane } from "../../src/control-plane/maintenance.ts";
import { MemoryStore } from "../../src/store.ts";
import { expectCode } from "./helpers.ts";

function setup() {
  const repositoryId = "kr://acme/public/core";
  const repo = new MemoryRepository(repositoryId, "P0");
  const store = new MemoryStore();
  store.addRepository(repo);
  const ingress = new Ingress(store);
  const access = new Access(store);
  const cp = new ControlPlane(store, ingress);
  return { repo, store, ingress, access, cp, repositoryId };
}

function commitToMain(ingress: Ingress, repositoryId: string, base: string, objectId: string, value: unknown): string {
  const cs: CommitChangeSet = {
    targetRepository: repositoryId,
    targetRef: "refs/heads/main",
    baseCommit: base,
    expectedTargetCommit: base,
    operations: [{ op: "PUT", address: { kind: "Entity", objectId }, value }],
  };
  return ingress.commit(`main:${objectId}:${base}`, cs).result.commitId;
}

describe("ControlPlane maintenance loop (Phase 3)", () => {
  it("PROPOSAL writes a candidate branch and never moves main (T6)", () => {
    const { repo, ingress, cp, repositoryId } = setup();
    const base = repo.head();

    const proposal = cp.propose({
      proposalId: "PR-1",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-1",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "policy/P-103" }, value: { v: "candidate" } }],
    });

    expect(proposal.candidateCommit).not.toBe(base);
    expect(repo.head("refs/heads/main")).toBe(base); // main unchanged
    expect(repo.head("refs/heads/candidates/PR-1")).toBe(proposal.candidateCommit);
  });

  it("validation binds the preview; a moved candidate invalidates it (ADR-013)", () => {
    const { repo, cp, repositoryId } = setup();
    const base = repo.head();

    const p1 = cp.propose({
      proposalId: "PR-2",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-2",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview1 = cp.createPreview(p1);
    const val1 = cp.validate(preview1, "S7", "PASSED");

    // candidate advances -> a new commit on the candidate ref (same main base)
    const p2 = cp.propose({
      proposalId: "PR-2b",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-2",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 2 }],
    });

    // old validation val1 (bound to p1.candidateCommit) no longer gates: candidate moved
    expectCode(() => cp.merge(p1, val1), "CANDIDATE_MOVED");

    // new candidate needs its own preview + validation
    const preview2 = cp.createPreview(p2);
    const val2 = cp.validate(preview2, "S7", "PASSED");
    expect(cp.merge(p2, val2)).toBe(p2.candidateCommit);
  });

  it("merge is a CAS fast-forward; promote is a separate CAS on the channel", () => {
    const { repo, cp, repositoryId } = setup();
    const base = repo.head();

    const p = cp.propose({
      proposalId: "PR-3",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-3",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview = cp.createPreview(p);
    const val = cp.validate(preview, "S7", "PASSED");

    const merged = cp.merge(p, val);
    expect(repo.head("refs/heads/main")).toBe(merged);

    // promote is a separate CAS on the channel (does not touch the repo)
    cp.promote("stable", undefined, merged);
    expect(cp.channel("stable")).toBe(merged);
    expect(repo.head("refs/heads/main")).toBe(merged); // repo unchanged by promote

    // stale promote fails
    expectCode(() => cp.promote("stable", base, merged), "PROMOTION_CAS_FAILED");
  });

  it("merge rejects when main moved underneath (CAS)", () => {
    const { repo, ingress, cp, repositoryId } = setup();
    const base = repo.head();

    const p = cp.propose({
      proposalId: "PR-4",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-4",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview = cp.createPreview(p);
    const val = cp.validate(preview, "S7", "PASSED");

    // concurrent write advances main
    const newHead = commitToMain(ingress, repositoryId, base, "other", 99);
    expect(newHead).not.toBe(base);

    expectCode(() => cp.merge(p, val), "NON_FAST_FORWARD");
  });
});
