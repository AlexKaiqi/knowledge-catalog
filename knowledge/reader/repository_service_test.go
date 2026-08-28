package reader_test

import (
	"testing"

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
