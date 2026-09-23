package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"kc/kernel"
	"kc/observability"
	"kc/retrieval"
)

func TestSearchRerankRecordsCompletedRetrievalWhenOnlyRefineFails(t *testing.T) {
	request := retrieval.SearchOf(retrieval.SearchMATCH("refund"))
	flags := map[string]FlagValue{
		"as": "agent:test", "trace-id": "trace-two-stage", "dataset": "agent", "_search-request": request,
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

func TestTraverseRecordsRetrievalEvidenceWithPinnedNodes(t *testing.T) {
	flags := map[string]FlagValue{
		"as": "agent:test", "trace-id": "trace-traverse", "dataset": "agent", "object": "kc://scene/knowledge/metric/gmv",
		"max-hops": "2", "min-hops": "1", "relation-type": "defines", "direction": "DIRECTED",
	}
	page := retrieval.TraversePage{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{
			"kr://scene/knowledge": "c1", "kr://scene/graph": "c2",
		}},
		Nodes: []retrieval.TraverseNode{
			{Repository: "kr://scene/knowledge", ObjectID: "schema/metric.definition", Depth: 1},
			{Repository: "kr://scene/knowledge", ObjectID: "metric/gmv", Depth: 0},
		},
		Edges: []retrieval.TraverseEdge{{
			Repository: "kr://scene/graph", Commit: "c2", ObjectID: "rel/defines/gmv",
		}},
		Exhausted: true,
	}
	event, err := retrievalEventFrom("knowledge-traverse", flags, page, "ev_2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if event.Operator != observability.RetrievalOperatorTraverse || event.Completeness != "complete" {
		t.Fatalf("traverse event = operator %s completeness %s", event.Operator, event.Completeness)
	}
	// Window = nodes (identity digests) then edges (relation body digests).
	if len(event.Candidates) != 3 || event.Candidates[0].KnowledgeRef.Commit != "c1" || event.Candidates[0].Rank != 1 {
		t.Fatalf("candidates must be pinned at snapshot basis: %#v", event.Candidates)
	}
	if event.Candidates[2].KnowledgeRef.Object != "rel/defines/gmv" || event.Candidates[2].ValueDigest == "" {
		t.Fatalf("edge candidate must carry its own commit and relation digest: %#v", event.Candidates[2])
	}
	// The recorder stamps identity and schema version at persist time.
	event.SchemaVersion = 1
	event.EvidenceID = "rt_test_1"
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchEventRecordsRecallStrategyInLogicalRequest(t *testing.T) {
	request := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	request.Recall = retrieval.RecallSemantic
	flags := map[string]FlagValue{
		"as": "agent:test", "trace-id": "trace-recall", "dataset": "agent", "_search-request": request,
	}
	search := retrieval.SearchResult{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Claims:     []string{"recall=semantic: approximate k-NN ranking window"},
	}
	event, err := retrievalEventFrom("knowledge-search", flags, search, "ev_3", nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, encodeErr := json.Marshal(event.LogicalRequest)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if !strings.Contains(string(encoded), "semantic") {
		t.Fatalf("logical request must disclose the recall strategy: %s", encoded)
	}
	if len(event.Claims) != 1 || !strings.Contains(event.Claims[0], "approximate") {
		t.Fatalf("claims must carry the approximate disclosure: %#v", event.Claims)
	}
}
