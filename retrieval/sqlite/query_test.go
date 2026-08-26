package sqlite

import (
	"kc/retrieval"
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

func TestDeclaredTextFallbackSupportsCJKWithoutScanningOtherFields(t *testing.T) {
	opened, err := Open("", "kr://test/cjk")
	if err != nil {
		t.Fatal(err)
	}
	engine := opened.(*sqliteEngine)
	defer engine.Close()

	body := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/runbook", Path: "body"},
		Type:     "string", Access: []reader.AccessHint{reader.HintText},
	}
	title := retrieval.AccessField{
		FieldRef: retrieval.FieldRef{Schema: "schema/runbook", Path: "title"},
		Type:     "string", Access: []reader.AccessHint{reader.HintText},
	}
	secret := retrieval.FieldRef{Schema: "schema/runbook", Path: "secret"}
	spec := retrieval.AccessSpec{Fields: []retrieval.AccessField{body, title}}
	docs := []index.CompiledDoc{
		{
			ObjectID: "runbook/a",
			Text:     "切换支付流量前先检查冻结窗口",
			Fields: [][2]string{
				{body.FieldRef.Key(), "切换支付流量前先检查冻结"},
				{title.FieldRef.Key(), "窗口"},
			},
		},
		{
			ObjectID: "runbook/hidden",
			Text:     "",
			Fields:   [][2]string{{secret.Key(), "秘密窗口"}},
		},
	}
	if err := engine.Rebuild(docs, index.Meta{Basis: "c1"}); err != nil {
		t.Fatal(err)
	}

	ids, err := clauseIDs(engine.db, retrieval.SearchClause{
		Op: retrieval.OpMatch, Value: "冻结 窗口", Mode: retrieval.MatchAllTerms,
	}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != knowledge.ObjectID("runbook/a") {
		t.Fatalf("CJK all-terms across declared text fields: %#v", ids)
	}

	ids, err = clauseIDs(engine.db, retrieval.SearchClause{
		Op: retrieval.OpMatch, Value: "秘密", Mode: retrieval.MatchAllTerms,
	}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("fallback scanned a field without text access: %#v", ids)
	}

	if engine.ProviderRevision() != "sqlite-v4-search-mvp" || engine.PhysicalDigest() == kernel.Digest("") {
		t.Fatalf("provider revision must identify the new query semantics: %s", engine.ProviderRevision())
	}
}
