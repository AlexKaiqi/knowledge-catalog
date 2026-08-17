import { describe, expect, it } from "vitest";
import type { Candidate, SemanticRerankResult } from "../../src/contracts/refine.ts";
import { keywordJudge, keywordScorer, SemanticRefine } from "../../src/api/refine.ts";

const candidates: Candidate[] = [
  { ref: { repository: "kr://acme/public/core", object: "p1" }, value: { body: "refund timeout diagnosis" } },
  { ref: { repository: "kr://acme/public/core", object: "p2" }, value: { body: "deployment checklist" } },
  { ref: { repository: "kr://acme/public/core", object: "p3" }, value: { body: "refund SLA and timeout policy" } },
];

describe("SemanticRefine (Phase 4)", () => {
  it("SEM_FILTER partitions into matched/rejected and stays Ref-preserving", () => {
    const refine = new SemanticRefine();
    const result = refine.filter(candidates, "refund timeout", keywordJudge("refund timeout"));

    expect(result.matched.map((r) => r.object).sort()).toEqual(["p1", "p3"]);
    expect(result.rejected.map((r) => r.object)).toEqual(["p2"]);
    expect(result.unknown).toEqual([]);
    expect(result.complete).toBe(true);

    // Ref-preserving: matched ∪ rejected ∪ unknown ∪ unjudged == input refs
    const all = [...result.matched, ...result.rejected, ...result.unknown, ...result.unjudged];
    expect(new Set(all)).toEqual(new Set(candidates.map((c) => c.ref)));
  });

  it("SEM_FILTER separates UNKNOWN from NO_MATCH, and UNJUDGED on budget cut-off", () => {
    const refine = new SemanticRefine();
    // judge returns UNKNOWN for p2
    const judge = (c: Candidate, _criterion: string) => (c.ref.object === "p2" ? "UNKNOWN" : "MATCH");
    const result = refine.filter(candidates, "x", judge, 2); // budget 2 -> p3 unjudged

    expect(result.matched.map((r) => r.object)).toEqual(["p1"]);
    expect(result.unknown.map((r) => r.object)).toEqual(["p2"]);
    expect(result.unjudged.map((r) => r.object)).toEqual(["p3"]);
    expect(result.complete).toBe(false);
    expect(result.truncationReason).toBe("CANDIDATE_BUDGET");
  });

  it("SEM_RERANK groups ties into RankGroups without fake probabilities", () => {
    const refine = new SemanticRefine();
    const result = refine.rerank(candidates, "refund timeout", keywordScorer("refund timeout"));

    // p3 hits both "refund" and "timeout" (score 2) -> rank 1; p1 hits both (score 2)? check
    // p1 "refund timeout diagnosis" -> refund+timeout = 2; p3 "refund SLA and timeout policy" -> 2; p2 = 0
    expect(result.groups[0]?.refs.map((r) => r.object).sort()).toEqual(["p1", "p3"]);
    expect(result.groups[0]?.rank).toBe(1);
    expect(result.groups[1]?.refs.map((r) => r.object)).toEqual(["p2"]);
    expect(result.complete).toBe(true);
  });

  it("SEM_RERANK honors topK and marks the rest unjudged", () => {
    const refine = new SemanticRefine();
    const result = refine.rerank(candidates, "refund timeout", keywordScorer("refund timeout"), 1);
    expect(result.complete).toBe(false);
    expect(result.unjudged.length).toBeGreaterThan(0);
    const kept = result.groups.flatMap((g) => g.refs);
    expect(kept.length).toBe(1);
    // Ref-preserving: kept ∪ unjudged == input
    expect(new Set([...kept, ...result.unjudged])).toEqual(new Set(candidates.map((c) => c.ref)));
  });

  it("runs a frozen SemanticOperatorSpec with field projection + output contract", () => {
    const refine = new SemanticRefine();
    const spec = {
      specRef: "urn:semantic-spec:refund-rank",
      revision: 3,
      operator: "SEMANTIC_RERANK" as const,
      criterion: "refund timeout",
      evaluationProjection: { fields: ["body"] },
      outputContract: { topK: 2, allowTies: true, allowUnjudged: true },
    };
    const result = refine.run(spec, candidates, undefined, keywordScorer("refund timeout")) as SemanticRerankResult;
    // topK 2 keeps p1+p3 (score 2); p2 (score 0) is unjudged
    const kept = result.groups.flatMap((g) => g.refs).map((r) => r.object).sort();
    expect(kept).toEqual(["p1", "p3"]);
    expect(result.unjudged.map((r) => r.object)).toEqual(["p2"]);
  });

  it("rejects unjudged results when output contract disallows them", () => {
    const refine = new SemanticRefine();
    const spec = {
      specRef: "urn:semantic-spec:strict-rank",
      revision: 1,
      operator: "SEMANTIC_RERANK" as const,
      criterion: "refund timeout",
      outputContract: { topK: 1, allowTies: true, allowUnjudged: false },
    };
    // topK 1 truncates -> unjudged exists, but allowUnjudged=false rejects
    expect(() => refine.run(spec, candidates, undefined, keywordScorer("refund timeout"))).toThrow(/unjudged/);
  });
});
