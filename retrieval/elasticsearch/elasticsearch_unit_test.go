package elasticsearch

import (
	"fmt"
	"kc/retrieval"
	"strings"
	"testing"

	"kc/index"
	"kc/knowledge/reader"
)

func TestOpenSearchProbeTypedSubset(t *testing.T) {
	engine := &esEngine{}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "note"}, Type: "string", Access: []reader.AccessHint{reader.HintText}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "name"}, Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
		{FieldRef: retrieval.FieldRef{Schema: "schema/t", Path: "n"}, Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
	}}
	for _, clause := range []retrieval.SearchClause{
		retrieval.SearchMATCHMode("daily events", retrieval.MatchAllTerms),
		retrieval.SearchMATCHMode("daily events", retrieval.MatchAnyTerms),
		retrieval.SearchMATCHMode("daily events", retrieval.MatchPhrase),
		retrieval.SearchEQ("name", "customer.orders"),
		retrieval.SearchIN("name", "a", "b"),
		retrieval.SearchEXISTS("name"),
		retrieval.SearchMISSING("name"),
		retrieval.SearchPREFIX("name", "customer."),
		retrieval.SearchNEQ("name", "x"),
		retrieval.SearchRange(retrieval.OpGT, "n", "1"),
	} {
		if capability := engine.Probe(clause, spec); capability.Guarantee != index.GuaranteeExact {
			t.Fatalf("%s: %#v", clause.Op, capability)
		}
	}
	if capability := engine.Probe(retrieval.SearchSORT("name", "asc"), spec); capability.Guarantee != index.GuaranteeUnsupported {
		t.Fatalf("SORT must remain unsupported until multi-value reduction is declared: %#v", capability)
	}
}

func TestOpenSearchMissingRequiresApplicability(t *testing.T) {
	clause := retrieval.SearchClause{Op: retrieval.OpMissing, Path: "schema/t\x1f\x1fname"}
	query, _, err := osClause(clause, "string")
	if err != nil {
		t.Fatal(err)
	}
	encoded := fmt.Sprint(query)
	if !strings.Contains(encoded, "eligible_fields") || !strings.Contains(encoded, "must_not") {
		t.Fatalf("MISSING must distinguish absent from inapplicable: %#v", query)
	}
}

func TestOpenSearchDocumentIDIsCollisionSafe(t *testing.T) {
	if documentID("a/b:c") == documentID("a:b/c") {
		t.Fatal("lossy path sanitization must not identify projection documents")
	}
}
