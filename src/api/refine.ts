/**
 * SemanticRefine — reference implementation of SEM_FILTER / SEM_RERANK.
 * The judge/scorer is injected (a keyword rule now, a model provider later);
 * the protocol shape and Ref-preserving invariant are fixed here.
 */

import type {
  Candidate,
  EvaluationProjection,
  FilterJudge,
  RankGroup,
  RerankScorer,
  SemanticFilterResult,
  SemanticOperatorSpec,
  SemanticRerankResult,
} from "../contracts/refine.ts";
import { IngressError } from "../contracts/errors.ts";

/** Project a value to only the declared fields (EvaluationProjection fixes visibility). */
function projectFields(value: unknown, projection: EvaluationProjection): unknown {
  const fields = projection.fields ?? [];
  if (fields.length === 0) return value;
  if (value === null || typeof value !== "object" || Array.isArray(value)) return value;
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    const parts = f.split(".");
    let cur: unknown = value;
    for (const p of parts) {
      if (cur === null || typeof cur !== "object") {
        cur = undefined;
        break;
      }
      cur = (cur as Record<string, unknown>)[p];
    }
    out[f] = cur;
  }
  return out;
}

export class SemanticRefine {
  /** SEM_FILTER: partition input candidates into matched/rejected/unknown/unjudged. */
  filter(
    candidates: readonly Candidate[],
    criterion: string,
    judge: FilterJudge,
    budget?: number,
  ): SemanticFilterResult {
    const matched: Candidate["ref"][] = [];
    const rejected: Candidate["ref"][] = [];
    const unknown: Candidate["ref"][] = [];
    const unjudged: Candidate["ref"][] = [];
    let judged = 0;

    for (const c of candidates) {
      if (budget !== undefined && judged >= budget) {
        unjudged.push(c.ref);
        continue;
      }
      judged += 1;
      const j = judge(c, criterion);
      if (j === "MATCH") matched.push(c.ref);
      else if (j === "NO_MATCH") rejected.push(c.ref);
      else unknown.push(c.ref);
    }
    return {
      matched,
      rejected,
      unknown,
      unjudged,
      complete: unjudged.length === 0,
      truncationReason: unjudged.length ? "CANDIDATE_BUDGET" : undefined,
    };
  }

  /** SEM_RERANK: score then group ties into RankGroups (no fake probabilities). */
  rerank(
    candidates: readonly Candidate[],
    criterion: string,
    scorer: RerankScorer,
    topK?: number,
  ): SemanticRerankResult {
    const scored = candidates.map((c) => ({ ref: c.ref, score: scorer(c, criterion) }));
    scored.sort((a, b) => b.score - a.score);

    const groups: RankGroup[] = [];
    let current: Candidate["ref"][] = [];
    let currentScore: number | null = null;
    for (const s of scored) {
      if (currentScore !== s.score && currentScore !== null) {
        groups.push({ rank: groups.length + 1, refs: current });
        current = [];
      }
      currentScore = s.score;
      current.push(s.ref);
    }
    if (current.length) groups.push({ rank: groups.length + 1, refs: current });

    // topK truncation: keep top K refs across groups, the rest are unjudged.
    const unjudged: Candidate["ref"][] = [];
    let truncated = false;
    if (topK !== undefined) {
      const kept: RankGroup[] = [];
      let count = 0;
      for (const g of groups) {
        const remaining = topK - count;
        if (remaining <= 0) break;
        if (g.refs.length <= remaining) {
          kept.push(g);
          count += g.refs.length;
        } else {
          kept.push({ rank: g.rank, refs: g.refs.slice(0, remaining) });
          unjudged.push(...g.refs.slice(remaining));
          count += remaining;
          truncated = true;
          break;
        }
      }
      // refs beyond kept groups become unjudged
      const keptRefs = new Set(kept.flatMap((g) => g.refs));
      for (const g of groups) for (const r of g.refs) if (!keptRefs.has(r)) unjudged.push(r);
      return { groups: kept, unjudged, complete: !truncated, truncationReason: truncated ? "CANDIDATE_BUDGET" : undefined };
    }
    return { groups, unjudged: [], complete: true };
  }

  /**
   * Run a frozen SemanticOperatorSpec: project fields, dispatch by operator,
   * and apply the OutputContract (topK / allowUnjudged). Model/prompt stays in
   * the Provider; this is the protocol-level execution.
   */
  run(
    spec: SemanticOperatorSpec,
    candidates: readonly Candidate[],
    judge?: FilterJudge,
    scorer?: RerankScorer,
  ): SemanticFilterResult | SemanticRerankResult {
    const projected = spec.evaluationProjection
      ? candidates.map((c) => ({ ref: c.ref, value: projectFields(c.value, spec.evaluationProjection!) }))
      : candidates;

    if (spec.operator === "SEMANTIC_FILTER") {
      if (!judge) throw new IngressError("CAPABILITY_UNSATISFIED", "SEMANTIC_FILTER requires a judge");
      const result = this.filter(projected, spec.criterion, judge);
      if (!spec.outputContract.allowUnjudged && result.unjudged.length > 0) {
        throw new IngressError("CAPABILITY_UNSATISFIED", "unjudged results not allowed by output contract");
      }
      return result;
    }
    if (!scorer) throw new IngressError("CAPABILITY_UNSATISFIED", "SEMANTIC_RERANK requires a scorer");
    const result = this.rerank(projected, spec.criterion, scorer, spec.outputContract.topK);
    if (!spec.outputContract.allowUnjudged && result.unjudged.length > 0) {
      throw new IngressError("CAPABILITY_UNSATISFIED", "unjudged results not allowed by output contract");
    }
    return result;
  }
}

/** Rule judge: MATCH iff any keyword from the criterion appears in the value's text. */
export function keywordJudge(criterion: string): FilterJudge {
  const keywords = criterion.toLowerCase().split(/\s+/).filter((w) => w.length > 1);
  return (candidate) => {
    const text = JSON.stringify(candidate.value).toLowerCase();
    return keywords.some((k) => text.includes(k)) ? "MATCH" : "NO_MATCH";
  };
}

/** Rule scorer: number of criterion keywords found in the value's text. */
export function keywordScorer(criterion: string): RerankScorer {
  const keywords = criterion.toLowerCase().split(/\s+/).filter((w) => w.length > 1);
  return (candidate) => {
    const text = JSON.stringify(candidate.value).toLowerCase();
    return keywords.reduce((n, k) => n + (text.includes(k) ? 1 : 0), 0);
  };
}
