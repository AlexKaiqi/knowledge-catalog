package index

import (
	"kc/retrieval"
	"strings"
	"testing"
	"unicode"
)

// The fixture provider uses lowercased letter/digit tokens, with contiguous
// token positions for phrase queries. Its superset lane returns every object.
func (*stateTestEngine) VerifyResidualMatch(text, query string, mode retrieval.MatchMode) (bool, error) {
	tokenize := func(raw string) []string {
		return strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	}
	terms, wanted := tokenize(text), tokenize(query)
	if len(wanted) == 0 {
		return false, nil
	}
	if mode == retrieval.MatchPhrase {
		for i := 0; i+len(wanted) <= len(terms); i++ {
			if strings.Join(terms[i:i+len(wanted)], "\x00") == strings.Join(wanted, "\x00") {
				return true, nil
			}
		}
		return false, nil
	}
	for _, want := range wanted {
		found := false
		for _, term := range terms {
			if term == want {
				found = true
				break
			}
		}
		if mode == retrieval.MatchAnyTerms && found {
			return true, nil
		}
		if mode != retrieval.MatchAnyTerms && !found {
			return false, nil
		}
	}
	return mode != retrieval.MatchAnyTerms, nil
}

func TestResidualAnalysisPreservesTokensPhrasesAndAny(t *testing.T) {
	verifier := &stateTestEngine{}
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
		got, err := residualMatch(test.text, test.query, test.mode, verifier)
		if err != nil || got != test.want {
			t.Fatalf("%#v: %v %v", test, got, err)
		}
	}
	expr := retrieval.SearchAny(retrieval.SearchLeaf(retrieval.SearchMATCHMode("cat", retrieval.MatchAllTerms)), retrieval.SearchLeaf(retrieval.SearchMATCHMode("hello world", retrieval.MatchPhrase)))
	got, err := matchesResidualExpression(expr, CompiledDoc{Text: "hello-world"}, nil, retrieval.AccessSpec{}, verifier)
	if err != nil || !got {
		t.Fatalf("Any lost matching branch: %v %v", got, err)
	}
}
