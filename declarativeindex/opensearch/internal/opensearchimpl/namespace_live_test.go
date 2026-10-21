package opensearchimpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	ports "kc/discovery/projection"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Run against the existing cluster. Missing configuration is a failed required
// live contract, never a skipped or automatically provisioned environment.
func TestLiveOpenSearchNamespacesIsolateSameRepositoryAndContinuations(t *testing.T) {
	endpoint := strings.TrimRight(os.Getenv("KC_TEST_OPENSEARCH_URL"), "/")
	if endpoint == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL must name an existing OpenSearch environment")
	}
	t.Setenv(EnvEmbeddingModel, "")
	identity := sha256.Sum256([]byte(t.TempDir()))
	name := "namespace-test-" + hex.EncodeToString(identity[:8])
	repo := testkit.MakeRepository(t, "kr://regression/"+name)
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	ops := []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{
		"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}},
	}}}
	ops = append(ops, testkit.IndexText("schema/article", "body"))
	for _, id := range []string{"article/a", "article/b", "article/c"} {
		ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook"}})
	}
	for _, id := range []string{"relation/a", "relation/b"} {
		ops = append(ops, objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: objectmodel.ObjectID(id)}, Value: map[string]any{
			"relationId": id, "relationType": "related", "direction": "DIRECTED", "endpoints": []any{
				map[string]any{"role": "subject", "object": "article/a"},
				map[string]any{"role": "target", "object": "article/b"},
			},
		}})
	}
	a, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: ops})
	if err != nil {
		t.Fatal(err)
	}
	resources := &namespaceResources{endpoint: endpoint, indices: map[string]bool{}, documents: map[string]bool{}}
	t.Cleanup(func() { resources.cleanup(t) })
	openIndex := func(namespace string) *index.Index {
		// Decode the selected public configuration. Before Namespace is implemented
		// this remains buildable, and fails on actual shared physical state.
		raw, err := json.Marshal(map[string]any{"url": endpoint, "namespace": namespace, "primaryShards": 1})
		if err != nil {
			t.Fatal(err)
		}
		var cfg Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		opener := Open(cfg)
		idx := index.NewIndexEngine(t.TempDir(), func(dir string, id kernel.RepositoryID) (ports.Engine, error) {
			engine, err := opener(dir, id)
			if err == nil {
				engine.(*openSearchEngine).http.Transport = resources
			}
			return engine, err
		})
		t.Cleanup(func() { _ = idx.Close() })
		return idx
	}
	left, right := openIndex(name+"-a"), openIndex(name+"-b")
	for _, idx := range []*index.Index{left, right} {
		if _, err := idx.Rebuild(repo, a); err != nil {
			t.Fatal(err)
		}
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	query.Limit = 1
	first, err := left.SearchAt(repo, a, query)
	if err != nil || len(first.Hits) != 1 || first.Continuation == "" {
		t.Fatalf("initial namespace page: result=%#v error=%v", first, err)
	}
	continued := query
	continued.Continuation = first.Continuation
	if _, err := right.SearchAt(repo, a, continued); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Errorf("another namespace accepted this instance's continuation: %v", err)
	}
	relationQuery := retrieval.RelationPageRequest{Limit: 1, Query: retrieval.RelationQuery{
		Endpoint: "article/a", RelationType: "related", Role: "subject",
	}}
	firstRelation, err := left.RelationsAt(repo, a, relationQuery)
	if err != nil || len(firstRelation.Hits) != 1 || firstRelation.Continuation == "" {
		t.Fatalf("initial relation page: result=%#v error=%v", firstRelation, err)
	}
	relationQuery.Continuation = firstRelation.Continuation
	if _, err := right.RelationsAt(repo, a, relationQuery); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Errorf("another namespace accepted this instance's relation continuation: %v", err)
	}
	// The same instance identity must reopen existing physical resources even
	// when the private directory changes. Its own continuation remains valid.
	reopened := openIndex(name + "-a")
	page, err := reopened.SearchAt(repo, a, continued)
	if err != nil || len(page.Hits) != 1 || page.Hits[0].Object.Address.ObjectID == first.Hits[0].Object.Address.ObjectID {
		t.Fatalf("stable namespace failed to resume its own page: result=%#v error=%v", page, err)
	}
	firstRight, err := right.SearchAt(repo, a, query)
	if err != nil || len(firstRight.Hits) != 1 || firstRight.Continuation == "" {
		t.Fatalf("independent namespace initial page: result=%#v error=%v", firstRight, err)
	}
	relationPage, err := reopened.RelationsAt(repo, a, relationQuery)
	if err != nil || len(relationPage.Hits) != 1 || relationPage.Hits[0].ObjectID == firstRelation.Hits[0].ObjectID {
		t.Fatalf("stable namespace failed to resume its own relation page: result=%#v error=%v", relationPage, err)
	}
	b, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: a, ExpectedTargetCommit: a, Operations: []objectmodel.Operation{{
		Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/d"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := left.Ensure(context.Background(), repo, b); err != nil {
		t.Fatal(err)
	}
	for _, state := range []struct {
		idx   *index.Index
		basis kernel.CommitID
		count int
		// The ordinary declaration object joins the three articles and two
		// relations; only the left instance has the fourth article.
	}{{left, b, 7}, {right, a, 6}} {
		desc, err := state.idx.Describe(repo)
		if err != nil || desc.BasisCommit != state.basis || desc.ObjectCount != state.count {
			t.Errorf("instance physical progress leaked: descriptor=%#v error=%v want basis=%s count=%d", desc, err, state.basis, state.count)
		}
	}
	if err := left.Close(); err != nil {
		t.Fatal(err)
	}
	continued.Continuation = firstRight.Continuation
	rightPage, err := right.SearchAt(repo, a, continued)
	if err != nil || len(rightPage.Hits) != 1 || rightPage.Hits[0].Object.Address.ObjectID == firstRight.Hits[0].Object.Address.ObjectID {
		t.Fatalf("other instance advancement invalidated this namespace's own continuation: result=%#v error=%v", rightPage, err)
	}
	query.Limit = 100
	result, err := right.SearchAt(repo, a, query)
	if err != nil {
		t.Fatalf("independent namespace lost its fixed basis after other instance advanced and closed: %v", err)
	}
	ids := make([]objectmodel.ObjectID, 0, len(result.Hits))
	for _, hit := range result.Hits {
		if hit.Object.Repository != repo.ID() || hit.Object.Commit != a {
			t.Errorf("namespace changed logical object coordinates: %#v", hit)
		}
		ids = append(ids, hit.Object.Address.ObjectID)
	}
	if result.Completeness != retrieval.CompletenessComplete || !reflect.DeepEqual(ids, []objectmodel.ObjectID{"article/a", "article/b", "article/c"}) {
		t.Fatalf("independent namespace returned wrong contents: %#v", result)
	}
}

// Track resources actually created through the real transport, not inferred
// names. The legacy control index is shared, so remove only our control docs.
type namespaceResources struct {
	endpoint  string
	mu        sync.Mutex
	indices   map[string]bool
	documents map[string]bool
}

func (r *namespaceResources) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err == nil && request.Method == http.MethodPut && response.StatusCode < 300 {
		path := strings.Trim(request.URL.Path, "/")
		r.mu.Lock()
		if !strings.Contains(path, "/") && path != controlIndexName {
			r.indices[path] = true
		} else if strings.Contains(path, "/_doc/") {
			r.documents[path] = true
		}
		r.mu.Unlock()
	}
	return response, err
}

func (r *namespaceResources) cleanup(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	for _, group := range []map[string]bool{r.documents, r.indices} {
		for path := range group {
			request, err := http.NewRequest(http.MethodDelete, r.endpoint+"/"+path, nil)
			if err != nil {
				t.Error(err)
				continue
			}
			response, err := client.Do(request)
			if err != nil {
				t.Errorf("cleanup owned namespace resource %s: %v", path, err)
				continue
			}
			response.Body.Close()
			if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
				t.Errorf("cleanup owned namespace resource %s: HTTP %d", path, response.StatusCode)
			}
		}
	}
}
