package dataset_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

type catalogMemChanges struct{ *catalogMem }

func (s catalogMemChanges) ChangedPaths(from, to kernel.CommitID) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSnapshots(from, to) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit missing")
	}
	seen := map[string]bool{}
	var out []string
	for _, files := range []map[string][]byte{s.commits[from], s.commits[to]} {
		for path := range files {
			if !seen[path] && !bytes.Equal(s.commits[from][path], s.commits[to][path]) {
				out = append(out, path)
				seen[path] = true
			}
		}
	}
	return out, nil
}

type catalogHistoryPage struct{ *catalogMem }

func (s catalogHistoryPage) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return s.catalogMem.CommitHistory(commit, min(limit, 1000))
}

func TestSnapshotCatalogDatasetAuditCrossesBackendHistoryPage(t *testing.T) {
	authority := newCatalogMem("kr://audit/catalog")
	backend := catalogHistoryPage{authority}
	reg, err := catalog.CreateSnapshotRegistry(backend, "")
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(newCatalogMem("table-meta")); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(stores, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.DefineDataset("orders", 1, []catalog.DatasetSource{{Repository: "table-meta", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	want, _ := reg.Head()
	for i := 0; i < 1100; i++ {
		head, _ := authority.Head(snapshot.DefaultRef)
		if _, err := authority.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: authority.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Changes: []snapshot.TreeChange{{Path: "noise.txt", Content: []byte(fmt.Sprint(i))}}}); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := catalog.OpenSnapshotRegistry(backend, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err = catalog.NewService(stores, reopened)
	if err != nil {
		t.Fatal(err)
	}
	hist, err := cat.Log(catalog.LogQuery{Dataset: "orders", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.Commits) != 1 || hist.Commits[0].Commit != want {
		t.Fatalf("page boundary invented/truncated Dataset history: %#v, want %s", hist, want)
	}
}

func (s catalogMemChanges) hasSnapshots(from, to kernel.CommitID) bool {
	_, a := s.commits[from]
	_, b := s.commits[to]
	return a && b
}

func TestSnapshotCatalogDatasetAuditTracksActualChangingCommits(t *testing.T) {
	for _, accelerated := range []bool{false, true} {
		name := "tree comparison"
		if accelerated {
			name = "changed paths"
		}
		t.Run(name, func(t *testing.T) {
			authority := newCatalogMem("kr://audit/catalog")
			var backend snapshot.Store = authority
			if accelerated {
				backend = catalogMemChanges{authority}
			}
			reg, err := catalog.CreateSnapshotRegistry(backend, "")
			if err != nil {
				t.Fatal(err)
			}
			stores := snapshot.NewRegistry()
			if err := stores.Add(newCatalogMem("table-meta")); err != nil {
				t.Fatal(err)
			}
			cat, err := catalog.NewService(stores, reg)
			if err != nil {
				t.Fatal(err)
			}
			if err := cat.RegisterRepository("table-meta"); err != nil {
				t.Fatal(err)
			}
			sources := []catalog.DatasetSource{{Repository: "table-meta", Selector: snapshot.DefaultRef, Path: catalog.MountPath("tables")}}
			if _, err := cat.DefineDataset("orders", 1, sources); err != nil {
				t.Fatal(err)
			}
			v1, _ := reg.Head()
			if _, err := cat.DefineDataset("unrelated", 1, sources); err != nil {
				t.Fatal(err)
			}
			if _, err := cat.DefineDataset("orders", 2, sources); err != nil {
				t.Fatal(err)
			}
			v2, _ := reg.Head()
			if err := cat.RegisterRepository("another"); err != nil {
				t.Fatal(err)
			}
			if err := cat.RetireDataset("orders"); err != nil {
				t.Fatal(err)
			}
			retired, _ := reg.Head()
			if err := cat.UnregisterRepository("another"); err != nil {
				t.Fatal(err)
			}
			head, _ := reg.Head()
			want := []string{retired, v2, v1}
			for _, limit := range []int{1, 2, 20} {
				queryWant := want
				if limit < len(want) {
					queryWant = want[:limit]
				}
				hist, err := cat.Log(catalog.LogQuery{Dataset: "orders", Limit: limit})
				if err != nil {
					t.Fatal(err)
				}
				var got []string
				for _, entry := range hist.Commits {
					got = append(got, entry.Commit)
				}
				if !reflect.DeepEqual(got, queryWant) {
					t.Fatalf("limit %d: got %v want %v", limit, got, queryWant)
				}
			}
			all, err := cat.Log(catalog.LogQuery{Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(all.Commits) != 1 || all.Commits[0].Commit != head {
				t.Fatal("whole Service audit lost latest membership change", all)
			}
			missing, err := cat.Log(catalog.LogQuery{Dataset: "missing", Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if len(missing.Commits) != 0 {
				t.Fatal("unrelated commits attributed to absent Dataset", missing)
			}
			reopened, err := catalog.OpenSnapshotRegistry(backend, "")
			if err != nil {
				t.Fatal(err)
			}
			recovered, err := catalog.NewService(stores, reopened)
			if err != nil {
				t.Fatal(err)
			}
			hist, err := recovered.Log(catalog.LogQuery{Dataset: "orders", Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			var recoveredCommits []string
			for _, entry := range hist.Commits {
				recoveredCommits = append(recoveredCommits, entry.Commit)
			}
			if !reflect.DeepEqual(recoveredCommits, want) {
				t.Fatalf("recovery changed audit: %v", recoveredCommits)
			}
		})
	}
}

type catalogShortHistory struct{ *catalogMem }

func (s catalogShortHistory) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return s.catalogMem.CommitHistory(commit, min(limit, 3))
}

func TestSnapshotCatalogAuditAcrossShortPagesPreservesEachAcceptedCommit(t *testing.T) {
	authority := newCatalogMem("kr://audit/catalog")
	reg, err := catalog.CreateSnapshotRegistry(catalogShortHistory{authority}, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := catalogFromRegistry(t, reg)
	initial, _ := reg.Head()
	want := []string{initial}
	for i := 0; i < 8; i++ {
		if err := cat.RegisterRepository(kernel.RepositoryID(fmt.Sprintf("source-%d", i))); err != nil {
			t.Fatal(err)
		}
		head, _ := reg.Head()
		want = append([]string{head}, want...)
	}
	hist, err := cat.Log(catalog.LogQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range hist.Commits {
		got = append(got, entry.Commit)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("short provider pages duplicated/lost accepted history: got %v want %v", got, want)
	}
}
