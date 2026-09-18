package reader_test

import (
	"fmt"
	"testing"

	"kc/internal/repofile"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// Expose only the Snapshot ports, as a tree authority does in production. In
// particular, do not forward testkit's UnitLocator: this exercises the Reader's
// versioned per-object locator rather than a test-only in-memory shortcut.
type locatorCountingRepository struct {
	snapshot.Store
	snapshot.TreeStore
	reads     map[string]int
	readBytes int
	commits   []kernel.CommitID
	listCalls int
}

func (r *locatorCountingRepository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	r.reads[path]++
	r.commits = append(r.commits, commit)
	raw, err := r.TreeStore.ReadFile(path, commit)
	if err == nil {
		r.readBytes += len(raw)
	}
	return raw, err
}

func TestSingleObjectReadDecodeBytesAreIndependentOfRepositorySize(t *testing.T) {
	measure := func(objects int) (reads, bytes, lists int) {
		t.Helper()
		raw := testkit.MakeTreeStore(t, fmt.Sprintf("kr://reader/bounded/%d", objects))
		registry := snapshot.NewRegistry()
		if err := registry.Add(raw); err != nil {
			t.Fatal(err)
		}
		w, err := writer.NewWriter(registry, nil)
		if err != nil {
			t.Fatal(err)
		}
		root := testkit.MustHead(t, raw, snapshot.DefaultRef)
		operations := make([]knowledge.Operation, 0, objects)
		for i := 0; i < objects; i++ {
			operations = append(operations, testkit.PutEntity(
				fmt.Sprintf("object/%06d", i), map[string]any{"value": i}, "")[0])
		}
		receipt, err := w.Commit("seed", knowledge.ChangeSet{
			TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: root, ExpectedTargetCommit: root, Operations: operations,
		})
		if err != nil {
			t.Fatal(err)
		}
		counting := &locatorCountingRepository{
			Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{},
		}
		repo, err := reader.NewReader(nil).Wrap(counting, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Read("object/000000", receipt.Result.CommitID); err != nil {
			t.Fatal(err)
		}
		return len(counting.commits), counting.readBytes, counting.listCalls
	}

	smallReads, smallBytes, smallLists := measure(10)
	largeReads, largeBytes, largeLists := measure(1000)
	if smallLists != 0 || largeLists != 0 {
		t.Fatalf("single-object READ scanned trees: small=%d large=%d", smallLists, largeLists)
	}
	if smallReads != largeReads || smallBytes != largeBytes {
		t.Fatalf("single-object READ grew with repository: reads %d→%d, bytes %d→%d",
			smallReads, largeReads, smallBytes, largeBytes)
	}
}

func (r *locatorCountingRepository) ListFiles(commit kernel.CommitID) ([]string, error) {
	r.listCalls++
	return r.TreeStore.ListFiles(commit)
}

func TestReadManyLoadsOnlyRequestedObjectLocatorsAndUnits(t *testing.T) {
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
	counting := &locatorCountingRepository{Store: s.Repo, TreeStore: s.Repo, reads: map[string]int{}}
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
		for _, id := range []knowledge.ObjectID{"metric/gmv", "table/orders", "missing/object"} {
			if got := counting.reads[repofile.ObjectLocatorPath(id)]; got != call {
				t.Fatalf("locator %s reads=%d, want one per batch (%d)", id, got, call)
			}
		}
		if counting.reads[knowledge.RepositoryReadmePath] != call {
			t.Fatalf("README convention reads=%d, want one per batch (%d)", counting.reads[knowledge.RepositoryReadmePath], call)
		}
		if counting.reads[repofile.LocatorManifestPath] != 0 {
			t.Fatalf("complete locator layout fell back to whole manifest: %#v", counting.reads)
		}
		if len(counting.reads) != 8 {
			t.Fatalf("want three object locators, completeness marker, three unit paths, and README.md, got %#v", counting.reads)
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
	before := len(counting.commits)
	if _, err := batch.ReadMany([]knowledge.ObjectID{"", ""}, commit); err != nil {
		t.Fatal(err)
	}
	if len(counting.commits) != before {
		t.Fatal("empty batch performed authority I/O")
	}
	values, err := batch.ReadMany([]knowledge.ObjectID{"metric/gmv"}, updated.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if values["metric/gmv"].Value.(map[string]any)["name"] != "updated" ||
		counting.reads[repofile.ObjectLocatorPath("metric/gmv")] != 3 {
		t.Fatalf("new basis reused stale interpretation: %#v, reads=%#v", values, counting.reads)
	}
}
