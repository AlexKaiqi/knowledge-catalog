package reader_test

import (
	"fmt"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type scopedTreeGuard struct {
	countingRepository
	block bool
}

func (s *scopedTreeGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if s.block && strings.HasPrefix(path, "private/") {
		return nil, fmt.Errorf("read outside published file scope: %s", path)
	}
	return s.countingRepository.ReadFile(path, at)
}

func TestDatasetExactAddressReadsOnlySelectedTreePaths(t *testing.T) {
	base := testkit.MakeRepository(t, "kr://dataset/exact")
	guard := &scopedTreeGuard{countingRepository: countingRepository{Store: base, TreeStore: base}}
	store := snapshot.NewRegistry()
	if err := store.Add(guard); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, base, snapshot.DefaultRef)
	public := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "service/one", AspectName: "public"}
	private := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: public.ObjectID, AspectName: "private"}
	receipt, err := w.Commit("seed", knowledge.CommitChangeSet{TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: []knowledge.Operation{
		{Op: knowledge.OpPut, Address: public, PathHint: "public/a.yaml", Value: map[string]any{"body": "public"}},
		{Op: knowledge.OpPut, Address: private, PathHint: "private/a.yaml", Value: map[string]any{"body": "secret"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	at := receipt.Result.CommitID
	repo, err := reader.NewReader(store).Require(base.ID(), kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	want, err := repo.ResolveAddress(public, at)
	if err != nil {
		t.Fatal(err)
	}
	guard.block = true
	scope := reader.Open(func(kernel.RepositoryID) (knowledge.Repository, error) { return repo, nil }, reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{base.ID(): at}, Items: []reader.DatasetItem{{Repository: base.ID(), Commit: at, Kind: "prefix", Prefix: "public"}}})
	rows, err := scope.ReadAddress(public)
	if err != nil || len(rows) != 1 || rows[0].Value.(map[string]any)["body"] != "public" {
		t.Fatalf("exact read: %#v %v", rows, err)
	}
	resolved, err := scope.ResolveAddress(public)
	if err != nil || len(resolved) != 1 || resolved[0].Digest != want.Digest {
		t.Fatalf("unit digest changed: %#v %v", resolved, err)
	}
	if rows, err := scope.ReadAddress(private); err != nil || len(rows) != 0 {
		t.Fatalf("private unit escaped: %#v %v", rows, err)
	}
}
