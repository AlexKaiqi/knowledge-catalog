import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { CommitChangeSet } from "../../src/contracts/index.ts";
import { FileGitRepository } from "../../src/adapters/file-git/repository.ts";
import { expectCode } from "./helpers.ts";

const tmpDirs: string[] = [];

function makeRepo(repositoryId = "kr://acme/public/core"): FileGitRepository {
  const dir = mkdtempSync(path.join(tmpdir(), "kc-"));
  tmpDirs.push(dir);
  return new FileGitRepository(dir, repositoryId);
}

function put(objectId: string, value: unknown, pathHint?: string): CommitChangeSet["operations"] {
  return [{ op: "PUT", address: { kind: "Entity", objectId }, value, pathHint }];
}

afterEach(() => {
  for (const d of tmpDirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

describe("FileGitRepository (Phase 1)", () => {
  it("commits real files with object_id in frontmatter", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("policy/P-103", { statement: "v1" }, "policies/P-103.json"),
      provenance: { originKind: "DEFINITION", actorRef: "core-council" },
    });
    expect(c1).not.toBe(root);

    const res = repo.resolve("policy/P-103", c1);
    expect(res.status).toBe("RESOLVED");
    expect(res.pathHint).toBe("policies/P-103.json");
  });

  it("survives a path move (T1 on real filesystem)", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("policy/P-103", { statement: "v1" }, "policies/P-103.json"),
    });
    const c2 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: c1,
      expectedTargetCommit: c1,
      operations: put("policy/P-103", { statement: "v1" }, "policies/production/P-103.json"),
    });

    const res = repo.resolve("policy/P-103", c2);
    expect(res.status).toBe("RESOLVED");
    expect(res.pathHint).toBe("policies/production/P-103.json");
  });

  it("rejects a stale expected commit (CAS)", () => {
    const repo = makeRepo();
    const root = repo.head();
    repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("a", 1),
    });
    expectCode(
      () =>
        repo.applyCommit({
          targetRepository: repo.repositoryId,
          targetRef: "HEAD",
          baseCommit: root,
          expectedTargetCommit: root, // stale
          operations: put("b", 2),
        }),
      "NON_FAST_FORWARD",
    );
  });

  it("returns provenance via GET_PROVENANCE from frontmatter", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("policy/P-103", { statement: "v1" }, "policies/P-103.json"),
      provenance: { originKind: "DEFINITION", actorRef: "core-council", sourceRefs: ["handbook-v1"] },
    });
    const trace = repo.getProvenance("policy/P-103", c1);
    expect(trace.chain).toHaveLength(1);
    expect(trace.chain[0]?.originKind).toBe("DEFINITION");
    expect(trace.chain[0]?.actorRef).toBe("core-council");
  });

  it("rejects paths that escape the repository root", () => {
    const repo = makeRepo();
    const root = repo.head();
    expectCode(
      () => repo.applyCommit({
        targetRepository: repo.repositoryId,
        targetRef: "HEAD",
        baseCommit: root,
        expectedTargetCommit: root,
        operations: put("escape", 1, "../escape.json"),
      }),
      "PRECONDITION_FAILED",
    );
  });

  it("requires merge to be a true fast-forward", () => {
    const repo = makeRepo();
    const root = repo.head("refs/heads/main");
    repo.createRef("refs/heads/candidate", root);
    const candidate = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "refs/heads/candidate",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("candidate", 1),
    });
    const main = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("main", 2),
    });
    expectCode(() => repo.merge("refs/heads/main", candidate, main), "NON_FAST_FORWARD");
  });

  it("passes commit messages as arguments, not shell commands", () => {
    const repo = makeRepo();
    const root = repo.head();
    const marker = path.join(repo.rootDir, "PWNED");
    repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("safe-message", 1),
      message: `message\"; touch ${marker}; #`,
    });
    expect(existsSync(marker)).toBe(false);
  });

  it("reads pinned commits from the Git tree, not dirty working files", () => {
    const repo = makeRepo();
    const root = repo.head();
    const commit = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("a", { value: "committed" }, "objects/a.json"),
    });
    writeFileSync(
      path.join(repo.rootDir, "objects/a.json"),
      "---\nobject_id: a\n---\n{\"value\":\"dirty\"}\n",
      "utf8",
    );

    expect(repo.read("a", commit).value).toEqual({ value: "committed" });
  });

  it("rejects incomplete DERIVATION provenance", () => {
    const repo = makeRepo();
    const root = repo.head();
    expectCode(
      () => repo.applyCommit({
        targetRepository: repo.repositoryId,
        targetRef: "HEAD",
        baseCommit: root,
        expectedTargetCommit: root,
        operations: put("derived", 1),
        provenance: { originKind: "DERIVATION", inputViewReadVersionRef: "vr-1" },
      }),
      "PRECONDITION_FAILED",
    );

    const commit = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("derived", 1),
      provenance: {
        originKind: "DERIVATION",
        inputViewReadVersionRef: "vr-1",
        algorithm: { codeHash: "sha256:abc" },
      },
    });
    expect(repo.read("derived", commit).value).toBe(1);
  });

  it("writes aspects as separate units and assembles on entity read", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: [
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "Table:tl.db.t", aspectName: "structure" },
          value: { storage_type: "hive" },
        },
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "Table:tl.db.t", aspectName: "ownership" },
          value: { owner: "alice" },
        },
      ],
    });

    expect(repo.read("Table:tl.db.t", c1).value).toEqual({
      structure: { storage_type: "hive" },
      ownership: { owner: "alice" },
    });
    expect(repo.list(c1)).toHaveLength(1);
    expect(
      repo.readAddress({ kind: "Aspect", objectId: "Table:tl.db.t", aspectName: "structure" }, c1).value,
    ).toEqual({ storage_type: "hive" });
  });

  it("CAS is per aspect; updating one unit leaves the other", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: [
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "t", aspectName: "structure" },
          value: { pk: ["id"] },
        },
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "t", aspectName: "ownership" },
          value: { owner: "alice" },
        },
      ],
    });
    const structure = repo.resolveAddress({ kind: "Aspect", objectId: "t", aspectName: "structure" }, c1);
    const c2 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: c1,
      expectedTargetCommit: c1,
      operations: [
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "t", aspectName: "structure" },
          value: { pk: ["id", "ds"] },
          precondition: { type: "IF_DIGEST_EQUALS", digest: structure.digest },
        },
      ],
    });

    expect(repo.read("t", c2).value).toEqual({
      structure: { pk: ["id", "ds"] },
      ownership: { owner: "alice" },
    });
    expectCode(
      () =>
        repo.applyCommit({
          targetRepository: repo.repositoryId,
          targetRef: "HEAD",
          baseCommit: c2,
          expectedTargetCommit: c2,
          operations: [
            {
              op: "PUT",
              address: { kind: "Aspect", objectId: "t", aspectName: "structure" },
              value: { pk: ["x"] },
              precondition: { type: "IF_DIGEST_EQUALS", digest: structure.digest },
            },
          ],
        }),
      "PRECONDITION_FAILED",
    );
  });

  it("rejects mixing an entity blob with aspects on the same object_id", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: put("mixed", { blob: true }),
    });
    expectCode(
      () =>
        repo.applyCommit({
          targetRepository: repo.repositoryId,
          targetRef: "HEAD",
          baseCommit: c1,
          expectedTargetCommit: c1,
          operations: [
            {
              op: "PUT",
              address: { kind: "Aspect", objectId: "mixed", aspectName: "structure" },
              value: { x: 1 },
            },
          ],
        }),
      "PRECONDITION_FAILED",
    );
  });

  it("assembles keyed collection members and removes one aspect", () => {
    const repo = makeRepo();
    const root = repo.head();
    const c1 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: root,
      expectedTargetCommit: root,
      operations: [
        {
          op: "PUT",
          address: { kind: "Aspect", objectId: "t", aspectName: "structure" },
          value: { ok: true },
        },
        {
          op: "PUT",
          address: { kind: "Member", objectId: "t", aspectName: "permissions", memberKey: "user:a" },
          value: { privileges: ["SELECT"] },
        },
        {
          op: "PUT",
          address: { kind: "Member", objectId: "t", aspectName: "permissions", memberKey: "user:b" },
          value: { privileges: ["ALL"] },
        },
      ],
    });
    expect(repo.read("t", c1).value).toEqual({
      structure: { ok: true },
      permissions: {
        "user:a": { privileges: ["SELECT"] },
        "user:b": { privileges: ["ALL"] },
      },
    });

    const c2 = repo.applyCommit({
      targetRepository: repo.repositoryId,
      targetRef: "HEAD",
      baseCommit: c1,
      expectedTargetCommit: c1,
      operations: [{ op: "REMOVE", address: { kind: "Aspect", objectId: "t", aspectName: "structure" } }],
    });
    expect(repo.read("t", c2).value).toEqual({
      permissions: {
        "user:a": { privileges: ["SELECT"] },
        "user:b": { privileges: ["ALL"] },
      },
    });
  });
});
