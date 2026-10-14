package reader_test

import (
	"fmt"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
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
	public := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/one", AspectName: "public"}
	private := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: public.ObjectID, AspectName: "private"}
	receipt, err := w.Commit("seed", objectmodel.CommitChangeSet{TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: public, PathHint: "public/a.yaml", Value: map[string]any{"body": "public"}},
		{Op: objectmodel.OpPut, Address: private, PathHint: "private/a.yaml", Value: map[string]any{"body": "secret"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	at := receipt.Result.CommitID
	repo, err := reader.NewReader(store).Require(base.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	want, err := repo.ResolveAddress(public, at)
	if err != nil {
		t.Fatal(err)
	}
	guard.block = true
	selected, err := repo.ReadAddress(public, at)
	if err != nil || selected.Value.(map[string]any)["body"] != "public" {
		t.Fatalf("Repository Address read loaded an unselected sibling: %#v %v", selected, err)
	}
	direct, err := repo.ResolveAddress(public, at)
	if err != nil || direct.Digest != want.Digest || direct.Status != objectmodel.StatusResolved {
		t.Fatalf("Repository Address resolution loaded an unselected sibling or changed its digest: %#v %v", direct, err)
	}
	paths := repo.(objectmodel.AddressPathsReader)
	selected, err = paths.ReadAddressAtPaths(public, at, []string{"public/a.yaml", "private/a.yaml"})
	if err != nil || selected.Value.(map[string]any)["body"] != "public" {
		t.Fatalf("selected paths admitted an unselected sibling body: %#v %v", selected, err)
	}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{base.ID(): at}, Items: []reader.DatasetItem{{Repository: base.ID(), Commit: at, Kind: "prefix", Prefix: "public"}}})
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
