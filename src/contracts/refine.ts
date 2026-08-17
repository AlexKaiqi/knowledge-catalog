/**
 * Semantic Refinement — optional, Ref-preserving Access operators (ADR-017).
 * Only SEMANTIC_FILTER (subset) and SEMANTIC_RERANK (RankGroup) are standardized.
 *
 * Hard boundary vs an Agent/Application:
 *  - input candidates are fixed BEFORE the call (no further SEARCH);
 *  - output refs are a subset of input refs (never create new refs);
 *  - no tool calls, no side effects;
 *  - FILTER uses MATCH/NO_MATCH/UNKNOWN, plus UNJUDGED for budget cut-off.
 */

import type { KnowledgeRef } from "./identity.ts";

export type FilterJudgment = "MATCH" | "NO_MATCH" | "UNKNOWN";

export interface Candidate {
  readonly ref: KnowledgeRef;
  readonly value: unknown;
}

export interface SemanticFilterResult {
  readonly matched: readonly KnowledgeRef[];
  readonly rejected: readonly KnowledgeRef[];
  readonly unknown: readonly KnowledgeRef[];
  readonly unjudged: readonly KnowledgeRef[];
  readonly complete: boolean;
  readonly truncationReason?: string;
}

export interface RankGroup {
  readonly rank: number;
  readonly refs: readonly KnowledgeRef[];
}

export interface SemanticRerankResult {
  readonly groups: readonly RankGroup[];
  readonly unjudged: readonly KnowledgeRef[];
  readonly complete: boolean;
  readonly truncationReason?: string;
}

export type FilterJudge = (candidate: Candidate, criterion: string) => FilterJudgment;
export type RerankScorer = (candidate: Candidate, criterion: string) => number;
