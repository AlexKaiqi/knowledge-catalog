import { describe, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { expectCode, setup } from "./helpers.ts";

describe("T2 — Commit CAS", () => {
  it("rejects a write with a stale expected target commit", () => {
    const { ingress, repositoryId, rootCommitId } = setup();

    const first: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: rootCommitId,
      expectedTargetCommit: rootCommitId,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    };
    ingress.commit("cmd-1", first);

    // Stale expectedTargetCommit (still root) after ref has moved.
    const stale: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: rootCommitId,
      expectedTargetCommit: rootCommitId,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "b" }, value: 2 }],
    };
    expectCode(() => ingress.commit("cmd-2", stale), "NON_FAST_FORWARD");
  });
});
