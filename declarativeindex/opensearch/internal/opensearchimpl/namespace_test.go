package opensearchimpl

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/kernel"
	"kc/object/reader"
)

func TestOpenSearchNamespacePreservesLegacyIdentity(t *testing.T) {
	t.Setenv(EnvEmbeddingModel, "")
	repository := kernel.RepositoryID("kr://namespace/repo")
	open := func(namespace, directory string) *openSearchEngine {
		engine, err := Open(Config{Namespace: namespace, PrimaryShards: 1})(directory, repository)
		if err != nil {
			t.Fatal(err)
		}
		return engine.(*openSearchEngine)
	}
	legacy := open("", "legacy-dir")
	if legacy.prefix != "kc-proj-a26b3166e88592411679ac5f" || legacy.controlID != "a26b3166e88592411679ac5f03e8842fcc4dcec86920dfb72820592d48da154c" || legacy.controlIndexPath() != controlIndexName {
		t.Fatalf("empty namespace changed existing physical coordinates: %#v", legacy)
	}
	if digest := legacy.PhysicalDigest(); digest != "8333b7fc27d9fa6d46615500b6590576a31210135650ba4c2f188009c4bd9e7a" {
		t.Fatalf("empty namespace invalidated existing projection identity: %s", digest)
	}
	left, right, reopened := open("a", "dir-a"), open("b", "dir-b"), open("a", "another-dir")
	if left.prefix == right.prefix || left.controlIndexPath() == right.controlIndexPath() || left.PhysicalDigest() == right.PhysicalDigest() {
		t.Fatal("separate instance identities share physical resources or projection identity")
	}
	if left.prefix != reopened.prefix || left.controlIndexPath() != reopened.controlIndexPath() || left.PhysicalDigest() != reopened.PhysicalDigest() {
		t.Fatal("private directory changed stable instance resource identity")
	}
	for _, engine := range []*openSearchEngine{legacy, left, right, reopened} {
		if engine.repository != repository {
			t.Fatalf("namespace replaced the logical object Repository: %s", engine.repository)
		}
	}
	const oldPage = "eyJwaXQiOiJsZWdhY3ktcGl0IiwiYmFzaXMiOiJjMSIsInJlcG9zaXRvcnkiOiJrcjovL25hbWVzcGFjZS9yZXBvIiwicXVlcnkiOiJxMSIsImdlbmVyYXRpb24iOiJnMSIsInNvcnQiOlsiYXJ0aWNsZS9hIl0sInJhbmsiOjEsImNoZWNrIjoiMTk1ODNiZTBlNmQ3OWU5YjJjZmM4ZTQ4NTgyMjEwYTE4MzdmODdiYjA5NTNhNDQxYTZjM2FhNzA5MDQ3ZDJlOCJ9"
	page, err := decodePITContinuation(oldPage)
	if err != nil || page.Namespace != "" || encodePITContinuation(page) != oldPage {
		t.Fatalf("legacy continuation changed or no longer decodes: %#v %v", page, err)
	}
}

// Equal generation/basis/query is deliberately held constant: namespace must
// independently reject the page before opening PIT, for both provider lanes.
func TestOpenSearchContinuationRejectsOtherNamespaceBeforePIT(t *testing.T) {
	for _, pair := range [][2]string{{"a", "b"}, {"", "a"}, {"a", ""}} {
		for _, lane := range []string{"search", "relations"} {
			t.Run(pair[0]+"-to-"+pair[1]+"/"+lane, func(t *testing.T) {
				var reads, other atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						reads.Add(1)
						fmt.Fprint(w, `{"_source":{"active_index":"generation","generation":"g1","state":"READY","basis":"c1"},"_seq_no":1,"_primary_term":1}`)
						return
					}
					other.Add(1)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				engine := &openSearchEngine{base: server.URL, http: server.Client(), namespace: pair[1]}
				repository := kernel.RepositoryID("kr://namespace/repo")
				page := pitContinuation{Basis: "c1", Repository: repository, Generation: "g1", Namespace: namespaceDigest(pair[0])}
				var err error
				if lane == "search" {
					request := index.RetrieveRequest{Search: retrieval.SearchOf(retrieval.SearchMATCH("runbook")), Spec: retrieval.AccessSpec{
						Repository: repository, Commit: "c1", Fields: []retrieval.AccessField{{FieldRef: retrieval.FieldRef{Schema: "schema/article", Path: "body"}, Type: "string", Access: []reader.AccessHint{reader.HintText}}},
					}}
					page.Query = string(retrieval.SearchQueryDigest(request.Search))
					request.Continuation = encodePITContinuation(page)
					_, err = engine.Retrieve(request)
				} else {
					request := retrieval.RelationRetrieveRequest{Repository: repository, Basis: "c1", Query: retrieval.RelationQuery{Endpoint: "article/a"}}
					page.Query = string(retrieval.RelationQueryDigest(request.Query))
					request.Continuation = encodePITContinuation(page)
					_, err = engine.RetrieveRelations(request)
				}
				if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || reads.Load() != 1 || other.Load() != 0 {
					t.Fatalf("namespace mismatch bypassed page rejection: error=%v controlReads=%d laterRequests=%d", err, reads.Load(), other.Load())
				}
			})
		}
	}
}
