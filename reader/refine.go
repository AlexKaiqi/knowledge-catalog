package reader

import (
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

// Refine is optional Semantic Refinement (design 7.9). Ref-preserving:
// SEMANTIC_FILTER and SEMANTIC_RERANK. Not a base read. Not Derivation.

type FilterJudgment string

const (
	JudgmentMatch   FilterJudgment = "MATCH"
	JudgmentNoMatch FilterJudgment = "NO_MATCH"
	JudgmentUnknown FilterJudgment = "UNKNOWN"
)

type Candidate struct {
	Ref   knowledge.KnowledgeRef `json:"ref"`
	Value any                    `json:"value"`
}

type SemanticFilterResult struct {
	Matched          []knowledge.KnowledgeRef `json:"matched"`
	Rejected         []knowledge.KnowledgeRef `json:"rejected"`
	Unknown          []knowledge.KnowledgeRef `json:"unknown"`
	Unjudged         []knowledge.KnowledgeRef `json:"unjudged"`
	Complete         bool                     `json:"complete"`
	TruncationReason string                   `json:"truncationReason,omitempty"`
}

type RankGroup struct {
	Rank int                      `json:"rank"`
	Refs []knowledge.KnowledgeRef `json:"refs"`
}

type SemanticRerankResult struct {
	Groups           []RankGroup              `json:"groups"`
	Unjudged         []knowledge.KnowledgeRef `json:"unjudged"`
	Complete         bool                     `json:"complete"`
	TruncationReason string                   `json:"truncationReason,omitempty"`
}

type FilterJudge func(candidate Candidate, criterion string) FilterJudgment
type RerankScorer func(candidate Candidate, criterion string) float64

type SemanticOperator string

const (
	OpSemanticFilter SemanticOperator = "SEMANTIC_FILTER"
	OpSemanticRerank SemanticOperator = "SEMANTIC_RERANK"
)

// EvaluationProjection is the Refine judge/scorer field whitelist (design 7.3).
// Not an Access Projection and not AspectSelector.
type EvaluationProjection struct {
	Fields []string `json:"fields,omitempty"`
}

type OutputContract struct {
	TopK          *int `json:"topK,omitempty"`
	AllowTies     bool `json:"allowTies"`
	AllowUnjudged bool `json:"allowUnjudged"`
}

type SemanticOperatorSpec struct {
	SpecRef              string                `json:"specRef"`
	Revision             int                   `json:"revision"`
	Operator             SemanticOperator      `json:"operator"`
	Criterion            string                `json:"criterion"`
	EvaluationProjection *EvaluationProjection `json:"evaluationProjection,omitempty"`
	ContextRefs          []string              `json:"contextRefs,omitempty"`
	OutputContract       OutputContract        `json:"outputContract"`
}

func projectFields(value any, projection EvaluationProjection) any {
	if len(projection.Fields) == 0 {
		return value
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := map[string]any{}
	for _, f := range projection.Fields {
		parts := strings.Split(f, ".")
		var cur any = obj
		for _, p := range parts {
			m, ok := cur.(map[string]any)
			if !ok {
				cur = nil
				break
			}
			cur = m[p]
		}
		out[f] = cur
	}
	return out
}

type Refine struct{}

func (Refine) Filter(candidates []Candidate, criterion string, judge FilterJudge, budget *int) SemanticFilterResult {
	matched := []knowledge.KnowledgeRef{}
	rejected := []knowledge.KnowledgeRef{}
	unknown := []knowledge.KnowledgeRef{}
	unjudged := []knowledge.KnowledgeRef{}
	judged := 0
	for _, c := range candidates {
		if budget != nil && judged >= *budget {
			unjudged = append(unjudged, c.Ref)
			continue
		}
		judged++
		switch judge(c, criterion) {
		case JudgmentMatch:
			matched = append(matched, c.Ref)
		case JudgmentNoMatch:
			rejected = append(rejected, c.Ref)
		default:
			unknown = append(unknown, c.Ref)
		}
	}
	reason := ""
	if len(unjudged) > 0 {
		reason = "CANDIDATE_BUDGET"
	}
	return SemanticFilterResult{
		Matched: matched, Rejected: rejected, Unknown: unknown, Unjudged: unjudged,
		Complete: len(unjudged) == 0, TruncationReason: reason,
	}
}

func (Refine) Rerank(candidates []Candidate, criterion string, scorer RerankScorer, topK *int) SemanticRerankResult {
	type scored struct {
		ref   knowledge.KnowledgeRef
		score float64
	}
	items := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		items = append(items, scored{ref: c.Ref, score: scorer(c, criterion)})
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].score > items[i].score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	var groups []RankGroup
	var current []knowledge.KnowledgeRef
	var currentScore *float64
	for _, s := range items {
		if currentScore != nil && *currentScore != s.score {
			groups = append(groups, RankGroup{Rank: len(groups) + 1, Refs: current})
			current = nil
		}
		score := s.score
		currentScore = &score
		current = append(current, s.ref)
	}
	if len(current) > 0 {
		groups = append(groups, RankGroup{Rank: len(groups) + 1, Refs: current})
	}
	if topK == nil {
		return SemanticRerankResult{Groups: groups, Unjudged: []knowledge.KnowledgeRef{}, Complete: true}
	}
	var kept []RankGroup
	var unjudged []knowledge.KnowledgeRef
	count := 0
	truncated := false
	for _, g := range groups {
		remaining := *topK - count
		if remaining <= 0 {
			break
		}
		if len(g.Refs) <= remaining {
			kept = append(kept, g)
			count += len(g.Refs)
			continue
		}
		kept = append(kept, RankGroup{Rank: g.Rank, Refs: g.Refs[:remaining]})
		unjudged = append(unjudged, g.Refs[remaining:]...)
		count += remaining
		truncated = true
		break
	}
	keptRefs := map[knowledge.KnowledgeRef]struct{}{}
	for _, g := range kept {
		for _, ref := range g.Refs {
			keptRefs[ref] = struct{}{}
		}
	}
	for _, g := range groups {
		for _, ref := range g.Refs {
			if _, ok := keptRefs[ref]; !ok {
				already := false
				for _, u := range unjudged {
					if u == ref {
						already = true
						break
					}
				}
				if !already {
					unjudged = append(unjudged, ref)
				}
			}
		}
	}
	reason := ""
	if truncated {
		reason = "CANDIDATE_BUDGET"
	}
	return SemanticRerankResult{Groups: kept, Unjudged: unjudged, Complete: !truncated, TruncationReason: reason}
}

func (s Refine) Run(spec SemanticOperatorSpec, candidates []Candidate, judge FilterJudge, scorer RerankScorer) (any, error) {
	projected := candidates
	if spec.EvaluationProjection != nil {
		projected = make([]Candidate, len(candidates))
		for i, c := range candidates {
			projected[i] = Candidate{Ref: c.Ref, Value: projectFields(c.Value, *spec.EvaluationProjection)}
		}
	}
	if spec.Operator == OpSemanticFilter {
		if judge == nil {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SEMANTIC_FILTER requires a judge")
		}
		result := s.Filter(projected, spec.Criterion, judge, nil)
		if !spec.OutputContract.AllowUnjudged && len(result.Unjudged) > 0 {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unjudged results not allowed by output contract")
		}
		return result, nil
	}
	if scorer == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "SEMANTIC_RERANK requires a scorer")
	}
	result := s.Rerank(projected, spec.Criterion, scorer, spec.OutputContract.TopK)
	if !spec.OutputContract.AllowUnjudged && len(result.Unjudged) > 0 {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unjudged results not allowed by output contract")
	}
	return result, nil
}

func KeywordJudge(criterion string) FilterJudge {
	keywords := keywordList(criterion)
	return func(candidate Candidate, _ string) FilterJudgment {
		text := strings.ToLower(mustJSON(candidate.Value))
		for _, k := range keywords {
			if strings.Contains(text, k) {
				return JudgmentMatch
			}
		}
		return JudgmentNoMatch
	}
}

func KeywordScorer(criterion string) RerankScorer {
	keywords := keywordList(criterion)
	return func(candidate Candidate, _ string) float64 {
		text := strings.ToLower(mustJSON(candidate.Value))
		n := 0.0
		for _, k := range keywords {
			if strings.Contains(text, k) {
				n++
			}
		}
		return n
	}
}

func keywordList(criterion string) []string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(criterion)) {
		if len(w) > 1 {
			out = append(out, w)
		}
	}
	return out
}

func mustJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(b)
}
