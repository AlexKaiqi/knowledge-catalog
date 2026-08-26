package reader_test

import (
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
	"kc/snapshot/filegit"
)

type countingRepository struct {
	snapshot.Store
	snapshot.TreeStore
	listCalls int
	readCalls int
}

func (r *countingRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	r.listCalls++
	return r.TreeStore.ListFiles(commit)
}

func (r *countingRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	r.readCalls++
	return r.TreeStore.ReadFile(path, commit)
}

func TestKnowledgeServiceBatchHydratesOneTreeAndReusesCanonicalObjects(t *testing.T) {
	base, err := filegit.NewFileGit(t.TempDir(), "kr://acme/public/core")
	if err != nil {
		t.Fatal(err)
	}
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
	if counting.listCalls != 1 {
		t.Fatalf("one batch must scan one Snapshot tree, got %d scans", counting.listCalls)
	}

	counting.listCalls, counting.readCalls = 0, 0
	if _, err := repo.Read("metric/gmv", commit); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Read("table/orders", commit); err != nil {
		t.Fatal(err)
	}
	if counting.listCalls != 0 || counting.readCalls != 0 {
		t.Fatalf("Canonical cache miss: list=%d read=%d", counting.listCalls, counting.readCalls)
	}
}
