package reader_test

import (
	"testing"

	"kc/kernel"
	"kc/reader"
)

func TestValidateSearchRequiresLocator(t *testing.T) {
	err := reader.ValidateSearch(reader.SearchOf(reader.SearchSORT("updated_at", "asc")))
	if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("%v", err)
	}
	if err := reader.ValidateSearch(reader.SearchOf(reader.SearchMATCH("runbook"))); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSearchHintMismatch(t *testing.T) {
	spec := reader.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Aspect: "structure",
		Fields: []reader.FieldAccess{
			{Path: "db", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "note", Access: []reader.AccessHint{reader.HintText}},
			{Path: "n", Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "raw", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "updated_at", Type: "date", Access: []reader.AccessHint{reader.HintSort}},
		},
	}}})
	if err := reader.CheckSearch(reader.SearchOf(reader.SearchEQ("db", "tl")), spec); err != nil {
		t.Fatal(err)
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchEQ("note", "x")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ on text-only path")
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchClause{Op: reader.OpMatch, Path: "db", Value: "tl"}), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("MATCH on filter-only path")
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchRange(reader.OpGT, "raw", "1")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("GT needs a comparable type")
	}
	if err := reader.CheckSearch(reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "1")), spec); err != nil {
		t.Fatal(err)
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchEQ("missing", "x")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("unknown path")
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchMATCH("x"), reader.SearchSORT("db", "asc")), spec)) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("SORT without sort hint")
	}
}

func TestAllowsOpImpliedTable(t *testing.T) {
	text := reader.AccessField{Access: []reader.AccessHint{reader.HintText}}
	filter := reader.AccessField{Type: "string", Access: []reader.AccessHint{reader.HintFilter}}
	opaque := reader.AccessField{Type: "object", Access: []reader.AccessHint{reader.HintFilter}}
	num := reader.AccessField{Type: "number", Access: []reader.AccessHint{reader.HintFilter}}
	sort := reader.AccessField{Access: []reader.AccessHint{reader.HintSort}}
	both := reader.AccessField{Access: []reader.AccessHint{reader.HintText, reader.HintFilter}}
	if !reader.AllowsOp(text, reader.OpMatch) || reader.AllowsOp(text, reader.OpEQ) {
		t.Fatal("text")
	}
	if !reader.AllowsOp(filter, reader.OpEQ) || reader.AllowsOp(filter, reader.OpGT) || reader.AllowsOp(filter, reader.OpMatch) {
		t.Fatal("filter string: EQ yes; range and MATCH no")
	}
	if reader.AllowsOp(opaque, reader.OpGT) {
		t.Fatal("object filter does not imply compare")
	}
	if !reader.AllowsOp(num, reader.OpGT) {
		t.Fatal("number filter implies compare")
	}
	if !reader.AllowsOp(sort, reader.OpSort) || reader.AllowsOp(sort, reader.OpEQ) {
		t.Fatal("sort")
	}
	if !reader.AllowsOp(both, reader.OpMatch) || !reader.AllowsOp(both, reader.OpEQ) {
		t.Fatal("text+filter is two faces")
	}
}

func TestCheckSearchBareMatchNeedsTextHint(t *testing.T) {
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchMATCH("runbook")), reader.AccessSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("bare MATCH without text AccessHint")
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchEQ("db", "tl")), reader.AccessSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ without hints")
	}
}

func TestCheckSearchRejectsAmbiguousBarePath(t *testing.T) {
	spec := reader.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{
		{ObjectID: "schema/table.structure", Aspect: "structure", Fields: []reader.FieldAccess{{Path: "name", Access: []reader.AccessHint{reader.HintFilter}}}},
		{ObjectID: "schema/table.owner", Aspect: "owner", Fields: []reader.FieldAccess{{Path: "name", Access: []reader.AccessHint{reader.HintFilter}}}},
	}})
	bare := reader.SearchOf(reader.SearchEQ("name", "alice"))
	if code := kernel.CodeOf(reader.CheckSearch(bare, spec)); code != kernel.ErrUsageInvalid {
		t.Fatalf("ambiguous bare path must be rejected, got %s", code)
	}
	explicit := reader.FieldRef{Schema: "schema/table.owner", Aspect: "owner", Path: "name"}
	request := reader.SearchOf(reader.SearchClause{Op: reader.OpEQ, Field: &explicit, Value: "alice"})
	if err := reader.CheckSearch(request, spec); err != nil {
		t.Fatalf("fully qualified FieldRef must resolve: %v", err)
	}
}

func TestSearchMVPValidation(t *testing.T) {
	spec := reader.AccessSpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
		ObjectID: "schema/t", Aspect: "structure",
		Fields: []reader.FieldAccess{
			{Path: "name", Type: "string", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "n", Type: "number", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "day", Type: "date", Access: []reader.AccessHint{reader.HintFilter}},
			{Path: "note", Type: "string", Access: []reader.AccessHint{reader.HintText}},
		},
	}}})
	for _, mode := range []reader.MatchMode{reader.MatchAllTerms, reader.MatchAnyTerms, reader.MatchPhrase} {
		if _, err := reader.ResolveSearch(reader.SearchOf(reader.SearchMATCHMode("daily events", mode)), spec); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	if code := kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchMATCHMode("x", "Fuzzy")), spec)); code != kernel.ErrUsageInvalid {
		t.Fatalf("unknown mode: %s", code)
	}
	for _, clause := range []reader.SearchClause{reader.SearchMISSING("name"), reader.SearchPREFIX("name", "customer.")} {
		if _, err := reader.ResolveSearch(reader.SearchOf(clause), spec); err != nil {
			t.Fatal(err)
		}
	}
	if code := kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchPREFIX("n", "1")), spec)); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("numeric prefix: %s", code)
	}
	if _, err := reader.ResolveSearch(reader.SearchOf(reader.SearchRange(reader.OpGT, "n", "not-a-number")), spec); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("invalid typed scalar: %v", err)
	}
	resolved, err := reader.ResolveSearch(reader.SearchOf(reader.SearchRange(reader.OpGTE, "day", "2024-01-02")), spec)
	if err != nil || resolved.Clauses[0].Value != "2024-01-02" {
		t.Fatalf("typed date: %#v %v", resolved, err)
	}
}
