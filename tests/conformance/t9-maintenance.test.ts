import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { Writer } from "../../src/api/writer.ts";
import { Catalog } from "../../src/catalog/catalog.ts";
import { ControlPlane } from "../../src/control-plane/maintenance.ts";
import { expectCode, makeRepository, setup as setupRepository } from "./helpers.ts";

function setup() {
  const base = setupRepository("kr://acme/public/core");
  const supportRepo = makeRepository("kr://acme/groups/support");
  base.store.addRepository(supportRepo);
  const catalog = new Catalog(base.store);
  const definition = catalog.defineView("maintenance", 1, [
    { repository: base.repositoryId, selector: "refs/heads/main" },
    { repository: supportRepo.repositoryId, selector: "refs/heads/main" },
  ]);
  const baseGeneration = catalog.pinView(definition);
  const cp = new ControlPlane(base.store, base.writer, catalog);
  return { ...base, supportRepo, catalog, definition, baseGeneration, cp };
}

function commitToMain(writer: Writer, repositoryId: string, base: string, objectId: string, value: unknown): string {
  const cs: CommitChangeSet = {
    targetRepository: repositoryId,
    targetRef: "refs/heads/main",
    baseCommit: base,
    expectedTargetCommit: base,
    operations: [{ op: "PUT", address: { kind: "Entity", objectId }, value }],
  };
  return writer.commit(`main:${objectId}:${base}`, cs).result.commitId;
}

describe("ControlPlane maintenance loop (Phase 3, real git)", () => {
  it("PROPOSAL writes a candidate branch and never moves main (T6)", () => {
    const { repo, cp, repositoryId } = setup();
    const base = repo.head("refs/heads/main");

    const proposal = cp.propose({
      proposalId: "PR-1",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-1",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "policy/P-103" }, value: { v: "candidate" } }],
    });

    expect(proposal.candidateCommit).not.toBe(base);
    expect(repo.head("refs/heads/main")).toBe(base);
    expect(repo.head("refs/heads/candidates/PR-1")).toBe(proposal.candidateCommit);
  });

  it("validation binds a complete preview; a moved candidate invalidates it (K-09/ADR-013)", () => {
    const { repo, supportRepo, cp, repositoryId, baseGeneration } = setup();
    const base = repo.head("refs/heads/main");

    const p1 = cp.propose({
      proposalId: "PR-2",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-2",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview1 = cp.createPreview(baseGeneration.generationId, p1);
    expect(Object.keys(preview1.generation.repositories)).toHaveLength(2);
    expect(preview1.generation.repositories[supportRepo.repositoryId]).toBe(
      baseGeneration.repositories[supportRepo.repositoryId],
    );
    const val1 = cp.recordValidation(preview1, "S7", "PASSED");
    expectCode(
      () => cp.merge(p1, preview1, { ...val1, previewGenerationId: baseGeneration.generationId }),
      "VALIDATION_BASIS_MISMATCH",
    );

    const p2 = cp.propose({
      proposalId: "PR-2b",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-2",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 2 }],
    });

    expectCode(() => cp.merge(p1, preview1, val1), "CANDIDATE_MOVED");

    const preview2 = cp.createPreview(baseGeneration.generationId, p2);
    const val2 = cp.recordValidation(preview2, "S7", "PASSED");
    expect(cp.merge(p2, preview2, val2)).toBe(p2.candidateCommit);
  });

  it("validateStructure runs mounted-repo checks then records the outcome", () => {
    const { repo, store, cp, repositoryId, baseGeneration } = setup();
    const base = repo.head("refs/heads/main");
    const proposal = cp.propose({
      proposalId: "PR-struct",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-struct",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview = cp.createPreview(baseGeneration.generationId, proposal);
    const passed = cp.validateStructure(preview);
    expect(passed.outcome).toBe("PASSED");
    expect(passed.suiteRevision).toBe("structure");
    expect(passed.check.issues).toEqual([]);

    store.repos.delete(repositoryId);
    const failed = cp.validateStructure(preview);
    expect(failed.outcome).toBe("FAILED");
    expect(failed.check.issues.some((issue) => issue.code === "TEMPORARY_UNAVAILABLE")).toBe(true);
  });

  it("merge and Catalog promotion are separate CAS operations", () => {
    const { repo, cp, catalog, definition, baseGeneration, repositoryId } = setup();
    const base = repo.head("refs/heads/main");

    const p = cp.propose({
      proposalId: "PR-3",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-3",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview = cp.createPreview(baseGeneration.generationId, p);
    const val = cp.recordValidation(preview, "S7", "PASSED");

    const merged = cp.merge(p, preview, val);
    expect(repo.head("refs/heads/main")).toBe(merged);
    expect(catalog.release("stable")).toBeUndefined();

    const servingGeneration = catalog.pinView(definition);
    catalog.promote("stable", undefined, servingGeneration.generationId);
    expect(catalog.release("stable")).toBe(servingGeneration.generationId);
    expect(repo.head("refs/heads/main")).toBe(merged);
  });

  it("merge rejects when main moved underneath (CAS)", () => {
    const { repo, writer, cp, repositoryId, baseGeneration } = setup();
    const base = repo.head("refs/heads/main");

    const p = cp.propose({
      proposalId: "PR-4",
      repositoryId,
      targetRef: "refs/heads/main",
      candidateRef: "refs/heads/candidates/PR-4",
      baseCommit: base,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    });
    const preview = cp.createPreview(baseGeneration.generationId, p);
    const val = cp.recordValidation(preview, "S7", "PASSED");

    const newHead = commitToMain(writer, repositoryId, base, "other", 99);
    expect(newHead).not.toBe(base);

    expectCode(() => cp.merge(p, preview, val), "NON_FAST_FORWARD");
  });
});
