package projection_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"kc/declarativeindex/capability"
	"kc/declarativeindex/internal/testfixture"
	"kc/declarativeindex/opensearch"
	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type datasetTestEngine struct {
	meta    ports.Meta
	docs    []ports.CompiledDoc
	queries int
}

func (e *datasetTestEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeSuperset, Coverage: 1}
}
func (e *datasetTestEngine) LoadMeta() (ports.Meta, error) { return e.meta, nil }
func (e *datasetTestEngine) Rebuild(docs []ports.CompiledDoc, meta ports.Meta) error {
	e.docs = append([]ports.CompiledDoc(nil), docs...)
	e.meta = meta
	return nil
}
func (e *datasetTestEngine) Apply(upserts []ports.CompiledDoc, deletes []objectmodel.ObjectID, meta ports.Meta) error {
	return e.Rebuild(upserts, meta)
}
func (e *datasetTestEngine) Count() (int, error) { return len(e.docs), nil }
func (e *datasetTestEngine) Close() error        { return nil }
func (e *datasetTestEngine) Retrieve(req ports.RetrieveRequest) (ports.CandidatePage, error) {
	e.queries++
	out := ports.CandidatePage{Exhausted: true}
	for _, doc := range e.docs {
		repo, at := doc.SourceRepository, doc.SourceCommit
		if repo == "" {
			repo, at = req.Spec.Repository, e.meta.Basis
		}
		out.Candidates = append(out.Candidates, ports.CandidateRef{Repository: repo, Basis: at, ObjectID: doc.ObjectID})
	}
	return out, nil
}
func (e *datasetTestEngine) VerifyResidualMatch(text, query string, _ retrieval.MatchMode) (bool, error) {
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(strings.ToLower(text), word) {
			return false, nil
		}
	}
	return true, nil
}

func TestDatasetSpaceBuildsOneCorpusAndComposesCrossSourceAspects(t *testing.T) {
	runDatasetSpaceCorpus(t, false)
}

func TestDatasetSpaceLiveOpenSearchComposesCrossSourceAspects(t *testing.T) {
	runDatasetSpaceCorpus(t, true)
}

func runDatasetSpaceCorpus(t *testing.T, live bool) {
	a := testkit.MakeRepository(t, "kr://dataset/title")
	b := testkit.MakeRepository(t, "kr://dataset/body")
	decl := testkit.MakeRepository(t, "kr://dataset/maintainer")
	commit := func(repo *testkit.ObjectRepository, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		base, _ := repo.Head(snapshot.DefaultRef)
		at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	schema := func(id objectmodel.ObjectID, aspect, path string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, PathHint: "schemas/" + path + ".yaml", Value: map[string]any{"entity": "Article", "aspect": aspect, "fields": map[string]any{path: map[string]any{"type": "string"}}}}
	}
	name, body := schema("schema/article.name", "properties", "name"), schema("schema/article.body", "definition", "body")
	unit := func(aspect, field, text string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: aspect}, SchemaRef: "schema/article." + field, Value: map[string]any{field: text}}
	}
	atA, atB := commit(a, name, unit("properties", "name", "runbook")), commit(b, body, unit("definition", "body", "discount original"))
	op := testkit.IndexUnder("indexes", testkit.IndexMember("article-text", "text", []string{"text"}, "schema/article.name#/fields/name", "schema/article.body#/fields/body"))
	atD := commit(decl, name, body, op)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		switch id {
		case a.ID():
			return a, nil
		case b.ID():
			return b, nil
		default:
			return decl, nil
		}
	}
	pin := reader.DatasetPin{SetID: "articles", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{a.ID(): atA, b.ID(): atB, decl.ID(): atD}, Items: []reader.DatasetItem{{Repository: a.ID(), Commit: atA, Kind: "prefix"}, {Repository: b.ID(), Commit: atB, Kind: "prefix"}, {Repository: decl.ID(), Commit: atD, Kind: "prefix", Prefix: "indexes"}}}
	engines := map[kernel.RepositoryID]*datasetTestEngine{}
	opener := func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		eng := engines[id]
		if eng == nil {
			eng = &datasetTestEngine{}
			engines[id] = eng
		}
		return eng, nil
	}
	if live {
		endpoint := os.Getenv("KC_TEST_OPENSEARCH_URL")
		if endpoint == "" {
			t.Fatal("Dataset corpus contract requires existing OpenSearch")
		}
		namespace := "dataset-space-" + string(kernel.CanonicalDigest(t.TempDir()))
		t.Cleanup(func() { testfixture.CleanupNamespace(t, endpoint, namespace) })
		opener = opensearch.Open(opensearch.Config{URL: endpoint, Namespace: namespace, PrimaryShards: 1})
	}
	idx := index.NewIndexEngine(t.TempDir(), opener)
	t.Cleanup(func() { _ = idx.Close() })
	manifest := capability.DefaultManifest()
	idx.SetDeclarationManifest(manifest.Check, manifest.Describe().Digest)
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook discount"))
	result, err := idx.SearchDataset(context.Background(), lookup, pin, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Completeness != retrieval.CompletenessComplete || len(result.Hits[0].Object.Sources) != 2 || result.Dataset.ID != "articles" || result.Dataset.Revision != 1 {
		t.Fatalf("Dataset corpus lost cross-source object or basis: %#v", result)
	}
	hit := result.Hits[0].Object
	if hit.Repository != "" || hit.Commit != "" || hit.Sources[0].Commit != atB && hit.Sources[1].Commit != atB {
		t.Fatalf("composed object was falsely assigned one source version: %#v", hit)
	}
	queries := 0
	for _, eng := range engines {
		queries += eng.queries
	}
	if !live && queries != 1 {
		t.Fatalf("one Dataset query fanned out into %d physical queries", queries)
	}
	commit(b, unit("definition", "body", "replacement"))
	fixed, err := idx.SearchDataset(context.Background(), lookup, pin, query)
	if err != nil || len(fixed.Hits) != 1 || kernel.CanonicalDigest(fixed.Hits[0].Object.Value) != kernel.CanonicalDigest(hit.Value) {
		t.Fatalf("source HEAD changed the pinned Dataset: %#v %v", fixed, err)
	}
	plain := pin
	plain.SetID = "plain"
	plain.Items = append([]reader.DatasetItem(nil), pin.Items[:2]...)
	if _, err := idx.PrepareDataset(context.Background(), lookup, plain); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.SearchDataset(context.Background(), lookup, plain, query); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("source declaration leaked into a Dataset that did not select it: %v", err)
	}
}
