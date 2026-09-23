package retrieval

import (
	"testing"

	"kc/kernel"
)

func TestValidateSearchRecallStrategyClosedSet(t *testing.T) {
	valid := []SearchRequest{
		SearchOf(SearchMATCH("gmv")),
		SearchOf(SearchMATCH("gmv")),
	}
	valid[0].Recall = ""
	valid[1].Recall = RecallLexical
	for _, req := range valid {
		if err := ValidateSearch(req); err != nil {
			t.Fatalf("lexical recall must validate: %v", err)
		}
	}
	unknown := SearchOf(SearchMATCH("gmv"))
	unknown.Recall = RecallStrategy("vector")
	err := ValidateSearch(unknown)
	if code := kernel.CodeOf(err); code != kernel.ErrUsageInvalid {
		t.Fatalf("unknown recall must be USAGE_INVALID, got %v", err)
	}
}

func TestValidateSearchHybridReserved(t *testing.T) {
	req := SearchOf(SearchMATCH("gmv"))
	req.Recall = RecallHybrid
	err := ValidateSearch(req)
	if code := kernel.CodeOf(err); code != kernel.ErrUsageInvalid {
		t.Fatalf("reserved hybrid recall must be rejected with USAGE_INVALID, got %v", err)
	}
}

func TestValidateSearchSemanticRequiresMatchText(t *testing.T) {
	withText := SearchOf(SearchMATCH("gmv"))
	withText.Recall = RecallSemantic
	if err := ValidateSearch(withText); err != nil {
		t.Fatalf("semantic recall with MATCH text must validate: %v", err)
	}
	expression := SearchRequest{Expression: &SearchExpr{All: []SearchExpr{
		SearchLeaf(SearchMATCH("gmv")),
		SearchLeaf(SearchEQ("meta.status", "published")),
	}}, Recall: RecallSemantic}
	if err := ValidateSearch(expression); err != nil {
		t.Fatalf("semantic recall with expression MATCH leaf must validate: %v", err)
	}
	filterOnly := SearchOf(SearchEQ("meta.status", "published"))
	filterOnly.Recall = RecallSemantic
	err := ValidateSearch(filterOnly)
	if code := kernel.CodeOf(err); code != kernel.ErrUsageInvalid {
		t.Fatalf("semantic recall without MATCH text must be USAGE_INVALID, got %v", err)
	}
}
