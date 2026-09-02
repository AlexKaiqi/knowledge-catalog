package cli

import (
	"testing"

	"kc/kernel"
	"kc/observability"
	"kc/retrieval"
)

func TestSearchRerankRecordsCompletedRetrievalWhenOnlyRefineFails(t *testing.T) {
	request := retrieval.SearchOf(retrieval.SearchMATCH("refund"))
	flags := map[string]FlagValue{
		"as": "agent:test", "trace-id": "trace-two-stage", "workspace": "agent", "_search-request": request,
	}
	search := retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Completeness: retrieval.CompletenessComplete, Hits: []retrieval.KnowledgeHit{},
	}
	event, err := retrievalEventFrom("search-rerank", flags, observedAccessResult{RetrievalSource: search}, "ev_1",
		kernel.Fail(kernel.ErrTemporaryUnavailable, "refine timeout"))
	if err != nil {
		t.Fatal(err)
	}
	if event.Outcome != "COMPLETED" || event.Operator != observability.RetrievalOperatorSearch || len(event.Error) != 0 {
		t.Fatalf("retrieval stage inherited downstream failure: %#v", event)
	}
}
