import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { SqliteProjection } from "../../src/adapters/embedded/projection.ts";
import { makeRepository, setup } from "./helpers.ts";

function buildRepo() {
  const repo = makeRepository("kr://acme/public/core");
  const put = (objectId: string, value: unknown): CommitChangeSet["operations"] => [
    { op: "PUT", address: { kind: "Entity", objectId }, value },
  ];
  let head = repo.head("refs/heads/main");
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
    expect(hits).toHaveLength(2);
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

    const proj2 = new SqliteProjection();
    proj2.build(repo, head);
    const after = proj2.search(repo, "runbook").map((h) => h.address.objectId).sort();

    expect(after).toEqual(before);
    expect(repo.list(head)).toHaveLength(2);
  });

  it("indexes selected aspects only; ACL text does not locate the object", () => {
    const repo = makeRepository("kr://acme/public/core");
    let head = repo.head("refs/heads/main");
    head = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: head,
      expectedTargetCommit: head,
      operations: [
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "Table:tl.db.t", aspectName: "structure" },
          value: { storage_type: "hive", raw_description: "user events" },
        },
        {
          op: "PUT",
          address: { kind: "Member", objectId: "Table:tl.db.t", aspectName: "permissions", memberKey: "user:a" },
          value: { privileges: ["SELECT"] },
        },
      ],
    });

    const proj = new SqliteProjection();
    proj.build(repo, head, { exclude: ["permissions"] });

    expect(proj.search(repo, "hive")).toHaveLength(1);
    expect(proj.search(repo, "SELECT")).toHaveLength(0);
    expect(proj.search(repo, "hive")[0]?.value).toEqual({
      structure: { storage_type: "hive", raw_description: "user events" },
    });
  });

  it("Access reads one aspect and excludes others on assemble", () => {
    const { repo, access, repositoryId } = setup();
    const root = repo.head();
    const commit = repo.applyCommit({
      targetRepository: repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: [
        { op: "PUT", address: { kind: "Aspect", objectId: "t", aspectName: "structure" }, value: { pk: ["id"] } },
        { op: "PUT", address: { kind: "Aspect", objectId: "t", aspectName: "ownership" }, value: { owner: "alice" } },
      ],
    });

    expect(
      access.readAddress(repositoryId, { kind: "Aspect", objectId: "t", aspectName: "structure" }, commit).value,
    ).toEqual({ pk: ["id"] });
    expect(access.read({ repository: repositoryId, object: "t" }, commit, { exclude: ["ownership"] }).value).toEqual({
      structure: { pk: ["id"] },
    });
  });
});
