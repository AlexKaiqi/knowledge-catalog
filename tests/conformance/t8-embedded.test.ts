import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { MemoryRepository } from "../../src/adapters/memory/repository.ts";
import { SqliteProjection } from "../../src/adapters/embedded/projection.ts";

function buildRepo(): { repo: MemoryRepository; head: string } {
  const repo = new MemoryRepository("kr://acme/public/core", "P0");
  const put = (objectId: string, value: unknown): CommitChangeSet["operations"] => [
    { op: "PUT", address: { kind: "Entity", objectId }, value },
  ];
  let head = repo.head();
  head = repo.applyCommit({
    targetRepository: repo.repositoryId,
    targetRef: "refs/heads/main",
    baseCommit: head,
    expectedTargetCommit: head,
    operations: put("policy/P-103", { title: "refund policy", body: "production requires a tested runbook" }),
  });
  head = repo.applyCommit({
    targetRepository: repo.repositoryId,
    targetRef: "refs/heads/main",
    baseCommit: head,
    expectedTargetCommit: head,
    operations: put("procedure/refund-timeout", { title: "refund timeout", body: "diagnose runbook failures" }),
  });
  return { repo, head };
}

describe("SqliteProjection (Phase 2 Embedded Access)", () => {
  it("locates via FTS5 and reads values back from Canonical", () => {
    const { repo, head } = buildRepo();
    const proj = new SqliteProjection();
    proj.build(repo, head);

    const hits = proj.search(repo, "runbook");
    expect(hits).toHaveLength(2); // both objects mention runbook
    // values come from canonical repo, with provenance-bearing shape
    expect(hits.every((h) => h.repository === repo.repositoryId)).toBe(true);
  });

  it("records basis and reports lag behind head", () => {
    const { repo, head } = buildRepo();
    const proj = new SqliteProjection();
    proj.build(repo, head);

    const desc = proj.describeIndex(repo);
    expect(desc.basisCommit).toBe(head);
    expect(desc.objectCount).toBe(2);
    expect(desc.lagBehindHead).toBe(false);

    // advance head; index is now stale (coverage/lag visible)
    const newHead = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: head,
      expectedTargetCommit: head,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "policy/P-200" }, value: { title: "new" } }],
    });
    expect(newHead).not.toBe(head);
    expect(proj.describeIndex(repo).lagBehindHead).toBe(true);
  });

  it("is rebuildable and non-authoritative (T9)", () => {
    const { repo, head } = buildRepo();
    const proj1 = new SqliteProjection();
    proj1.build(repo, head);
    const before = proj1.search(repo, "runbook").map((h) => h.address.objectId).sort();

    // rebuild a fresh index from the same canonical state
    const proj2 = new SqliteProjection();
    proj2.build(repo, head);
    const after = proj2.search(repo, "runbook").map((h) => h.address.objectId).sort();

    expect(after).toEqual(before);
    // canonical state untouched: still two objects at head
    expect(repo.list(head)).toHaveLength(2);
  });
});
