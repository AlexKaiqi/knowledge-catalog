package index_test

import (
	"kc/retrieval"
	"testing"

	"kc/index"
	"kc/knowledge/reader"
)

type clauseProbeRetriever struct {
	probed []retrieval.SearchOp
}

func (r *clauseProbeRetriever) Probe(clause retrieval.SearchClause, _ retrieval.AccessSpec) index.Capability {
	r.probed = append(r.probed, clause.Op)
	guarantee := index.GuaranteeExact
	if clause.Op == retrieval.OpMatch {
		guarantee = index.GuaranteeSuperset
	}
	return index.Capability{Guarantee: guarantee, Coverage: 1}
}

func (*clauseProbeRetriever) ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}

func (*clauseProbeRetriever) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	return index.CandidatePage{Exhausted: true}, nil
}

func TestPlanProbesEveryClauseFragment(t *testing.T) {
	retriever := &clauseProbeRetriever{}
	spec := retrieval.AccessSpec{
		Repository: "kr://acme/public/core", Commit: "c1",
		Fields: []retrieval.AccessField{
			{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
			{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "db"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		},
	}
	request := retrieval.SearchOf(retrieval.SearchMATCH("runbook"), retrieval.SearchPREFIX("db", "prod"))
	plan, err := index.PlanRetrieval(retriever, nil, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Fragments) != 2 || len(retriever.probed) != 2 || plan.Fragments[0].Capability.Guarantee != index.GuaranteeSuperset || plan.Fragments[1].Capability.Guarantee != index.GuaranteeExact {
		t.Fatalf("plan=%#v probed=%v", plan, retriever.probed)
	}
}

func TestPlanPreservesExpressionAndProbesEveryLeaf(t *testing.T) {
	retriever := &clauseProbeRetriever{}
	spec := retrieval.AccessSpec{
		Repository: "kr://acme/public/core", Commit: "c1",
		Fields: []retrieval.AccessField{
			{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "db"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "owner"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		},
	}
	request := retrieval.SearchWhere(retrieval.SearchAll(
		retrieval.SearchAny(
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
			retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
		),
		retrieval.SearchLeaf(retrieval.SearchEQ("owner", "alice")),
	))
	plan, err := index.PlanRetrieval(retriever, nil, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Search.Expression == nil || len(plan.Search.Expression.All) != 2 || plan.Composition.Guarantee != index.GuaranteeExact || len(plan.Fragments) != 3 || len(retriever.probed) != 3 {
		t.Fatalf("plan=%#v probed=%v", plan, retriever.probed)
	}
}

type leafOnlyRetriever struct{}

func (*leafOnlyRetriever) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}

func (*leafOnlyRetriever) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	return index.CandidatePage{Exhausted: true}, nil
}

func TestPlanFailsClosedWhenProviderDoesNotProveExpressionComposition(t *testing.T) {
	spec := retrieval.AccessSpec{
		Repository: "kr://acme/public/core", Commit: "c1",
		Fields: []retrieval.AccessField{{
			FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "db"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter},
		}},
	}
	request := retrieval.SearchWhere(retrieval.SearchAny(
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "tl")),
		retrieval.SearchLeaf(retrieval.SearchEQ("db", "prod")),
	))
	plan, err := index.PlanRetrieval(&leafOnlyRetriever{}, nil, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Composition.Guarantee != index.GuaranteeUnsupported {
		t.Fatalf("composition = %#v", plan.Composition)
	}
}
