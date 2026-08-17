import { describe, expect, it } from "vitest";
import { expectCode, setup } from "./helpers.ts";

describe("T5 — Append Idempotency", () => {
  it("replays same event id, conflicts on different payload", () => {
    const { ingress, repo, repositoryId } = setup();
    const headBefore = repo.head("refs/heads/main");

    const entries = [{ eventId: "evt-1", payload: { outcome: "PASSED" } }];
    const r1 = ingress.append("cmd-a", { targetRepository: repositoryId, streamRef: "evidence", entries });
    expect(r1.result.appended).toHaveLength(1);

    const r2 = ingress.append("cmd-b", { targetRepository: repositoryId, streamRef: "evidence", entries });
    expect(r2.result.appended).toEqual(r1.result.appended); // same record id
    expect(repo.head("refs/heads/main")).toBe(headBefore); // side stream does not move Git

    expectCode(
      () =>
        ingress.append("cmd-c", {
          targetRepository: repositoryId,
          streamRef: "evidence",
          entries: [{ eventId: "evt-1", payload: { outcome: "FAILED" } }],
        }),
      "EVENT_ID_CONFLICT",
    );
  });
});
