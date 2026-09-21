package opensearch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/knowledge"
	"kc/retrieval"
)

func TestRelationStorageRepositoryIsIndependentOfEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"_source":{"active_index":"graph-generation","generation":"g1","state":"READY","basis":"graph-c1"},"_seq_no":1,"_primary_term":1}`)
		case r.Method == http.MethodDelete:
			fmt.Fprint(w, `{}`)
		case r.URL.Path == "/_search":
			var query map[string]any
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				t.Error(err)
			}
			body, _ := json.Marshal(query)
			if !strings.Contains(string(body), `"relation_endpoints.repository":"kr://source/entities"`) || !strings.Contains(string(body), `"relation_endpoints.object_id":"metric/gmv"`) {
				t.Errorf("endpoint predicate lost its own repository: %s", body)
			}
			fmt.Fprint(w, `{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[{"_source":{"object_id":"relation/defines"},"sort":["relation/defines"]}]}}`)
		default:
			fmt.Fprint(w, `{"pit_id":"graph-pit","_shards":{"total":1,"successful":1,"failed":0}}`)
		}
	}))
	defer server.Close()
	engine := &openSearchEngine{base: server.URL, http: server.Client()}
	page, err := engine.RetrieveRelations(retrieval.RelationRetrieveRequest{
		Repository: "kr://source/graph", Basis: "graph-c1", Limit: 2,
		Query: retrieval.RelationQuery{Endpoint: knowledge.KnowledgeRef{Repository: "kr://source/entities", Object: "metric/gmv"}},
	})
	if err != nil || len(page.Candidates) != 1 || !page.Exhausted {
		t.Fatalf("cross-repository relation discovery: %#v %v", page, err)
	}
	candidate := page.Candidates[0]
	if candidate.Repository != "kr://source/graph" || candidate.Basis != "graph-c1" || candidate.ObjectID != "relation/defines" {
		t.Fatalf("candidate lost its storage authority: %#v", candidate)
	}
}
