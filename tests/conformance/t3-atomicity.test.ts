import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { expectCode, setup } from "./helpers.ts";

describe("T3 — ChangeSet Atomicity", () => {
  it("no partial commit when any operation fails", () => {
    const { ingress, access, repositoryId, rootCommitId } = setup();

    const first: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: rootCommitId,
      expectedTargetCommit: rootCommitId,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    };
    const c1 = ingress.commit("cmd-1", first);

    // Two operations: first would succeed, second fails (IF_ABSENT on existing "a").
    const failing: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: c1.result.commitId,
      expectedTargetCommit: c1.result.commitId,
      operations: [
        { op: "PUT", address: { kind: "Entity", objectId: "b" }, value: 2 },
        { op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 3, precondition: { type: "IF_ABSENT" } },
      ],
    };
    expectCode(() => ingress.commit("cmd-2", failing), "PRECONDITION_FAILED");

    // "b" must NOT have been committed; head is still c1.
    const head = ingress["store"].repos.get(repositoryId)!.head();
    expect(head).toBe(c1.result.commitId);
    expect(access.list(repositoryId, head).map((v) => v.address.objectId)).toEqual(["a"]);
  });
});
