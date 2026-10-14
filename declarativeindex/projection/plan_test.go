package projection_test

import (
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"testing"

	index "kc/declarativeindex/projection"
	"kc/object/reader"
)

type clauseProbeRetriever struct {
	probed []retrieval.SearchOp
}

func (r *clauseProbeRetriever) Probe(clause retrieval.SearchClause, _ retrieval.AccessSpec) ports.Capability {
	r.probed = append(r.probed, clause.Op)
	guarantee := ports.GuaranteeExact
	if clause.Op == retrieval.OpMatch {
		guarantee = ports.GuaranteeSuperset
	}
	return ports.Capability{Guarantee: guarantee, Coverage: 1}
}

func (*clauseProbeRetriever) ProbeExpression(retrieval.SearchExpr, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeExact, Coverage: 1}
}

func (*clauseProbeRetriever) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	return ports.CandidatePage{Exhausted: true}, nil
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
	if len(plan.Fragments) != 2 || len(retriever.probed) != 2 || plan.Fragments[0].Capability.Guarantee != ports.GuaranteeSuperset || plan.Fragments[1].Capability.Guarantee != ports.GuaranteeExact {
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
	if plan.Search.Expression == nil || len(plan.Search.Expression.All) != 2 || plan.Composition.Guarantee != ports.GuaranteeExact || len(plan.Fragments) != 3 || len(retriever.probed) != 3 {
		t.Fatalf("plan=%#v probed=%v", plan, retriever.probed)
	}
}

type leafOnlyRetriever struct{}

func (*leafOnlyRetriever) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeExact, Coverage: 1}
}

func (*leafOnlyRetriever) Retrieve(ports.RetrieveRequest) (ports.CandidatePage, error) {
	return ports.CandidatePage{Exhausted: true}, nil
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
	if plan.Composition.Guarantee != ports.GuaranteeUnsupported {
		t.Fatalf("composition = %#v", plan.Composition)
	}
}
