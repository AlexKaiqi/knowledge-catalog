package index_test

import (
	"testing"

	"kc/index"
	"kc/reader"
)

type clauseProbeRetriever struct {
	probed []reader.SearchOp
}

func (r *clauseProbeRetriever) Probe(clause reader.SearchClause, _ reader.AccessSpec) index.Capability {
	r.probed = append(r.probed, clause.Op)
	guarantee := index.GuaranteeExact
	if clause.Op == reader.OpMatch {
		guarantee = index.GuaranteeSuperset
	}
	return index.Capability{Guarantee: guarantee, Coverage: 1}
}

func (*clauseProbeRetriever) Retrieve(index.RetrieveRequest) (index.CandidatePage, error) {
	return index.CandidatePage{Exhausted: true}, nil
}

func TestPlanProbesEveryClauseFragment(t *testing.T) {
	retriever := &clauseProbeRetriever{}
	spec := reader.AccessSpec{
		Repository: "kr://acme/public/core", Commit: "c1",
		Fields: []reader.AccessField{
			{FieldRef: reader.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
			{FieldRef: reader.FieldRef{Schema: "schema/t", Path: "db"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		},
	}
	request := reader.SearchOf(reader.SearchMATCH("runbook"), reader.SearchPREFIX("db", "prod"))
	plan, err := index.PlanRetrieval(retriever, nil, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Fragments) != 2 || len(retriever.probed) != 2 || plan.Fragments[0].Capability.Guarantee != index.GuaranteeSuperset || plan.Fragments[1].Capability.Guarantee != index.GuaranteeExact {
		t.Fatalf("plan=%#v probed=%v", plan, retriever.probed)
	}
}
