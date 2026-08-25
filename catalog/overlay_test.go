package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
)

func TestMergeOverlayAddsReplacesAndRemoves(t *testing.T) {
	base := catalog.WorkspaceDefinition{
		WorkspaceID: "notes",
		Revision:    1,
		Sources: []catalog.WorkspaceSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over, err := catalog.ParseWorkspaceOverlay([]byte(`
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
	base := catalog.WorkspaceDefinition{
		WorkspaceID: "notes",
		Sources: []catalog.WorkspaceSource{
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over := catalog.WorkspaceOverlay{Mounts: []catalog.WorkspaceMount{{
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
	base := catalog.WorkspaceDefinition{
		WorkspaceID: "notes",
		Sources: []catalog.WorkspaceSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		},
	}
	if _, err := catalog.MergeOverlay(base, catalog.WorkspaceOverlay{Remove: []string{"kr://missing"}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
	if _, err := catalog.MergeOverlay(base, catalog.WorkspaceOverlay{Name: "other", Mounts: []catalog.WorkspaceMount{{
		Repository: "kr://acme/personals/scratch", Selector: "refs/heads/main", Path: "scratch",
	}}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
}

func TestResolveHonorsBaseRevCAS(t *testing.T) {
	s := setupFed(t)
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.catalog.DefineWorkspace("locked", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", BaseRev: string(head)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.ResolveWorkspace("locked"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.publicRepo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository:     "kr://acme/public/core",
		TargetRef:            "refs/heads/main",
		BaseCommit:           head,
		ExpectedTargetCommit: head,
		Operations:           testkit.PutEntity("policy/P-103", map[string]any{"statement": "moved"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := s.catalog.ResolveWorkspace("locked")
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}
