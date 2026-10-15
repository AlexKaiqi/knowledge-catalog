package invocation

import (
	"encoding/json"
	retrieval "kc/discovery"
	"kc/invoke"
	"kc/kernel"
	"kc/observability"
	"strings"
	"testing"
)

func TestSearchRerankRecordsCompletedRetrievalWhenOnlyRefineFails(t *testing.T) {
	request := retrieval.SearchOf(retrieval.SearchMATCH("refund"))
	flags := map[string]invoke.FlagValue{
		"as": "agent:test", "trace-id": "trace-two-stage", "dataset": "agent", "_search-request": request,
	}
	search := retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Completeness: retrieval.CompletenessComplete, Hits: []retrieval.ObjectHit{},
	}
	event, err := RetrievalEventFrom("search-rerank", flags, ObservedAccessResult{RetrievalSource: search}, "ev_1",
		kernel.Fail(kernel.ErrTemporaryUnavailable, "refine timeout"))
	if err != nil {
		t.Fatal(err)
	}
	if event.Outcome != "COMPLETED" || event.Operator != observability.RetrievalOperatorSearch || len(event.Error) != 0 {
		t.Fatalf("retrieval stage inherited downstream failure: %#v", event)
	}
}

func TestTraverseRecordsRetrievalEvidenceWithPinnedEdges(t *testing.T) {
	flags := map[string]invoke.FlagValue{
		"as": "agent:test", "trace-id": "trace-traverse", "dataset": "agent", "object": "metric/gmv",
		"max-hops": "2", "min-hops": "1", "relation-type": "defines", "direction": "DIRECTED",
	}
	page := retrieval.TraversePage{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{
			"kr://scene/object": "c1", "kr://scene/graph": "c2",
		}},
		Nodes: []retrieval.TraverseNode{{ObjectID: "schema/metric.definition", Depth: 1}},
		Edges: []retrieval.TraverseEdge{{
			Repository: "kr://scene/graph", Commit: "c2", ObjectID: "rel/defines/gmv",
		}},
		Exhausted: true,
	}
	event, err := RetrievalEventFrom("object-traverse", flags, page, "ev_2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if event.Operator != observability.RetrievalOperatorTraverse || event.Completeness != "complete" {
		t.Fatalf("traverse event = operator %s completeness %s", event.Operator, event.Completeness)
	}
	// Nodes are object IDs, not stored values at one commit; the citable
	// candidates are the edges, each pinned at its own storage commit.
	if event.Execution.Candidates != 2 || len(event.Candidates) != 1 {
		t.Fatalf("traverse window = %d candidates %#v", event.Execution.Candidates, event.Candidates)
	}
	edge := event.Candidates[0]
	if edge.ObjectRef.Repository != "kr://scene/graph" || edge.ObjectRef.Commit != "c2" || edge.ObjectRef.Object != "rel/defines/gmv" || edge.Rank != 1 || edge.ValueDigest == "" {
		t.Fatalf("edge candidate must carry its own commit and relation digest: %#v", edge)
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
	flags := map[string]invoke.FlagValue{
		"as": "agent:test", "trace-id": "trace-recall", "dataset": "agent", "_search-request": request,
	}
	search := retrieval.SearchResult{
		SearchView: retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{"kr://acme/runbooks": "c1"}},
		Claims:     []string{"recall=semantic: approximate k-NN ranking window"},
	}
	event, err := RetrievalEventFrom("object-search", flags, search, "ev_3", nil)
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
