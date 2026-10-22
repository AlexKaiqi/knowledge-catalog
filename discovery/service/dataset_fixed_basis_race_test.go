package service

import (
	"context"
	"reflect"
	"testing"

	indeximplementation "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/internal/datasetbridge"
	"kc/internal/testkit"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/observability"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// The provider only locates candidates. Production Index, DatasetSearchExecutor,
// Reader and logical hydration still interpret and read the actual Writer data.
type datasetRaceEngine struct {
	meta       index.Meta
	candidates []index.CandidateRef
	advance    func(index.RetrieveRequest) error
}

func (e *datasetRaceEngine) Probe(retrieval.SearchClause, retrieval.AccessSpec) index.Capability {
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}
func (e *datasetRaceEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	if err := e.advance(req); err != nil {
		return index.CandidatePage{}, err
	}
	start := 0
	if req.Continuation == "private" {
		start = 1
	}
	end := len(e.candidates)
	if req.Search.Limit > 0 && start+req.Search.Limit < end {
		end = start + req.Search.Limit
	}
	page := index.CandidatePage{Candidates: e.candidates[start:end], Exhausted: end == len(e.candidates)}
	if !page.Exhausted {
		page.Continuation = "private"
	}
	return page, nil
}
func (e *datasetRaceEngine) LoadMeta() (index.Meta, error) { return e.meta, nil }
func (e *datasetRaceEngine) Rebuild(_ []index.CompiledDoc, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *datasetRaceEngine) Apply(_ []index.CompiledDoc, _ []objectmodel.ObjectID, meta index.Meta) error {
	e.meta = meta
	return nil
}
func (e *datasetRaceEngine) Count() (int, error) { return len(e.candidates), nil }
func (e *datasetRaceEngine) Close() error        { return nil }

type datasetRaceHydrator struct {
	basis kernel.CommitID
	ids   []objectmodel.ObjectID
	reads int
}

func (h *datasetRaceHydrator) ReadMany(repo objectmodel.Repository, at kernel.CommitID, ids []objectmodel.ObjectID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	if at != h.basis || (len(ids) != 0 && !reflect.DeepEqual(ids, h.ids)) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "read escaped frozen Dataset basis or scope: %s %v", at, ids)
	}
	if len(ids) != 0 {
		h.reads++
	}
	values := make(map[objectmodel.ObjectID]objectmodel.ObjectValue, len(ids))
	for _, id := range ids {
		value, err := repo.Read(id, at)
		if err != nil {
			return nil, err
		}
		values[id] = value
	}
	return values, nil
}
func (h *datasetRaceHydrator) ReadAddress(repo objectmodel.Repository, at kernel.CommitID, addr objectmodel.Address) (objectmodel.ObjectValue, error) {
	return repo.ReadAddress(addr, at)
}

