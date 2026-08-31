package retrieval_test

import (
	"context"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

type rerankerFunc func(context.Context, retrieval.RerankRequest) (retrieval.RerankProviderResult, error)

func (f rerankerFunc) Rerank(ctx context.Context, request retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
	return f(ctx, request)
}

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
	if !result.Complete || len(result.Unjudged) != 0 || len(result.NotSelected) != 2 {
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
	if !kept["p1"] || !kept["p3"] || len(result.NotSelected) != 1 || result.NotSelected[0].Object != "p2" || !result.Complete {
		t.Fatal(result)
	}
}

func TestT10TopKSelectionIsNotUnjudged(t *testing.T) {
	refine := retrieval.Refine{}
	topK := 1
	spec := retrieval.SemanticOperatorSpec{
		SpecRef:        "urn:semantic-spec:strict-rank",
		Revision:       1,
		Operator:       retrieval.OpSemanticRerank,
		Criterion:      "refund timeout",
		OutputContract: retrieval.OutputContract{TopK: &topK, AllowTies: true, AllowUnjudged: false},
	}
	raw, err := refine.Run(spec, candidates(), nil, retrieval.KeywordScorer("refund timeout"))
	if err != nil {
		t.Fatal(err)
	}
	result := raw.(retrieval.SemanticRerankResult)
	if !result.Complete || len(result.Unjudged) != 0 || len(result.NotSelected) != 1 {
		t.Fatalf("topK is output selection, not provider budget: %#v", result)
	}
}

func TestExecuteRerankProjectsFieldsPreservesRefsAndKeepsBoundaryTies(t *testing.T) {
	topK := 1
	request := retrieval.RerankRequest{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:refund-rank", Revision: 3, Operator: retrieval.OpSemanticRerank,
			Criterion: "refund timeout", EvaluationProjection: &retrieval.EvaluationProjection{Fields: []string{"body"}},
			OutputContract: retrieval.OutputContract{TopK: &topK, AllowTies: true},
		},
		Candidates: []retrieval.Candidate{
			{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p1"}, Value: map[string]any{"body": "refund", "secret": "hidden"}, OriginalRank: 1, RetrievalEvidence: []retrieval.LaneEvidence{{Provider: "opensearch", Lane: "lexical", LocalRank: 1}}},
			{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p2"}, Value: map[string]any{"body": "refund timeout", "secret": "hidden"}},
			{Ref: knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "p3"}, Value: map[string]any{"body": "deploy", "secret": "hidden"}},
		},
	}
	provider := rerankerFunc(func(_ context.Context, got retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
		for _, candidate := range got.Candidates {
			value := candidate.Value.(map[string]any)
			if _, leaked := value["secret"]; leaked || len(value) != 1 {
				t.Fatalf("evaluation projection leaked fields: %#v", value)
			}
		}
		return retrieval.RerankProviderResult{
			Groups: []retrieval.RankGroup{
				{Rank: 1, Refs: []knowledge.KnowledgeRef{got.Candidates[1].Ref, got.Candidates[0].Ref}},
				{Rank: 2, Refs: []knowledge.KnowledgeRef{got.Candidates[2].Ref}},
			},
			Provider: "llm-runtime", Model: "judge", ModelRevision: "v3",
		}, nil
	})
	result, err := retrieval.ExecuteRerank(context.Background(), provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Refs) != 2 || len(result.NotSelected) != 1 || !result.Complete {
		t.Fatalf("boundary tie/output semantics: %#v", result)
	}
	if result.Evidence == nil || result.Evidence.CandidateDigest == "" || result.Evidence.CandidateCount != 3 || result.Evidence.JudgedCount != 3 {
		t.Fatalf("evidence: %#v", result.Evidence)
	}
	if result.Evidence.ProjectedInputBytes <= 0 || result.Evidence.Candidates[0].OriginalRank != 1 || result.Evidence.Candidates[0].RetrievalEvidence[0].Lane != "lexical" {
		t.Fatalf("candidate origin evidence: %#v", result.Evidence)
	}
}

func TestExecuteRerankRejectsOversizedModelVisibleInputBeforeProvider(t *testing.T) {
	calls := 0
	provider := rerankerFunc(func(context.Context, retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
		calls++
		return retrieval.RerankProviderResult{}, nil
	})
	request := retrieval.RerankRequest{
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:budget", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{},
		},
		Candidates: []retrieval.Candidate{{
			Ref:   knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: "oversized"},
			Value: map[string]any{"body": strings.Repeat("x", retrieval.MaxRerankCandidateBytes+1)},
		}},
	}
	if _, err := retrieval.ExecuteRerank(context.Background(), provider, request); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("error = %v", err)
	}
	if calls != 0 {
		t.Fatal("oversized input reached provider")
	}
}

