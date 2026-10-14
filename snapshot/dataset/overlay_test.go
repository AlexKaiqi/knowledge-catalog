package dataset_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	catalog "kc/snapshot/dataset"
)

func TestMergeOverlayAddsReplacesAndRemoves(t *testing.T) {
	base := catalog.Dataset{
		SetID:    "notes",
		Revision: 1,
		Sources: []catalog.DatasetSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over, err := catalog.ParseDatasetOverlay([]byte(`
name: notes
remove:
  - kr://acme/public/semantic
mounts:
  - repository: kr://acme/personals/scratch
    selector: refs/heads/main
    path: scratch
`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.MergeOverlay(base, over)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("%#v", got.Sources)
	}
	if got.Sources[0].Repository != "kr://acme/personals/alice" {
		t.Fatal(got.Sources[0])
	}
	if got.Sources[1].Repository != "kr://acme/personals/scratch" || got.Sources[1].Path == nil || *got.Sources[1].Path != "scratch" {
		t.Fatal(got.Sources[1])
	}
}

func TestMergeOverlayReplacesSelectorAndBaseRev(t *testing.T) {
	base := catalog.Dataset{
		SetID: "notes",
		Sources: []catalog.DatasetSource{
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over := catalog.DatasetOverlay{Mounts: []catalog.DatasetMount{{
		Repository: "kr://acme/public/semantic",
		Selector:   "refs/heads/main",
		Path:       "refs/semantic",
		BaseRev:    "abc",
	}}}
	got, err := catalog.MergeOverlay(base, over)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources[0].Selector != "refs/heads/main" || got.Sources[0].BaseRev != "abc" {
		t.Fatal(got.Sources[0])
	}
}

func TestMergeOverlayRejectsUnknownRemoveAndNameMismatch(t *testing.T) {
	base := catalog.Dataset{
		SetID: "notes",
		Sources: []catalog.DatasetSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		},
	}
	if _, err := catalog.MergeOverlay(base, catalog.DatasetOverlay{Remove: []string{"kr://missing"}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
	if _, err := catalog.MergeOverlay(base, catalog.DatasetOverlay{Name: "other", Mounts: []catalog.DatasetMount{{
		Repository: "kr://acme/personals/scratch", Selector: "refs/heads/main", Path: "scratch",
	}}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
}

func TestResolveHonorsBaseRevCAS(t *testing.T) {
	s := setupFed(t)
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.catalog.DefineDataset("locked", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", BaseRev: string(head)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.ResolveDataset("locked"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.publicRepo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository:     "kr://acme/public/core",
		TargetRef:            "refs/heads/main",
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Operations:           testkit.PutEntity("policy/P-103", map[string]any{"statement": "moved"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.catalog.ResolveDataset("locked")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Repositories["kr://acme/public/core"] != head {
		t.Fatal("published dataset must keep the frozen commit after the branch moves", resolved.Repositories, head)
	}
	_, err = s.catalog.DefineDataset("stale", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", BaseRev: string(head)},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}
