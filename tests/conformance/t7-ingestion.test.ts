import { afterEach, describe, expect, it } from "vitest";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import type { KnowledgeValue } from "../../src/index.ts";
import { groundingCitation, ingest, reconcile } from "../../src/api/ingestion.ts";
import { canonicalDigest } from "../../src/digest.ts";

const tmpDirs: string[] = [];
afterEach(() => {
  for (const d of tmpDirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

describe("Ingestion & Grounding (reference)", () => {
  it("ingest scans a directory into a SOURCE ChangeSet (one file = one object)", () => {
    const dir = mkdtempSync(path.join(tmpdir(), "kc-ingest-"));
    tmpDirs.push(dir);
    writeFileSync(path.join(dir, "policy.md"), "# Policy\nproduction requires a runbook");
    mkdirSync(path.join(dir, "notes"), { recursive: true });
    writeFileSync(path.join(dir, "notes", "oncall.txt"), "check freeze window");

    const preview = ingest(dir, "kr://acme/public/core", "P0");
    expect(preview.files).toHaveLength(2);
    expect(preview.changeSet.operations).toHaveLength(2);
    expect(preview.changeSet.provenance?.originKind).toBe("SOURCE");
    expect(preview.changeSet.operations[0]?.op).toBe("PUT");
  });

  it("reconcile set-diffs a snapshot against current state", () => {
    const snapshot = new Map<string, unknown>([
      ["a", { v: 1 }], // unchanged
      ["b", { v: 2 }], // updated
      ["c", { v: 3 }], // added
    ]);
    const current = new Map<string, string>([
      ["a", canonicalDigest({ v: 1 })],
      ["b", "stale-digest"],
      ["d", "will-be-removed"],
    ]);

    const preview = reconcile(snapshot, current, "kr://acme/public/core", "P0");
    expect(preview.summary.added).toBe(1); // c
    expect(preview.summary.updated).toBe(1); // b
    expect(preview.summary.removed).toBe(1); // d
  });

  it("groundingCitation projects a KnowledgeValue with pinned ref + provenance", () => {
    const value: KnowledgeValue = {
      knowledgeRef: { repository: "kr://acme/public/core", object: "policy/P-103" },
      repository: "kr://acme/public/core",
      commit: "abc123",
      address: { kind: "Entity", objectId: "policy/P-103" },
      value: { statement: "v1" },
      provenance: { originKind: "DEFINITION", actorRef: "core-council", sourceRefs: ["handbook-v1"] },
    };
    const citation = groundingCitation(value);
    expect(citation.pinnedRef).toBe("kc://acme/public/core@abc123/policy/P-103");
    expect(citation.provenanceSummary?.originKind).toBe("DEFINITION");
    expect(citation.provenanceSummary?.actorRef).toBe("core-council");
  });
});
