package projection

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	objectmodel "kc/object"
)

func coverageSpec() retrieval.AccessSpec {
	return retrieval.AccessSpec{Declarations: []retrieval.DeclarationStatus{
		{Member: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/sales-analysis", MemberKey: "table-name"}, Status: retrieval.DeclarationUnsupported,
			Reason: "analyzer not-installed is not registered", Uses: []string{objectmodel.IndexUseText}},
		{Member: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/sales-analysis", MemberKey: "metric-text"}, Status: retrieval.DeclarationReady,
			Uses: []string{objectmodel.IndexUseText}},
		{Member: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/sales-analysis", MemberKey: "metric-unit"}, Status: retrieval.DeclarationInvalid,
			Reason: "input missing", Uses: []string{objectmodel.IndexUseFilter}},
	}}
}

func TestUnfieldedMatchNamesUnreadyTextMemberAsPartial(t *testing.T) {
	result := retrieval.SearchResult{Completeness: retrieval.CompletenessComplete}
	req := retrieval.SearchRequest{Clauses: []retrieval.SearchClause{retrieval.SearchMATCH("orders")}}
	applyDeclarationCoverage(req, coverageSpec(), &result)
	if result.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("unfielded MATCH over an unsupported text member reported %s", result.Completeness)
	}
	if len(result.Claims) != 1 {
		t.Fatalf("expected exactly the unready text member to be named, got %v", result.Claims)
	}
	claim := result.Claims[0]
	if !strings.Contains(claim, "table-name") || !strings.Contains(claim, retrieval.DeclarationUnsupported) || !strings.Contains(claim, "analyzer") {
		t.Fatalf("claim must name member, status and reason: %s", claim)
	}
}

func TestDeclarationCoverageIgnoresQueriesWithoutUnfieldedMatch(t *testing.T) {
	field := retrieval.FieldRef{Path: "name"}
	for name, req := range map[string]retrieval.SearchRequest{
		"fielded match": {Clauses: []retrieval.SearchClause{{Op: retrieval.OpMatch, Value: "orders", Field: &field}}},
		"filter only":   {Clauses: []retrieval.SearchClause{retrieval.SearchEQ("unit", "cny")}},
	} {
		result := retrieval.SearchResult{Completeness: retrieval.CompletenessComplete}
		applyDeclarationCoverage(req, coverageSpec(), &result)
		if result.Completeness != retrieval.CompletenessComplete || len(result.Claims) != 0 {
			t.Fatalf("%s: unexpected coverage downgrade %s %v", name, result.Completeness, result.Claims)
		}
	}
	result := retrieval.SearchResult{Completeness: retrieval.CompletenessComplete}
	ready := retrieval.AccessSpec{Declarations: coverageSpec().Declarations[1:]}
	applyDeclarationCoverage(retrieval.SearchRequest{Clauses: []retrieval.SearchClause{retrieval.SearchMATCH("orders")}}, ready, &result)
	if result.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("non-text unready member must not downgrade text search: %v", result.Claims)
	}
}
