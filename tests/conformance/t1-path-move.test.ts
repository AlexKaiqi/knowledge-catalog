import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { setup } from "./helpers.ts";

describe("T1 — Path Move", () => {
  it("object identity survives a path move", () => {
    const { ingress, access, repositoryId, rootCommitId } = setup();

    const first: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: rootCommitId,
      expectedTargetCommit: rootCommitId,
      operations: [
        {
          op: "PUT",
          address: { kind: "Entity", objectId: "policy/P-103" },
          value: { statement: "production services require an owned runbook" },
          pathHint: "policies/P-103.yaml",
        },
      ],
    };
    const c1 = ingress.commit("cmd-1", first);

    // Move: same object id, same value, different pathHint.
    const move: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: c1.result.commitId,
      expectedTargetCommit: c1.result.commitId,
      operations: [
        {
          op: "PUT",
          address: { kind: "Entity", objectId: "policy/P-103" },
          value: { statement: "production services require an owned runbook" },
          pathHint: "policies/production/P-103.yaml",
        },
      ],
    };
    const c2 = ingress.commit("cmd-2", move);

    const res = access.resolve({ repository: repositoryId, object: "policy/P-103" }, c2.result.commitId);
    expect(res.status).toBe("RESOLVED");
    expect(res.objectId).toBe("policy/P-103");
    expect(res.pathHint).toBe("policies/production/P-103.yaml");
  });
});
