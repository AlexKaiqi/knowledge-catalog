import { describe, expect, it } from "vitest";
import { expectCode, setup } from "./helpers.ts";

describe("T5 — Append Idempotency", () => {
  it("replays same event id, conflicts on different payload", () => {
    const { writer, repo, repositoryId } = setup();
    const headBefore = repo.head("refs/heads/main");

    const entries = [{ eventId: "evt-1", payload: { outcome: "PASSED" } }];
    const r1 = writer.append("cmd-a", { targetRepository: repositoryId, streamRef: "evidence", entries });
    expect(r1.result.appended).toHaveLength(1);

    const r2 = writer.append("cmd-b", { targetRepository: repositoryId, streamRef: "evidence", entries });
    expect(r2.result.appended).toEqual(r1.result.appended); // same record id
    expect(repo.head("refs/heads/main")).toBe(headBefore); // side stream does not move Git

    expectCode(
      () =>
        writer.append("cmd-c", {
          targetRepository: repositoryId,
          streamRef: "evidence",
          entries: [{ eventId: "evt-1", payload: { outcome: "FAILED" } }],
        }),
      "EVENT_ID_CONFLICT",
    );
  });

  it("enforces expected stream cursor before appending", () => {
    const { writer, repositoryId } = setup();
    writer.append("cursor-1", {
      targetRepository: repositoryId,
      streamRef: "ordered",
      expectedCursor: "0",
      entries: [{ eventId: "evt-1", payload: 1 }],
    });

    expectCode(
      () => writer.append("cursor-stale", {
        targetRepository: repositoryId,
        streamRef: "ordered",
        expectedCursor: "0",
        entries: [{ eventId: "evt-2", payload: 2 }],
      }),
      "PRECONDITION_FAILED",
    );

    const receipt = writer.append("cursor-2", {
      targetRepository: repositoryId,
      streamRef: "ordered",
      expectedCursor: "1",
      entries: [{ eventId: "evt-2", payload: 2 }],
    });
    expect(receipt.result.cursor).toBe("2");
  });

  it("READ_STREAM returns appended records without moving git", () => {
    const { writer, reader, repo, repositoryId } = setup();
    const headBefore = repo.head("refs/heads/main");
    writer.append("read-1", {
      targetRepository: repositoryId,
      streamRef: "runs",
      entries: [{ eventId: "run-1", payload: { status: "ok" } }],
    });
    const slice = reader.readStream(repositoryId, "runs");
    expect(slice.cursor).toBe("1");
    expect(slice.records).toHaveLength(1);
    expect(slice.records[0]?.eventId).toBe("run-1");
    expect(slice.records[0]?.payload).toEqual({ status: "ok" });
    expect(repo.head("refs/heads/main")).toBe(headBefore);
    expect(reader.readStream(repositoryId, "empty").records).toEqual([]);
  });
});
