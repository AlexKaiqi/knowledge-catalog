package projection_test

import (
	"context"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/maintenance"
	"kc/object/reader"
	"kc/snapshot"
)

type emptyScopeSource struct {
	objectmodel.Repository
	objectmodel.SchemaStore
	objectmodel.SchemaReferrerLocator
	objectmodel.BindingLocator
	objectmodel.UnitLocator
	scanner maintenance.SnapshotScanner
	scans   int
}

func (s *emptyScopeSource) ScanSnapshotPage(at kernel.CommitID, request maintenance.ScanRequest) (maintenance.ScanPage, error) {
	s.scans++
	return s.scanner.ScanSnapshotPage(at, request)
}

func TestDatasetPreparationProvesEmptyScopeWithoutChangingSourceMetadata(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/empty-source")
	base, _ := repo.Head(snapshot.DefaultRef)
	source := newEmptyScopeSource(t, repo)
	pin := reader.DatasetPin{SetID: "empty-source", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): base}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return source, nil }
	engine := &datasetTestEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatalf("empty fixed source could not be prepared without object metadata: %v", err)
	}
	if source.scans != 1 || len(engine.docs) != 0 {
		t.Fatalf("empty proof did not reuse the explicit preparation scan: scans=%d docs=%#v", source.scans, engine.docs)
	}
	if head, _ := repo.Head(snapshot.DefaultRef); head != base {
		t.Fatalf("preparing an empty Dataset wrote the source: %s -> %s", base, head)
	}
	files, err := repo.ListFiles(base)
	if err != nil || len(files) != 0 {
		t.Fatalf("preparation manufactured object metadata: files=%v error=%v", files, err)
	}
	if _, err := idx.CheckDatasetProjection(lookup, pin); err != nil {
		t.Fatalf("prepared empty basis could not be checked exactly: %v", err)
	}
	if _, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("anything"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("empty source acquired undeclared query access: %v", err)
	}
	if source.scans != 1 || engine.queries != 0 {
		t.Fatalf("consumption scanned or queried the unindexed source: scans=%d queries=%d", source.scans, engine.queries)
	}
}

func TestDatasetEmptyProofCannotHideNonemptySourceWithMissingDeclarationMetadata(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/incomplete-source")
	base, _ := repo.Head(snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/note"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Note", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexUnder("private/indexes", testkit.IndexText("schema/note", "body")),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/one"}, PathHint: "public/note.yaml", SchemaRef: "schema/note", Value: map[string]any{"body": "selected content"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	broken, err := repo.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Changes: []snapshot.TreeChange{
		{Path: repofile.LocatorReferrersCompletePath, Remove: true},
		{Path: repofile.SchemaReferrersPath(objectmodel.IndexDeclarationSchemaV1), Remove: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	source := newEmptyScopeSource(t, repo)
	empty := testkit.MakeRepository(t, "kr://dataset/also-empty")
	emptyAt, _ := empty.Head(snapshot.DefaultRef)
	emptySource := newEmptyScopeSource(t, empty)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id == empty.ID() {
			return emptySource, nil
		}
		return source, nil
	}
	for _, selected := range []reader.DatasetItem{
		{Repository: repo.ID(), Commit: broken, Kind: "prefix", Prefix: "public"},
		{Repository: repo.ID(), Commit: broken, Kind: "file", File: "private/schema.yaml", Target: "schema.yaml"},
		{Repository: repo.ID(), Commit: broken, Kind: "prefix", Prefix: "private/indexes"},
	} {
		pin := reader.DatasetPin{SetID: "incomplete-source", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): broken, empty.ID(): emptyAt}, Items: []reader.DatasetItem{selected, {Repository: empty.ID(), Commit: emptyAt, Kind: "prefix"}}}
		engine := &datasetTestEngine{}
		idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
		t.Cleanup(func() { _ = idx.Close() })
		if _, err := idx.PrepareDataset(context.Background(), lookup, pin); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("missing declaration metadata became an empty contract for selected unit %#v: %v", selected, err)
		}
		if engine.meta.State == ports.ProjectionStateReady || len(engine.docs) != 0 {
			t.Fatalf("failed preparation published a partial corpus: %#v", engine)
		}
	}
}

func TestDatasetPreparationCombinesDeclaredContentWithEmptySources(t *testing.T) {
	for _, emptyFirst := range []bool{false, true} {
		name := "empty-last"
		if emptyFirst {
			name = "empty-first"
		}
		t.Run(name, func(t *testing.T) {
			emptyID, contentID := "kr://dataset/z-empty", "kr://dataset/a-content"
			if emptyFirst {
				emptyID, contentID = "kr://dataset/a-empty", "kr://dataset/z-content"
			}
			empty, content := testkit.MakeRepository(t, emptyID), testkit.MakeRepository(t, contentID)
			emptyAt, _ := empty.Head(snapshot.DefaultRef)
			base, _ := content.Head(snapshot.DefaultRef)
			at, err := content.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: content.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/note"}, Value: map[string]any{"entity": "Note", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
				testkit.IndexText("schema/note", "body"),
				{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/one"}, SchemaRef: "schema/note", Value: map[string]any{"body": "selected content"}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			emptySource, contentSource := newEmptyScopeSource(t, empty), newEmptyScopeSource(t, content)
			lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				if id == empty.ID() {
					return emptySource, nil
				}
				return contentSource, nil
			}
			pin := reader.DatasetPin{SetID: name, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{empty.ID(): emptyAt, content.ID(): at}}
			pin.Items = reader.WholeRepositoryItems(pin.Repositories)
			engine := &datasetTestEngine{}
			idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
				t.Fatalf("empty source prevented declared content preparation: %v", err)
			}
			result, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("selected")))
			if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Repository != content.ID() || result.Hits[0].Object.Commit != at {
				t.Fatalf("mixed-source preparation lost declared content or its fixed basis: %#v %v", result, err)
			}
			if emptySource.scans != 1 || contentSource.scans != 1 {
				t.Fatalf("mixed-source preparation or consumption rescanned sources: empty=%d content=%d", emptySource.scans, contentSource.scans)
			}
			if head, _ := empty.Head(snapshot.DefaultRef); head != emptyAt {
				t.Fatal("preparation wrote empty source metadata")
			}
			if head, _ := content.Head(snapshot.DefaultRef); head != at {
				t.Fatal("preparation changed the content source")
			}
			files, err := empty.ListFiles(emptyAt)
			if err != nil || len(files) != 0 {
				t.Fatalf("empty source changed: files=%v error=%v", files, err)
			}
		})
	}
}

func newEmptyScopeSource(t *testing.T, repo *testkit.ObjectRepository) *emptyScopeSource {
	t.Helper()
	raw := repo.Repository
	scanner, err := maintenance.RequireScanner(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &emptyScopeSource{Repository: raw, SchemaStore: raw.(objectmodel.SchemaStore), SchemaReferrerLocator: raw.(objectmodel.SchemaReferrerLocator), BindingLocator: raw.(objectmodel.BindingLocator), UnitLocator: raw.(objectmodel.UnitLocator), scanner: scanner}
}
