package reader_test

import (
	"testing"

	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type countingRepository struct {
	snapshot.Store
	snapshot.TreeStore
	listCalls int
	readCalls int
}

type gitLikeEmptyLocatorStore struct {
	snapshot.Store
	snapshot.TreeStore
	snapshot.DirectoryReader
}

func (s *gitLikeEmptyLocatorStore) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	if request.Directory == repofile.LocatorObjectDirectory {
		return snapshot.DirectoryPage{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
			"empty directories are absent")
	}
	return s.DirectoryReader.ReadDirectory(request)
}

func (r *countingRepository) ObjectUnitPaths(objectID knowledge.ObjectID, commit kernel.CommitID) ([]string, error) {
	return r.Store.(knowledge.UnitLocator).ObjectUnitPaths(objectID, commit)
}

func (r *countingRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	r.listCalls++
	return r.TreeStore.ListFiles(commit)
}

func (r *countingRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	r.readCalls++
	return r.TreeStore.ReadFile(path, commit)
}

func TestKnowledgeServiceBatchHydratesOneTreeWithoutCrossRequestObjectCache(t *testing.T) {
	base := testkit.MakeRepository(t, "kr://acme/public/core")
	var err error
	root, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingRepository{Store: base, TreeStore: base}
	registry := snapshot.NewRegistry()
	if err := registry.Add(counting); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := w.Commit("seed", knowledge.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "metric/gmv"}, Value: map[string]any{"name": "GMV"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "table/orders", AspectName: "structure"}, Value: map[string]any{"name": "orders"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit := receipt.Result.CommitID
	counting.listCalls, counting.readCalls = 0, 0
	service := reader.NewReader(registry)
	repo, err := service.Require(base.ID(), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	batch, ok := repo.(knowledge.BatchReadStore)
	if !ok {
		t.Fatal("knowledge service repository must expose batch hydration")
	}
	values, err := batch.ReadMany([]knowledge.ObjectID{"metric/gmv", "table/orders"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values["metric/gmv"].Commit != commit || values["table/orders"].Commit != commit {
		t.Fatalf("unexpected pinned values: %#v", values)
	}
	if counting.listCalls != 0 {
		t.Fatalf("batch exact read must not scan a Snapshot tree, got %d scans", counting.listCalls)
	}

	counting.listCalls, counting.readCalls = 0, 0
	if _, err := repo.Read("metric/gmv", commit); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Read("table/orders", commit); err != nil {
		t.Fatal(err)
	}
	if counting.listCalls != 0 || counting.readCalls == 0 {
		t.Fatalf("each read must use locator + authority bytes: list=%d read=%d", counting.listCalls, counting.readCalls)
	}
	first := values["metric/gmv"]
	first.Value.(map[string]any)["name"] = "mutated by caller"
	again, err := repo.Read("metric/gmv", commit)
	if err != nil {
		t.Fatal(err)
	}
	if again.Value.(map[string]any)["name"] != "GMV" {
		t.Fatalf("fresh authority interpretation retained caller mutation: %#v", again.Value)
	}
}

func TestObjectIdentityPageTreatsAbsentCompletedLocatorDirectoryAsEmpty(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://reader/git-empty-locators")
	stores := snapshot.NewRegistry()
	if err := stores.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := raw.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := w.Commit("seed-only-object", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{
				Kind: knowledge.KindEntity, ObjectID: "policy/only",
			}, Value: map[string]any{"name": "only"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := w.Commit("remove-only-object", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: seed.Result.CommitID, ExpectedTargetCommit: seed.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{
				Kind: knowledge.KindEntity, ObjectID: "policy/only",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gitLike := &gitLikeEmptyLocatorStore{
		Store: raw, TreeStore: raw.(snapshot.TreeStore),
		DirectoryReader: raw.(snapshot.DirectoryReader),
	}
	wrappedStores := snapshot.NewRegistry()
	if err := wrappedStores.Add(gitLike); err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(wrappedStores).Require(raw.ID(), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	page, err := repo.(knowledge.SnapshotObjectPager).ObjectIDsPage(removed.Result.CommitID, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if !page.Exhausted || len(page.ObjectIDs) != 0 || page.Continuation != "" {
		t.Fatalf("empty completed locator page = %#v", page)
	}
}

func TestTreeProviderSchemaIndexTracksSchemaWithoutRepositoryScan(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://reader/schema-index")
	stores := snapshot.NewRegistry()
	if err := stores.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := raw.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	published, err := w.Commit("publish-schema", knowledge.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{
				Kind: knowledge.KindEntity, ObjectID: "schema/note",
			}, Value: map[string]any{
				"entity": "Note", "pattern": "record",
				"fields": map[string]any{"body": map[string]any{
					"type": "string", "access": []any{"text"},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := reader.NewReader(stores).Require(raw.ID(), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := repo.(knowledge.SchemaStore).SchemaObjectIDs(published.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "schema/note" {
		t.Fatalf("schema index = %v", ids)
	}
}

func TestTreeRepositoryResolvePreservesRelationAddress(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://reader/relation")
	receipt, err := setup.Writer.Commit("relation", knowledge.ChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut,
			Address: knowledge.Address{
				Kind: knowledge.KindRelation, ObjectID: "relation/contains",
			},
			Value: map[string]any{
				"relationId": "relation/contains", "relationType": "contains", "direction": "DIRECTED",
				"endpoints": []any{
					map[string]any{"role": "container", "objectRef": map[string]any{"repository": string(setup.RepositoryID), "object": "dataset/A"}},
					map[string]any{"role": "member", "objectRef": map[string]any{"repository": string(setup.RepositoryID), "object": "dataset/B"}},
				},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := setup.Repo.Resolve("relation/contains", receipt.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Address.Kind != knowledge.KindRelation {
		t.Fatalf("relation resolved as %s: %#v", resolution.Address.Kind, resolution)
	}
}

func TestTreeRepositoryLogIncludesRemovalRevision(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://reader/removal-log")
	first, err := setup.Writer.Commit("put", knowledge.ChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: setup.RootCommitID, ExpectedTargetCommit: setup.RootCommitID,
		Operations: testkit.PutEntity("policy/A", map[string]any{"version": 1}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := setup.Writer.Commit("remove", knowledge.ChangeSet{
		TargetRepository: setup.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.CommitID, ExpectedTargetCommit: first.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	history, err := setup.Repo.Log("policy/A", second.Result.CommitID, knowledge.ObjectLogQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Commit != second.Result.CommitID ||
		history[0].Status != knowledge.StatusRemoved || history[1].Commit != first.Result.CommitID {
		t.Fatalf("removal history = %#v", history)
	}
}
