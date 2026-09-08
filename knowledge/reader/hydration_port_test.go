package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

type portHydrator struct{ reads, addresses int }

func (h *portHydrator) ReadMany(repo knowledge.Repository, commit kernel.CommitID, ids []knowledge.ObjectID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	h.reads++
	return repo.(knowledge.BatchReadStore).ReadMany(ids, commit)
}

func (h *portHydrator) ReadAddress(repo knowledge.Repository, commit kernel.CommitID, address knowledge.Address) (knowledge.KnowledgeValue, error) {
	h.addresses++
	return repo.ReadAddress(address, commit)
}

func TestReaderAndWorkspaceUseInjectedHydrationAtFixedBasis(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/hydration-port")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/a", AspectName: "body"}
	commit, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: address, Value: map[string]any{"text": "original"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := snapshot.NewRegistry()
	if err := registry.Add(repo); err != nil {
		t.Fatal(err)
	}
	rd := reader.NewReader(registry)
	port := &portHydrator{}
	rd.SetHydrator(port)
	ref := knowledge.KnowledgeRef{Repository: repo.ID(), Object: address.ObjectID}
	value, err := rd.Read(ref, commit, nil)
	if err != nil || value.Commit != commit || port.reads != 1 {
		t.Fatalf("reader did not use fixed-basis port: %+v %v reads=%d", value, err, port.reads)
	}
	if _, err := rd.ReadAddress(repo.ID(), address, commit); err != nil || port.addresses != 1 {
		t.Fatalf("address port: %v calls=%d", err, port.addresses)
	}
	s := reader.Open(rd.Lookup(func(id kernel.RepositoryID) (snapshot.Store, error) {
		return registry.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}), reader.WorkspacePin{WorkspaceID: "fixed", Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}})
	s.SetHydrator(port)
	values, err := s.Read(address.ObjectID, nil)
	if err != nil || len(values) != 1 || values[0].Commit != commit || port.reads != 2 {
		t.Fatalf("workspace port: %+v %v reads=%d", values, err, port.reads)
	}
	if _, err := s.ReadAddress(address); err != nil || port.addresses != 2 {
		t.Fatalf("workspace address port: %v calls=%d", err, port.addresses)
	}
}
