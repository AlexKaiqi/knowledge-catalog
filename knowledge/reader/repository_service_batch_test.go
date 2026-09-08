package reader_test

import (
	"testing"

	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

// Expose only the Snapshot ports, as a tree authority does in production. In
// particular, do not forward testkit's UnitLocator: this exercises the Reader's
// versioned manifest locator rather than a test-only in-memory shortcut.
type manifestCountingRepository struct {
	snapshot.Store
	snapshot.TreeStore
	reads     map[string]int
	commits   []kernel.CommitID
	listCalls int
}

func (r *manifestCountingRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	r.reads[path]++
	r.commits = append(r.commits, commit)
	return r.TreeStore.ReadFile(path, commit)
}

func (r *manifestCountingRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	r.listCalls++
	return r.TreeStore.ListFiles(commit)
}

func TestReadManyLoadsOneManifestPerBasisAndCall(t *testing.T) {
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("seed-batch", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "metric/gmv"}, Value: map[string]any{"name": "GMV"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "table/orders", AspectName: "structure"}, Value: map[string]any{"name": "orders"}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindMember, ObjectID: "table/orders", AspectName: "columns", MemberKey: "id"}, Value: map[string]any{"name": "order_id"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commit := published.Result.CommitID
	updated, err := s.Writer.Commit("advance-after-basis", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: commit, ExpectedTargetCommit: commit,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "metric/gmv"}, Value: map[string]any{"name": "updated"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	counting := &manifestCountingRepository{Store: s.Repo, TreeStore: s.Repo, reads: map[string]int{}}
	repo, err := reader.NewReader(nil).Wrap(counting, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	batch := repo.(knowledge.BatchReadStore)
	ids := []knowledge.ObjectID{"metric/gmv", "table/orders", "", "metric/gmv", "missing/object"}
	for call := 1; call <= 2; call++ {
		values, err := batch.ReadMany(ids, commit)
		if err != nil {
			t.Fatal(err)
		}
		if len(values) != 2 || values["metric/gmv"].Commit != commit || values["table/orders"].Commit != commit {
			t.Fatalf("unexpected fixed-basis values: %#v", values)
		}
		if values["metric/gmv"].Value.(map[string]any)["name"] != "GMV" {
			t.Fatalf("batch followed current HEAD or retained caller mutation: %#v", values)
		}
		if got := counting.reads[repofile.LocatorManifestPath]; got != call {
			t.Fatalf("manifest reads=%d, want one per batch (%d), independent of object count", got, call)
		}
		if len(counting.reads) != 4 {
			t.Fatalf("want only manifest and three unique unit paths, got %#v", counting.reads)
		}
		for path, reads := range counting.reads {
			if reads != call {
				t.Fatalf("path %s read %d times, want %d", path, reads, call)
			}
		}
		values["metric/gmv"].Value.(map[string]any)["name"] = "caller mutation"
	}
	if counting.listCalls != 0 {
		t.Fatalf("batch must not scan Snapshot: %d ListFiles calls", counting.listCalls)
	}
	for _, at := range counting.commits {
		if at != commit {
			t.Fatalf("batch mixed commit %s with requested basis %s", at, commit)
		}
	}
	if _, err := batch.ReadMany([]knowledge.ObjectID{"", ""}, commit); err != nil {
		t.Fatal(err)
	}
	if got := counting.reads[repofile.LocatorManifestPath]; got != 2 {
		t.Fatalf("empty batch performed authority I/O: %d manifest reads", got)
	}
	values, err := batch.ReadMany([]knowledge.ObjectID{"metric/gmv"}, updated.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if values["metric/gmv"].Value.(map[string]any)["name"] != "updated" || counting.reads[repofile.LocatorManifestPath] != 3 {
		t.Fatalf("new basis reused stale interpretation: %#v, reads=%#v", values, counting.reads)
	}
}
