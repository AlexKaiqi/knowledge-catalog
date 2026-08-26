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
