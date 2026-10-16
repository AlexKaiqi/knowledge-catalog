package opensearchimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

func TestCandidatePreservesCanonicalSourceCoordinates(t *testing.T) {
	doc, err := encodeDoc(index.CompiledDoc{ObjectID: "item/a", SourceRepository: "kr://source/a", SourceCommit: "source-c1"}, nil)
	if err != nil || doc.SourceRepository != "kr://source/a" || doc.SourceCommit != "source-c1" {
		t.Fatalf("compiled source coordinates were lost: %#v %v", doc, err)
	}
	body := []byte(`{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[{"_source":{"object_id":"item/a","source_repository":"kr://source/a","source_commit":"source-c1"},"sort":["item/a"]}]}}`)
	candidates, _, _, err := decodeCandidateResponse(body, 1)
	if err != nil || len(candidates) != 1 || candidates[0].Repository != "kr://source/a" || candidates[0].Basis != "source-c1" {
		t.Fatalf("candidate replaced the canonical source basis: %#v %v", candidates, err)
	}
	partial := []byte(`{"timed_out":false,"_shards":{"total":1,"successful":1,"failed":0},"hits":{"hits":[{"_source":{"object_id":"item/a","source_repository":"kr://source/a"},"sort":["item/a"]}]}}`)
	if _, _, _, err := decodeCandidateResponse(partial, 1); kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable {
		t.Fatalf("partial source coordinates were accepted: %v", err)
	}
}

type sourceBasisEmbedder struct{}

func (sourceBasisEmbedder) Embed(context.Context, retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	return retrieval.EmbeddingResult{}, fmt.Errorf("query vector must already be provided")
}

func TestCandidateBasisUsesControlForRepositoryAndSourcesForDataset(t *testing.T) {
	for _, lane := range []string{"lexical", "semantic"} {
		for _, dataset := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dataset=%t", lane, dataset), func(t *testing.T) {
				repository := kernel.RepositoryID("kr://source/a")
				want := []index.CandidateRef{{ObjectID: "item/a", Repository: repository, Basis: "current"}}
				stored := []index.CandidateRef{{ObjectID: "item/a", Repository: repository, Basis: "previous"}}
				if dataset {
					repository = "dataset-index/combined"
					stored = append(stored, index.CandidateRef{ObjectID: "item/b", Repository: "kr://source/b", Basis: "source-b-fixed"})
					want = stored
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet:
						_ = json.NewEncoder(w).Encode(map[string]any{"_source": controlDoc{Repository: string(repository), ActiveIndex: "generation", Generation: "g1", State: index.ProjectionStateReady, Basis: "current"}, "_seq_no": 1, "_primary_term": 1})
					case strings.HasSuffix(r.URL.Path, "/_search/point_in_time"):
						fmt.Fprint(w, `{"pit_id":"fixed","_shards":{"total":1,"successful":1,"failed":0}}`)
					case r.Method == http.MethodDelete:
						fmt.Fprint(w, `{}`)
					default:
						hits := make([]any, 0, len(stored))
						for _, candidate := range stored {
							hits = append(hits, map[string]any{"_source": map[string]any{"object_id": candidate.ObjectID, "source_repository": candidate.Repository, "source_commit": candidate.Basis}, "sort": []any{1, candidate.ObjectID}})
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"timed_out": false, "_shards": map[string]any{"total": 1, "successful": 1, "failed": 0}, "hits": map[string]any{"hits": hits}})
					}
				}))
				defer server.Close()
				engine := &openSearchEngine{base: server.URL, http: server.Client(), embedder: sourceBasisEmbedder{}, vectorDimension: 1}
				query, spec := responseTestSearch()
				spec.Repository, spec.Commit = repository, "current"
				spec.Declarations = []retrieval.DeclarationStatus{{Status: retrieval.DeclarationReady, Member: objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/main", AspectName: "index", MemberKey: "semantic"}, Uses: []string{objectmodel.IndexUseSemantic}}}
				request := index.RetrieveRequest{Search: query, Spec: spec}
				var page index.CandidatePage
				var err error
				if lane == "semantic" {
					page, err = engine.SemanticWindowContext(context.Background(), request, []float32{1})
				} else {
					page, err = engine.Retrieve(request)
				}
				if err != nil || len(page.Candidates) != len(want) {
					t.Fatalf("candidate read: %#v %v", page, err)
				}
				for i, candidate := range page.Candidates {
					if candidate.ObjectID != want[i].ObjectID || candidate.Repository != want[i].Repository || candidate.Basis != want[i].Basis {
						t.Errorf("candidate changed the wrong source basis: got=%#v want=%#v", candidate, want[i])
					}
				}
			})
		}
	}
}
