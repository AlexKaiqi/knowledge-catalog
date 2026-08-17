import { afterEach, describe, expect, it } from "vitest";
import { mkdtempSync, rmSync } from "node:fs";
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
});
