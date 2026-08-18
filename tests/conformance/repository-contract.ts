import { describe, expect, it } from "vitest";
import type { CommitChangeSet, Repository } from "../../src/index.ts";
import { IngressError } from "../../src/contracts/errors.ts";

export type RepositoryFactory = (repositoryId: string) => Repository;

function expectCode(fn: () => unknown, code: string): void {
  try {
    fn();
  } catch (error) {
    expect((error as { code?: string }).code).toBe(code);
    return;
  }
  throw new Error(`expected error code ${code} but nothing was thrown`);
}

function commit(
  repo: Repository,
  baseCommit: string,
  objectId: string,
  value: unknown,
  pathHint?: string,
): string {
  const changeSet: CommitChangeSet = {
    targetRepository: repo.repositoryId,
    targetRef: "refs/heads/main",
    baseCommit,
    expectedTargetCommit: baseCommit,
    operations: [{ op: "PUT", address: { kind: "Entity", objectId }, value, pathHint }],
  };
  return repo.applyCommit(changeSet);
}

/** Shared adapter-level conformance. Every Repository implementation invokes this suite. */
export function repositoryContract(adapterName: string, createRepository: RepositoryFactory): void {
  describe(`${adapterName} Repository contract`, () => {
    it("preserves object identity across path moves and pinned versions", () => {
      const repo = createRepository(`kr://conformance/${adapterName}/identity`);
      const root = repo.head("refs/heads/main");
      const first = commit(repo, root, "policy/P-1", { version: 1 }, "policies/P-1.json");
      const second = commit(repo, first, "policy/P-1", { version: 2 }, "archive/P-1.json");

      expect(repo.resolve("policy/P-1", second).pathHint).toBe("archive/P-1.json");
      expect(repo.read("policy/P-1", first).value).toEqual({ version: 1 });
      expect(repo.read("policy/P-1", second).value).toEqual({ version: 2 });
    });

    it("rejects stale ref preconditions", () => {
      const repo = createRepository(`kr://conformance/${adapterName}/cas`);
      const root = repo.head("refs/heads/main");
      commit(repo, root, "a", 1);
      expectCode(() => commit(repo, root, "b", 2), "NON_FAST_FORWARD");
    });

    it("enforces append event idempotency and cursor preconditions", () => {
      const repo = createRepository(`kr://conformance/${adapterName}/append`);
      const first = repo.append("events", [{ eventId: "e-1", payload: { value: 1 } }], "0");
      expect(repo.append("events", [{ eventId: "e-1", payload: { value: 1 } }], "1")).toEqual(first);
      expectCode(
        () => repo.append("events", [{ eventId: "e-2", payload: { value: 2 } }], "0"),
        "PRECONDITION_FAILED",
      );
      expectCode(
        () => repo.append("events", [{ eventId: "e-1", payload: { value: 2 } }], "1"),
        "EVENT_ID_CONFLICT",
      );
    });

    it("distinguishes an unresolved immutable version from an absent object", () => {
      const repo = createRepository(`kr://conformance/${adapterName}/version`);
      const root = repo.head("refs/heads/main");
      expectCode(() => repo.read("absent", root), "KNOWLEDGE_REF_UNRESOLVED");
      expectCode(() => repo.read("absent", "missing-version"), "VERSION_UNRESOLVED");
      expect(() => repo.head("refs/heads/missing")).toThrow(IngressError);
    });
  });
}
