package retrieval_test

import (
	"encoding/json"
	"testing"

	"kc/kernel"
	"kc/retrieval"
)

func TestSearchResultUsesSearchViewTermInJSON(t *testing.T) {
	raw, err := json.Marshal(retrieval.SearchResult{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
		Hits:       []retrieval.KnowledgeHit{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value["searchView"]; !ok {
		t.Fatalf("SearchResult must expose searchView: %s", raw)
	}
	if _, legacy := value["view"]; legacy {
		t.Fatalf("SearchResult must not expose ambiguous view: %s", raw)
	}
}
