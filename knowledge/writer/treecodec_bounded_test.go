package writer_test

import (
	"fmt"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type countingTreeAuthority struct {
	snapshot.Store
	snapshot.TreeStore
	reads     int
	readBytes int
	lists     int
}

func (s *countingTreeAuthority) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	raw, err := s.TreeStore.ReadFile(path, commit)
	if err == nil {
		s.readBytes += len(raw)
	}
	s.reads++
	return raw, err
}

func (s *countingTreeAuthority) ListFiles(commit kernel.CommitID) ([]string, error) {
	s.lists++
	return s.TreeStore.ListFiles(commit)
}

func TestSingleObjectPutCostIsIndependentOfRepositorySize(t *testing.T) {
	measure := func(objects int) (reads, bytes, lists int) {
		t.Helper()
		raw := testkit.MakeTreeStore(t, fmt.Sprintf("kr://writer/bounded/%d", objects))
		tree := raw.(snapshot.TreeStore)
		counting := &countingTreeAuthority{Store: raw, TreeStore: tree}
		registry := snapshot.NewRegistry()
		if err := registry.Add(counting); err != nil {
			t.Fatal(err)
		}
		w, err := writer.NewWriter(registry, nil)
		if err != nil {
			t.Fatal(err)
		}
		root := testkit.MustHead(t, counting, snapshot.DefaultRef)
		operations := make([]knowledge.Operation, 0, objects)
		for i := 0; i < objects; i++ {
			operations = append(operations, testkit.PutEntity(
				fmt.Sprintf("object/%06d", i), map[string]any{"value": i}, "")[0])
		}
		seed, err := w.Commit("seed", knowledge.ChangeSet{
			TargetRepository: counting.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: root, ExpectedTargetCommit: root, Operations: operations,
		})
		if err != nil {
			t.Fatal(err)
		}
		counting.reads, counting.readBytes, counting.lists = 0, 0, 0
		_, err = w.Commit("update", knowledge.ChangeSet{
			TargetRepository: counting.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: seed.Result.CommitID, ExpectedTargetCommit: seed.Result.CommitID,
			Operations: testkit.PutEntity("object/000000", map[string]any{"value": "updated"}, ""),
		})
		if err != nil {
			t.Fatal(err)
		}
		return counting.reads, counting.readBytes, counting.lists
	}

	smallReads, smallBytes, smallLists := measure(10)
	largeReads, largeBytes, largeLists := measure(1000)
	if smallLists != 0 || largeLists != 0 {
		t.Fatalf("single-object PUT scanned trees: small=%d large=%d", smallLists, largeLists)
	}
	if smallReads != largeReads || smallBytes != largeBytes {
		t.Fatalf("single-object PUT grew with repository: reads %d→%d, bytes %d→%d",
			smallReads, largeReads, smallBytes, largeBytes)
	}
}
