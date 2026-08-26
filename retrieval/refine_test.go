package retrieval_test

import (
	"kc/retrieval"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

func candidates() []retrieval.Candidate {
	return []retrieval.Candidate{
		{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p1"}, Value: map[string]any{"body": "refund timeout diagnosis"}},
		{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p2"}, Value: map[string]any{"body": "deployment checklist"}},
		{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p3"}, Value: map[string]any{"body": "refund SLA and timeout policy"}},
	}
}

func objects(refs []knowledge.KnowledgeRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = string(r.Object)
	}
	return out
}

func TestT10Filter(t *testing.T) {
	refine := retrieval.Refine{}
	result := refine.Filter(candidates(), "refund timeout", retrieval.KeywordJudge("refund timeout"), nil)
	got := map[string]bool{}
	for _, r := range result.Matched {
		got[string(r.Object)] = true
	}
	if !got["p1"] || !got["p3"] || len(result.Matched) != 2 {
		t.Fatal(objects(result.Matched))
	}
	if len(result.Rejected) != 1 || result.Rejected[0].Object != "p2" {
		t.Fatal(objects(result.Rejected))
	}
	if !result.Complete {
		t.Fatal("expected complete")
	}
}

func TestT10UnknownAndBudget(t *testing.T) {
	refine := retrieval.Refine{}
	budget := 2
	judge := func(c retrieval.Candidate, _ string) retrieval.FilterJudgment {
		if c.Ref.Object == "p2" {
			return retrieval.JudgmentUnknown
		}
		return retrieval.JudgmentMatch
	}
	result := refine.Filter(candidates(), "x", judge, &budget)
	if objects(result.Matched)[0] != "p1" || objects(result.Unknown)[0] != "p2" || objects(result.Unjudged)[0] != "p3" {
		t.Fatalf("%v %v %v", objects(result.Matched), objects(result.Unknown), objects(result.Unjudged))
	}
	if result.Complete || result.TruncationReason != "CANDIDATE_BUDGET" {
		t.Fatal(result)
	}
}

func TestT10RerankTies(t *testing.T) {
	refine := retrieval.Refine{}
	result := refine.Rerank(candidates(), "refund timeout", retrieval.KeywordScorer("refund timeout"), nil)
	got := map[string]bool{}
	for _, r := range result.Groups[0].Refs {
		got[string(r.Object)] = true
	}
	if !got["p1"] || !got["p3"] || result.Groups[0].Rank != 1 {
		t.Fatal(result.Groups[0])
	}
	if result.Groups[1].Refs[0].Object != "p2" || !result.Complete {
		t.Fatal(result.Groups[1])
	}
}

func TestT10RerankTopK(t *testing.T) {
	refine := retrieval.Refine{}
	topK := 1
	result := refine.Rerank(candidates(), "refund timeout", retrieval.KeywordScorer("refund timeout"), &topK)
	if result.Complete || len(result.Unjudged) == 0 {
		t.Fatal(result)
	}
	kept := 0
	for _, g := range result.Groups {
		kept += len(g.Refs)
	}
	if kept != 1 {
		t.Fatal(kept)
	}
}

func TestT10FrozenSpec(t *testing.T) {
	refine := retrieval.Refine{}
	topK := 2
	spec := retrieval.SemanticOperatorSpec{
		SpecRef:              "urn:semantic-spec:refund-rank",
		Revision:             3,
		Operator:             retrieval.OpSemanticRerank,
		Criterion:            "refund timeout",
		EvaluationProjection: &retrieval.EvaluationProjection{Fields: []string{"body"}},
		OutputContract:       retrieval.OutputContract{TopK: &topK, AllowTies: true, AllowUnjudged: true},
	}
	raw, err := refine.Run(spec, candidates(), nil, retrieval.KeywordScorer("refund timeout"))
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(retrieval.SemanticRerankResult)
	kept := map[string]bool{}
	for _, g := range result.Groups {
		for _, r := range g.Refs {
			kept[string(r.Object)] = true
		}
	}
	if !kept["p1"] || !kept["p3"] || result.Unjudged[0].Object != "p2" {
		t.Fatal(result)
	}
}

func TestT10RejectUnjudged(t *testing.T) {
	refine := retrieval.Refine{}
	topK := 1
	spec := retrieval.SemanticOperatorSpec{
		SpecRef:        "urn:semantic-spec:strict-rank",
		Revision:       1,
		Operator:       retrieval.OpSemanticRerank,
		Criterion:      "refund timeout",
		OutputContract: retrieval.OutputContract{TopK: &topK, AllowTies: true, AllowUnjudged: false},
	}
	_, err := refine.Run(spec, candidates(), nil, retrieval.KeywordScorer("refund timeout"))
	if err == nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("%v", err)
	}
}
