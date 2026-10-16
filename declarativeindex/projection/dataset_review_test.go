package projection_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

type datasetReviewEngine struct {
	datasetTestEngine
	metaReads int
}

func (e *datasetReviewEngine) LoadMeta() (ports.Meta, error) {
	e.metaReads++
	return e.meta, nil
}
func (e *datasetReviewEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) ports.Capability {
	return ports.Capability{Guarantee: ports.GuaranteeExact, Coverage: 1}
}
func (e *datasetReviewEngine) Retrieve(request ports.RetrieveRequest) (ports.CandidatePage, error) {
	e.queries++
	return ports.CandidatePage{Exhausted: true, Candidates: []ports.CandidateRef{{Repository: request.Spec.Repository, Basis: e.meta.Basis, ObjectID: "article/not-selected"}}}, nil
}

func datasetReviewFixture(t *testing.T) (objectmodel.Repository, reader.DatasetPin) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://dataset-review/articles")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexText("schema/article", "body"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/selected"}, SchemaRef: "schema/article", Value: map[string]any{"body": "runbook"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return repo, reader.DatasetPin{SetID: "articles", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: commit, Kind: "prefix"}}}
}

func TestDatasetCorpusRejectsUnknownObjectAtItsOwnPhysicalBasis(t *testing.T) {
	repo, pin := datasetReviewFixture(t)
	engine := &datasetReviewEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("bad Dataset corpus coordinate became a search result: complete=%s hits=%d queries=%d error=%v", result.Completeness, len(result.Hits), engine.queries, err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("bad coordinate produced a Canonical hit: %#v", result)
	}
}

func TestCancelledDatasetOperationsDoNotOpenReadOrBuild(t *testing.T) {
	repo, pin := datasetReviewFixture(t)
	for _, operation := range []string{"search", "semantic", "prepare"} {
		t.Run(operation, func(t *testing.T) {
			engine := &datasetReviewEngine{}
			opens, authorityReads := 0, 0
			idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { opens++; return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { authorityReads++; return repo, nil }
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var err error
			switch operation {
			case "search":
				_, err = idx.SearchDataset(ctx, lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
			case "semantic":
				_, err = idx.SemanticDatasetWindow(ctx, lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook")), []float32{1})
			case "prepare":
				_, err = idx.PrepareDataset(ctx, lookup, pin)
			}
			if !errors.Is(err, context.Canceled) || opens != 0 || authorityReads != 0 || engine.metaReads != 0 || engine.queries != 0 || len(engine.docs) != 0 {
				t.Fatalf("pre-cancelled %s touched retrieval state: open=%d authority=%d metadata=%d queries=%d docs=%d error=%v", operation, opens, authorityReads, engine.metaReads, engine.queries, len(engine.docs), err)
			}
		})
	}
}

func TestIndexDeclarationProtocolAspectIsFixedByWriter(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset-review/index-aspect")
	base := testkit.MustHead(t, repo, snapshot.DefaultRef)
	member := testkit.IndexText("schema/article", "body")
	member.Address.AspectName = "search"
	_, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}, member,
	}})
	if kernel.CodeOf(err) != kernel.ErrSchemaInstanceInvalid || !strings.Contains(err.Error(), "requires aspect index") {
		t.Fatalf("real Writer did not enforce the protocol aspect: %v", err)
	}
	if head := testkit.MustHead(t, repo, snapshot.DefaultRef); head != base {
		t.Fatalf("rejected declaration advanced HEAD: %s", head)
	}
}