func TestExecuteRerankRejectsOversizedAggregateWindowBeforeProvider(t *testing.T) {
	calls := 0
	provider := rerankerFunc(func(context.Context, retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
		calls++
		return retrieval.RerankProviderResult{}, nil
	})
	request := retrieval.RerankRequest{
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:window-budget", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{},
		},
	}
	for i := 0; i < 5; i++ {
		request.Candidates = append(request.Candidates, retrieval.Candidate{
			Ref:   knowledge.KnowledgeRef{Repository: "kr://acme/public/core", Object: knowledge.ObjectID("large-" + string(rune('a'+i)))},
			Value: map[string]any{"body": strings.Repeat("x", 30<<10)},
		})
	}
	if _, err := retrieval.ExecuteRerank(context.Background(), provider, request); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("error = %v", err)
	}
	if calls != 0 {
		t.Fatal("oversized aggregate input reached provider")
	}
}

func TestExecuteRerankRejectsDishonestOrIncompleteProviderOutput(t *testing.T) {
	request := retrieval.RerankRequest{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:test", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{AllowUnjudged: true},
		},
		Candidates: candidates(),
	}
	for name, groups := range map[string][]retrieval.RankGroup{
		"unknown":   {{Rank: 1, Refs: []knowledge.KnowledgeRef{{Repository: "kr://acme/public/core", Object: "other"}}}},
		"omitted":   {{Rank: 1, Refs: []knowledge.KnowledgeRef{request.Candidates[0].Ref, request.Candidates[1].Ref}}},
		"duplicate": {{Rank: 1, Refs: []knowledge.KnowledgeRef{request.Candidates[0].Ref, request.Candidates[0].Ref, request.Candidates[1].Ref, request.Candidates[2].Ref}}},
	} {
		t.Run(name, func(t *testing.T) {
			provider := rerankerFunc(func(context.Context, retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
				return retrieval.RerankProviderResult{Groups: groups, Provider: "test", Model: "test"}, nil
			})
			if _, err := retrieval.ExecuteRerank(context.Background(), provider, request); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExecuteRerankFailsClosedForUnjudgedWhenContractForbidsIt(t *testing.T) {
	request := retrieval.RerankRequest{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:strict", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{AllowUnjudged: false},
		},
		Candidates: candidates(),
	}
	provider := rerankerFunc(func(_ context.Context, got retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
		return retrieval.RerankProviderResult{
			Groups:   []retrieval.RankGroup{{Rank: 1, Refs: []knowledge.KnowledgeRef{got.Candidates[0].Ref, got.Candidates[1].Ref}}},
			Unjudged: []knowledge.KnowledgeRef{got.Candidates[2].Ref}, Provider: "test", Model: "test",
		}, nil
	})
	if _, err := retrieval.ExecuteRerank(context.Background(), provider, request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteRerankFailsClosedWithoutProvider(t *testing.T) {
	request := retrieval.RerankRequest{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:test", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", OutputContract: retrieval.OutputContract{},
		},
		Candidates: candidates(),
	}
	if _, err := retrieval.ExecuteRerank(context.Background(), nil, request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteRerankExplicitEmptyProjectionExposesNoValueOrObservationFields(t *testing.T) {
	request := retrieval.RerankRequest{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/public/core": "c1"}},
		Spec: retrieval.SemanticOperatorSpec{
			SpecRef: "urn:semantic-spec:minimal", Revision: 1, Operator: retrieval.OpSemanticRerank,
			Criterion: "relevance", EvaluationProjection: &retrieval.EvaluationProjection{Fields: []string{}},
			OutputContract: retrieval.OutputContract{},
		},
		Candidates: candidates(),
	}
	request.Candidates[0].Observations = []knowledge.UnitObservation{{Address: knowledge.Address{ObjectID: "p1"}}}
	provider := rerankerFunc(func(_ context.Context, got retrieval.RerankRequest) (retrieval.RerankProviderResult, error) {
		refs := make([]knowledge.KnowledgeRef, len(got.Candidates))
		for i, candidate := range got.Candidates {
			if value := candidate.Value.(map[string]any); len(value) != 0 || len(candidate.Observations) != 0 {
				t.Fatalf("empty projection/provider metadata leak: %#v", candidate)
			}
			refs[i] = candidate.Ref
		}
		return retrieval.RerankProviderResult{
			Groups: []retrieval.RankGroup{{Rank: 1, Refs: refs}}, Provider: "test", Model: "test",
		}, nil
	})
	if _, err := retrieval.ExecuteRerank(context.Background(), provider, request); err != nil {
		t.Fatal(err)
	}
}
