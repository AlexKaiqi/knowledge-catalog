package projection

import (
	"kc/declarativeindex/internal/testfixture"
	retrieval "kc/discovery"

	"testing"
)

// The fixture provider uses lowercased letter/digit tokens, with contiguous
// token positions for phrase queries. Its superset lane returns every object.

func TestResidualAnalysisPreservesTokensPhrasesAndAny(t *testing.T) {
	verifier := &testfixture.StateEngine{}
	for _, test := range []struct {
		text, query string
		mode        retrieval.MatchMode
		want        bool
	}{
		{"concatenate", "cat", retrieval.MatchAllTerms, false},
		{"hello-world", "hello world", retrieval.MatchPhrase, true},
		{"hello wide world", "hello world", retrieval.MatchPhrase, false},
		{"world hello", "hello world", retrieval.MatchAllTerms, true},
	} {
		got, err := retrieval.MatchFields(retrieval.SearchOf(retrieval.SearchMATCHMode(test.query, test.mode)), retrieval.FieldSet{Text: test.text}, retrieval.AccessSpec{}, verifier)
		if err != nil || got != test.want {
			t.Fatalf("%#v: %v %v", test, got, err)
		}
	}
	expr := retrieval.SearchAny(retrieval.SearchLeaf(retrieval.SearchMATCHMode("cat", retrieval.MatchAllTerms)), retrieval.SearchLeaf(retrieval.SearchMATCHMode("hello world", retrieval.MatchPhrase)))
	got, err := retrieval.MatchFields(retrieval.SearchRequest{Expression: &expr}, retrieval.FieldSet{Text: "hello-world"}, retrieval.AccessSpec{}, verifier)
	if err != nil || !got {
		t.Fatalf("Any lost matching branch: %v %v", got, err)
	}
}
