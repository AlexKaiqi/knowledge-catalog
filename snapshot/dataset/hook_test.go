package dataset_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	catalog "kc/snapshot/dataset"
)

type spyHook struct {
	snapshots []catalog.Snapshot
}

func (s *spyHook) AfterSnapshot(ev catalog.Snapshot) error {
	s.snapshots = append(s.snapshots, ev)
	return nil
}

func TestWriterCommitFiresRepositoryHook(t *testing.T) {
	s := setupFed(t)
	spy := &spyHook{}
	s.catalog.AddHook(spy)
	w, err := writer.NewWriter(s.store, nil)
	if err != nil {
		t.Fatal(err)
	}
	head := testkit.MustHead(t, s.publicRepo, "")
	if _, err := w.Commit("w1", objectmodel.CommitChangeSet{
		TargetRepository: s.publicRepo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-hook", map[string]any{"body": "from writer"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	if len(spy.snapshots) != 1 || spy.snapshots[0].To == "" {
		t.Fatalf("COMMIT must reach Service.Hook without a facade: %#v", spy.snapshots)
	}
	if string(spy.snapshots[0].To) == "" {
		t.Fatalf("COMMIT must reach Service.Hook without a facade: %#v", spy.snapshots)
	}
}

func TestCatalogNotifySnapshotSkipsUnregistered(t *testing.T) {
	s := setupFed(t)
	spy := &spyHook{}
	s.catalog.AddHook(spy)
	other := testkit.MakeRepository(t, "kr://acme/public/other")
	s.catalog.NotifySnapshot(catalog.Snapshot{
		Repository: other,
		From:       "",
		To:         testkit.MustHead(t, other, ""),
	})
	if len(spy.snapshots) != 0 {
		t.Fatalf("unregistered repository must not fire: %d", len(spy.snapshots))
	}
	s.catalog.NotifySnapshot(catalog.Snapshot{
		Repository: s.publicRepo,
		From:       "",
		To:         testkit.MustHead(t, s.publicRepo, ""),
	})
	if len(spy.snapshots) != 1 {
		t.Fatalf("registered snapshot: %d", len(spy.snapshots))
	}
}

func TestCatalogHookFailureDoesNotFailDefineDataset(t *testing.T) {
	s := setupFed(t)
	s.catalog.AddHook(failHook{})
	if _, err := s.catalog.DefineDataset("agent", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
}

type failHook struct{}

func (failHook) AfterSnapshot(catalog.Snapshot) error {
	return kernel.Fail(kernel.ErrPreconditionFailed, "hook")
}
