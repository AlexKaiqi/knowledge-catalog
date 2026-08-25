package elasticsearch

import (
	"testing"

	"kc/index"
	"kc/reader"
)

func TestElasticsearchProbeMVPSubset(t *testing.T) {
	engine := &esEngine{}
	spec := reader.AccessSpec{Fields: []reader.AccessField{
		{FieldRef: reader.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
		{FieldRef: reader.FieldRef{Schema: "schema/t", Path: "name"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		{FieldRef: reader.FieldRef{Schema: "schema/t", Path: "n"}, Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
	}}
	for _, clause := range []reader.SearchClause{
		reader.SearchMATCHMode("daily events", reader.MatchAllTerms),
		reader.SearchMATCHMode("daily events", reader.MatchAnyTerms),
		reader.SearchMATCHMode("daily events", reader.MatchPhrase),
		reader.SearchEQ("name", "customer.orders"),
		reader.SearchIN("name", "a", "b"),
		reader.SearchEXISTS("name"),
		reader.SearchMISSING("name"),
		reader.SearchPREFIX("name", "customer."),
	} {
		if capability := engine.Probe(clause, spec); capability.Guarantee != index.GuaranteeExact {
			t.Fatalf("%s: %#v", clause.Op, capability)
		}
	}
	for _, clause := range []reader.SearchClause{
		reader.SearchNEQ("name", "x"), reader.SearchRange(reader.OpGT, "n", "1"),
	} {
		if capability := engine.Probe(clause, spec); capability.Guarantee != index.GuaranteeUnsupported {
			t.Fatalf("%s must remain explicit unsupported: %#v", clause.Op, capability)
		}
	}
}