func TestDatasetSearchKeepsReleaseWhenPublicationAdvancesBeforeObjectRead(t *testing.T) {
	s := testkit.NewSetup(t, "kr://regression/dataset-search-race")
	schemaID := objectmodel.ObjectID("schema/article")
	articleID := objectmodel.ObjectID("article/public")
	hiddenID := objectmodel.ObjectID("article/private")
	schema := map[string]any{"entity": "Article", "fields": map[string]any{
		"body": map[string]any{"type": "string"},
	}}
	bodyText := testkit.IndexUnder("public", testkit.IndexText(schemaID, "body"))
	original := map[string]any{"body": "runbook from a"}
	aReceipt, err := s.Writer.Commit("publish-dataset-a", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schemaID}, PathHint: "public/schema.yaml", Value: schema}, bodyText,
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: articleID}, PathHint: "public/article.yaml", SchemaRef: string(schemaID), Value: original},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: hiddenID}, PathHint: "private/article.yaml", SchemaRef: string(schemaID), Value: map[string]any{"body": "runbook outside a's scope"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	a := aReceipt.Result.CommitID
	store := snapshot.NewRegistry()
	if err := store.Add(s.Repo.Snapshot()); err != nil {
		t.Fatal(err)
	}
	// Publication carries Snapshot authority; fixed Object read capabilities
	// come from the real Reader, not the fixture's body-classifying facade.
	sourceReader := reader.NewReader(store)
	cat := testkit.OpenDatasets(t, store)
	if _, err := cat.DefineDataset("articles", 1, []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef, BaseRev: string(a), Path: dataset.MountPath(""), SubPath: "public"}}); err != nil {
		t.Fatal(err)
	}
	openServing := func() (*reader.Serving, error) {
		resolved, err := cat.ResolveDataset("articles")
		if err != nil {
			return nil, err
		}
		pin := datasetbridge.Pin(resolved)
		return reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return sourceReader.Require(id, kernel.ErrObjectRefUnresolved)
		}, pin), nil
	}
	engine := &datasetRaceEngine{candidates: []index.CandidateRef{
		{Repository: s.RepositoryID, Basis: a, ObjectID: articleID},
		{Repository: s.RepositoryID, Basis: a, ObjectID: hiddenID},
	}}
	idx := indeximplementation.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.Rebuild(s.Repo, a); err != nil {
		t.Fatal(err)
	}
	reads := &datasetRaceHydrator{basis: a, ids: []objectmodel.ObjectID{articleID}}
	idx.SetHydrator(reads)
	var b kernel.CommitID
	engine.advance = func(req index.RetrieveRequest) error {
		if req.Spec.Commit != a {
			return kernel.Fail(kernel.ErrPreconditionFailed, "query did not retain its original declaration basis")
		}
		if b != "" {
			return nil
		}
		receipt, err := s.Writer.Commit("publish-dataset-b", objectmodel.CommitChangeSet{
			TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
			BaseCommit: a, ExpectedTargetCommit: a,
			Operations: []objectmodel.Operation{
				{Op: objectmodel.OpRemove, Address: bodyText.Address},
				testkit.IndexUnder("public", testkit.IndexField(schemaID, "body", "keyword")),
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: articleID}, SchemaRef: string(schemaID), Value: map[string]any{"body": "retired at b"}},
			},
		})
		if err != nil {
			return err
		}
		b = receipt.Result.CommitID
		_, err = cat.DefineDataset("articles", 2, []dataset.DatasetSource{{Repository: s.RepositoryID, Selector: snapshot.DefaultRef, BaseRev: string(b), Path: dataset.MountPath(""), SubPath: "private"}})
		return err
	}
	resolves := 0
	executor := DatasetSearchExecutor{
		Authorize: func(context.Context) error { return nil }, Repositories: sourceReader, Projection: fixedSourceSearchProjection{idx, idx},
		Resolve: func(context.Context) (*reader.Serving, LogicalHydrator, error) {
			resolves++
			serving, err := openServing()
			if err != nil {
				return nil, nil, err
			}
			return serving, liveservice.Open(serving, nil, observability.IdentityContext{}), nil
		},
		Deliver: func(_ context.Context, hit retrieval.ObjectHit) (retrieval.ObjectHit, error) { return hit, nil },
	}
	result, err := executor.Execute(context.Background(), retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil {
		t.Fatal(err)
	}
	if resolves != 1 || reads.reads != 1 || b == "" || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != b ||
		result.Completeness != retrieval.CompletenessComplete || result.Continuation != "" || len(result.Hits) != 1 ||
		!reflect.DeepEqual(result.SearchView.Snapshots, map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: a}) {
		t.Fatalf("query crossed publication or scope: resolves=%d reads=%d a=%s b=%s result=%#v", resolves, reads.reads, a, b, result)
	}
	hit := result.Hits[0]
	if hit.Object.Address.ObjectID != articleID || hit.Object.Commit != a || hit.Version.DeclarationCommit != a || !reflect.DeepEqual(hit.Object.Value, original) {
		t.Fatalf("Dataset candidate at a was combined with b: %#v", hit)
	}
	latest, err := openServing()
	if err != nil || latest.Pin().Revision != 2 || latest.Pin().Repositories[s.RepositoryID] != b {
		t.Fatalf("concurrent publication did not actually change Dataset: %v", err)
	}
	if inside, err := latest.Contains(s.RepositoryID, articleID); err != nil || inside {
		t.Fatalf("new Dataset still includes old scope: %v %v", inside, err)
	}
	if inside, err := latest.Contains(s.RepositoryID, hiddenID); err != nil || !inside {
		t.Fatalf("new Dataset did not change its scope: %v %v", inside, err)
	}
	valueB, err := s.Repo.Read(articleID, b)
	if err != nil || valueB.Value.(map[string]any)["body"] != "retired at b" {
		t.Fatalf("concurrent object change did not persist: %#v %v", valueB, err)
	}
	specB, err := retrieval.AccessSpecForRepository(s.Repo, b)
	if err != nil {
		t.Fatal(err)
	}
	accessErr := retrieval.CheckSearch(retrieval.SearchOf(retrieval.SearchMATCH("runbook")), specB)
	if kernel.CodeOf(accessErr) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("concurrent declaration change did not reject text access for the expected reason: %#v %v", specB, accessErr)
	}
}
