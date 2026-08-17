import { describe, expect, it } from "vitest";
import type { CommitChangeSet } from "../../src/index.ts";
import { expectCode, setup } from "./helpers.ts";

describe("T4 — Command Idempotency", () => {
  it("replays an identical command_id with REPLAYED; conflicts on different payload", () => {
    const { ingress, repositoryId, rootCommitId } = setup();

    const cs: CommitChangeSet = {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit: rootCommitId,
      expectedTargetCommit: rootCommitId,
      operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 1 }],
    };

    const r1 = ingress.commit("cmd-1", cs);
    const r2 = ingress.commit("cmd-1", cs);
    expect(r1.disposition).toBe("APPLIED");
    expect(r2.disposition).toBe("REPLAYED");
    expect(r2.result.commitId).toBe(r1.result.commitId);

    const different: CommitChangeSet = { ...cs, operations: [{ op: "PUT", address: { kind: "Entity", objectId: "a" }, value: 2 }] };
    expectCode(() => ingress.commit("cmd-1", different), "IDEMPOTENCY_CONFLICT");
  });
});
