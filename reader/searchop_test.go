package reader_test

import (
	"testing"

	"kc/kernel"
	"kc/reader"
)

func TestValidateSearchRequiresLocator(t *testing.T) {
	err := reader.ValidateSearch(reader.SearchOf(reader.SearchSORT("updated_at", "asc")))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("%v", err)
	}
	if err := reader.ValidateSearch(reader.SearchOf(reader.SearchMATCH("runbook"))); err != nil {
		t.Fatal(err)
	}
}

func TestCheckSearchHintMismatch(t *testing.T) {
	spec := reader.SpecFromReport(reader.SchemaReport{Schemas: []reader.SchemaDescription{{
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
	text := reader.IndexField{Access: []reader.AccessHint{reader.HintText}}
	filter := reader.IndexField{Type: "string", Access: []reader.AccessHint{reader.HintFilter}}
	opaque := reader.IndexField{Type: "object", Access: []reader.AccessHint{reader.HintFilter}}
	num := reader.IndexField{Type: "number", Access: []reader.AccessHint{reader.HintFilter}}
	sort := reader.IndexField{Access: []reader.AccessHint{reader.HintSort}}
	both := reader.IndexField{Access: []reader.AccessHint{reader.HintText, reader.HintFilter}}
	if !reader.AllowsOp(text, reader.OpMatch) || reader.AllowsOp(text, reader.OpEQ) {
		t.Fatal("text")
	}
	if !reader.AllowsOp(filter, reader.OpEQ) || !reader.AllowsOp(filter, reader.OpGT) || reader.AllowsOp(filter, reader.OpMatch) {
		t.Fatal("filter string: EQ/GT yes, MATCH no")
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
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchMATCH("runbook")), reader.IndexSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("bare MATCH without text AccessHint")
	}
	if kernel.CodeOf(reader.CheckSearch(reader.SearchOf(reader.SearchEQ("db", "tl")), reader.IndexSpec{})) != kernel.ErrCapabilityUnsatisfied {
		t.Fatal("EQ without hints")
	}
}
