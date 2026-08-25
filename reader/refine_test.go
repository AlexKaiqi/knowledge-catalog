package reader_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func candidates() []reader.Candidate {
	return []reader.Candidate{
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
	refine := reader.Refine{}
	result := refine.Filter(candidates(), "refund timeout", reader.KeywordJudge("refund timeout"), nil)
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
	refine := reader.Refine{}
	budget := 2
	judge := func(c reader.Candidate, _ string) reader.FilterJudgment {
		if c.Ref.Object == "p2" {
			return reader.JudgmentUnknown
		}
		return reader.JudgmentMatch
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
	refine := reader.Refine{}
	result := refine.Rerank(candidates(), "refund timeout", reader.KeywordScorer("refund timeout"), nil)
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
	refine := reader.Refine{}
	topK := 1
	result := refine.Rerank(candidates(), "refund timeout", reader.KeywordScorer("refund timeout"), &topK)
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
	refine := reader.Refine{}
	topK := 2
	spec := reader.SemanticOperatorSpec{
		SpecRef:              "urn:semantic-spec:refund-rank",
		Revision:             3,
		Operator:             reader.OpSemanticRerank,
		Criterion:            "refund timeout",
		EvaluationProjection: &reader.EvaluationProjection{Fields: []string{"body"}},
		OutputContract:       reader.OutputContract{TopK: &topK, AllowTies: true, AllowUnjudged: true},
	}
	raw, err := refine.Run(spec, candidates(), nil, reader.KeywordScorer("refund timeout"))
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(reader.SemanticRerankResult)
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
	refine := reader.Refine{}
	topK := 1
	spec := reader.SemanticOperatorSpec{
		SpecRef:        "urn:semantic-spec:strict-rank",
		Revision:       1,
		Operator:       reader.OpSemanticRerank,
		Criterion:      "refund timeout",
		OutputContract: reader.OutputContract{TopK: &topK, AllowTies: true, AllowUnjudged: false},
	}
	_, err := refine.Run(spec, candidates(), nil, reader.KeywordScorer("refund timeout"))
	if err == nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("%v", err)
	}
}
