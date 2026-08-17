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

/**
 * SemanticOperatorSpec — the frozen protocol object for a refinement operator
 * (whitepaper §23.3): Criterion, EvaluationProjection, ContextRefs, OutputContract.
 * The model/prompt/batch/cascade stay in the Provider; these fields are protocol.
 */
export type SemanticOperator = "SEMANTIC_FILTER" | "SEMANTIC_RERANK";

export interface EvaluationProjection {
  /** Dot-paths of the fields visible to the judge/scorer (fixed, no full-object peek). */
  readonly fields?: readonly string[];
}

export interface OutputContract {
  readonly topK?: number;
  readonly allowTies: boolean;
  readonly allowUnjudged: boolean;
}

export interface SemanticOperatorSpec {
  readonly specRef: string;
  readonly revision: number;
  readonly operator: SemanticOperator;
  readonly criterion: string;
  readonly evaluationProjection?: EvaluationProjection;
  readonly contextRefs?: readonly string[];
  readonly outputContract: OutputContract;
}
