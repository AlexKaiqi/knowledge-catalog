package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
)

func TestMergeOverlayAddsReplacesAndRemoves(t *testing.T) {
	base := catalog.KnowledgeSet{
		SetID:    "notes",
		Revision: 1,
		Sources: []catalog.KnowledgeSetSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over, err := catalog.ParseKnowledgeSetOverlay([]byte(`
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
	base := catalog.KnowledgeSet{
		SetID: "notes",
		Sources: []catalog.KnowledgeSetSource{
			{Repository: "kr://acme/public/semantic", Selector: "refs/heads/stable", Path: catalog.MountPath("refs/semantic")},
		},
	}
	over := catalog.KnowledgeSetOverlay{Mounts: []catalog.KnowledgeSetMount{{
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
	base := catalog.KnowledgeSet{
		SetID: "notes",
		Sources: []catalog.KnowledgeSetSource{
			{Repository: "kr://acme/personals/alice", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		},
	}
	if _, err := catalog.MergeOverlay(base, catalog.KnowledgeSetOverlay{Remove: []string{"kr://missing"}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
	if _, err := catalog.MergeOverlay(base, catalog.KnowledgeSetOverlay{Name: "other", Mounts: []catalog.KnowledgeSetMount{{
		Repository: "kr://acme/personals/scratch", Selector: "refs/heads/main", Path: "scratch",
	}}}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatal(err)
	}
}

func TestResolveHonorsBaseRevCAS(t *testing.T) {
	s := setupFed(t)
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.catalog.DefineKnowledgeSet("locked", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", BaseRev: string(head)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.ResolveKnowledgeSet("locked"); err != nil {
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
	resolved, err := s.catalog.ResolveKnowledgeSet("locked")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Repositories["kr://acme/public/core"] != head {
		t.Fatal("published dataset must keep the frozen commit after the branch moves", resolved.Repositories, head)
	}
	_, err = s.catalog.DefineKnowledgeSet("stale", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", BaseRev: string(head)},
	})
	testkit.ExpectCode(t, err, kernel.ErrNonFastForward)
}
