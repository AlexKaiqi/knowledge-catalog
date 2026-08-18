import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { CommitChangeSet } from "../../src/index.ts";
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

  it("returns provenance via ORIGIN from frontmatter", () => {
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
    const trace = repo.origin("policy/P-103", c1);
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
});
