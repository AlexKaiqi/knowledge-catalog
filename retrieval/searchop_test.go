package retrieval_test

import (
	"kc/retrieval"
	"testing"

	"kc/kernel"
	"kc/knowledge/reader"
)

func TestValidateSearchRequiresLocator(t *testing.T) {
	err := retrieval.ValidateSearch(retrieval.SearchOf(retrieval.SearchSORT("updated_at", "asc")))
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("%v", err)
	}
	if err := retrieval.ValidateSearch(retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); err != nil {
		t.Fatal(err)
	}
}

func TestSearchLimitIsBoundedAndDefaulted(t *testing.T) {
	req := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	req.Limit = retrieval.MaxSearchLimit + 1
	if code := kernel.CodeOf(retrieval.ValidateSearch(req)); code != kernel.ErrUsageInvalid {
		t.Fatalf("unbounded search must be rejected, got %s", code)
	}
	spec := retrieval.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Fields: []reader.FieldAccess{{Path: "body", Access: []reader.AccessHint{reader.HintText}}},
	}}})
	resolved, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchMATCH("runbook")), spec)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Limit != retrieval.DefaultSearchLimit {
		t.Fatalf("zero limit must become a bounded page, got %d", resolved.Limit)
	}
}

func TestCheckSearchHintMismatch(t *testing.T) {
	spec := retrieval.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Aspect: "structure",
		Fields: []reader.FieldAccess{
			{Path: "db", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "note", Access: []reader.AccessHint{reader.HintText}},
			{Path: "n", Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "raw", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "updated_at", Type: "date", Access: []reader.AccessHint{reader.HintSort}},
		},
	}}})
	if err := retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchEQ("db", "tl")), spec); err != nil {
		t.Fatal(err)
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchEQ("note", "x")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ on text-only path")
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpMatch, Path: "db", Value: "tl"}), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("MATCH on filter-only path")
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "raw", "1")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("GT needs a comparable type")
	}
	if err := retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "n", "1")), spec); err != nil {
		t.Fatal(err)
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchEQ("missing", "x")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("unknown path")
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchMATCH("x"), retrieval.SearchSORT("db", "asc")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("SORT without sort hint")
	}
}

func TestAllowsOpImpliedTable(t *testing.T) {
	text := retrieval.AccessField{Access: []reader.AccessHint{reader.HintText}}
	filter := retrieval.AccessField{Type: "string", Access: []reader.AccessHint{reader.HintFilter}}
	opaque := retrieval.AccessField{Type: "object", Access: []reader.AccessHint{reader.HintFilter}}
	num := retrieval.AccessField{Type: "number", Access: []reader.AccessHint{reader.HintFilter}}
	sort := retrieval.AccessField{Access: []reader.AccessHint{reader.HintSort}}
	both := retrieval.AccessField{Access: []reader.AccessHint{reader.HintText, reader.HintFilter}}
	if !retrieval.AllowsOp(text, retrieval.OpMatch) || retrieval.AllowsOp(text, retrieval.OpEQ) {
		t.Fatal("text")
	}
	if !retrieval.AllowsOp(filter, retrieval.OpEQ) || retrieval.AllowsOp(filter, retrieval.OpGT) || retrieval.AllowsOp(filter, retrieval.OpMatch) {
		t.Fatal("filter string: EQ yes; range and MATCH no")
	}
	if retrieval.AllowsOp(opaque, retrieval.OpGT) {
		t.Fatal("object filter does not imply compare")
	}
	if !retrieval.AllowsOp(num, retrieval.OpGT) {
		t.Fatal("number filter implies compare")
	}
	if !retrieval.AllowsOp(sort, retrieval.OpSort) || retrieval.AllowsOp(sort, retrieval.OpEQ) {
		t.Fatal("sort")
	}
	if !retrieval.AllowsOp(both, retrieval.OpMatch) || !retrieval.AllowsOp(both, retrieval.OpEQ) {
		t.Fatal("text+filter is two faces")
	}
}

func TestCheckSearchBareMatchNeedsTextHint(t *testing.T) {
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchMATCH("runbook")), retrieval.AccessSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("bare MATCH without text AccessHint")
	}
	if kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchEQ("db", "tl")), retrieval.AccessSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ without hints")
	}
}

func TestCheckSearchRejectsAmbiguousBarePath(t *testing.T) {
	spec := retrieval.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{
		{ObjectID: "schema/table.structure", Aspect: "structure", Fields: []reader.FieldAccess{{Path: "name", Access: []reader.AccessHint{reader.HintFilter}}}},
		{ObjectID: "schema/table.owner", Aspect: "owner", Fields: []reader.FieldAccess{{Path: "name", Access: []reader.AccessHint{reader.HintFilter}}}},
	}})
	bare := retrieval.SearchOf(retrieval.SearchEQ("name", "alice"))
	if code := kernel.CodeOf(retrieval.CheckSearch(bare, spec)); code != kernel.ErrUsageInvalid {
		t.Fatalf("ambiguous bare path must be rejected, got %s", code)
	}
	explicit := retrieval.FieldRef{Schema: "schema/table.owner", Aspect: "owner", Path: "name"}
	request := retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &explicit, Value: "alice"})
	if err := retrieval.CheckSearch(request, spec); err != nil {
		t.Fatalf("fully qualified FieldRef must resolve: %v", err)
	}
}

func TestSearchMVPValidation(t *testing.T) {
	spec := retrieval.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Aspect: "structure",
		Fields: []reader.FieldAccess{
			{Path: "name", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "n", Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "day", Type: "date", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "note", Type: "string", Access: []reader.AccessHint{reader.HintText}},
		},
	}}})
	for _, mode := range []retrieval.MatchMode{retrieval.MatchAllTerms, retrieval.MatchAnyTerms, retrieval.MatchPhrase} {
		if _, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchMATCHMode("daily events", mode)), spec); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if code := kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchMATCHMode("x", "Fuzzy")), spec)); code != kernel.ErrUsageInvalid {
		t.Fatalf("unknown mode: %s", code)
	}
	for _, clause := range []retrieval.SearchClause{retrieval.SearchMISSING("name"), retrieval.SearchPREFIX("name", "customer.")} {
		if _, err := retrieval.ResolveSearch(retrieval.SearchOf(clause), spec); err != nil {
			t.Fatal(err)
		}
	}
	if code := kernel.CodeOf(retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchPREFIX("n", "1")), spec)); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("numeric prefix: %s", code)
	}
	if _, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGT, "n", "not-a-number")), spec); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid typed scalar: %v", err)
	}
	resolved, err := retrieval.ResolveSearch(retrieval.SearchOf(retrieval.SearchRange(retrieval.OpGTE, "day", "2024-01-02")), spec)
	if err != nil || resolved.Clauses[0].Value != "2024-01-02" {
		t.Fatalf("typed date: %#v %v", resolved, err)
	}
}
