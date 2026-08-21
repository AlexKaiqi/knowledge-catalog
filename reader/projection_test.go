package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/local"
	"kc/repository"
)

func buildRepo(t *testing.T) (*local.FileGitRepository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	head := testkit.MustHead(t, repo, "refs/heads/main")
	var err error
	head, err = repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"title": "refund policy", "body": "production requires a tested runbook"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err = repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("procedure/refund-timeout", map[string]any{"title": "refund timeout", "body": "diagnose runbook failures"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, head
}

func TestT8LocateAndHydrate(t *testing.T) {
	repo, head := buildRepo(t)
	proj := reader.NewProjection()
	if err := proj.Build(repo, head, nil); err != nil {
		t.Fatal(err)
	}
	hits, err := proj.Search(repo, "runbook")
	if err != nil || len(hits) != 2 {
		t.Fatalf("%d %v", len(hits), err)
	}
	for _, h := range hits {
		if h.Repository != repo.ID() {
			t.Fatal(h.Repository)
		}
	}
}

func TestT8BasisAndLag(t *testing.T) {
	repo, head := buildRepo(t)
	proj := reader.NewProjection()
	if err := proj.Build(repo, head, nil); err != nil {
		t.Fatal(err)
	}
	desc, err := proj.DescribeIndex(repo)
	if err != nil || desc.BasisCommit != head || desc.ObjectCount != 2 || desc.LagBehindHead {
		t.Fatalf("%#v %v", desc, err)
	}
	newHead, err := repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-200", map[string]any{"title": "new"}, ""),
	})
	if err != nil || newHead == head {
		t.Fatal(err)
	}
	desc, err = proj.DescribeIndex(repo)
	if err != nil || !desc.LagBehindHead {
		t.Fatalf("%#v %v", desc, err)
	}
}

func TestT8Rebuildable(t *testing.T) {
	repo, head := buildRepo(t)
	p1 := reader.NewProjection()
	if err := p1.Build(repo, head, nil); err != nil {
		t.Fatal(err)
	}
	p2 := reader.NewProjection()
	if err := p2.Build(repo, head, nil); err != nil {
		t.Fatal(err)
	}
	a, _ := p1.Search(repo, "runbook")
	b, _ := p2.Search(repo, "runbook")
	if len(a) != len(b) {
		t.Fatalf("%d %d", len(a), len(b))
	}
	listed, err := repo.List(head)
	if err != nil || len(listed) != 2 {
		t.Fatal(listed, err)
	}
}

func TestT8AspectSelectorExcludesACL(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	head := testkit.MustHead(t, repo, "refs/heads/main")
	head, err := repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []repository.Operation{
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "Table:tl.db.t", AspectName: "structure"}, Value: map[string]any{"storage_type": "hive", "raw_description": "user events"}},
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindMember, ObjectID: "Table:tl.db.t", AspectName: "permissions", MemberKey: "user:a"}, Value: map[string]any{"privileges": []any{"SELECT"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proj := reader.NewProjection()
	if err := proj.Build(repo, head, &repository.AspectSelector{Exclude: []string{"permissions"}}); err != nil {
		t.Fatal(err)
	}
	hive, err := proj.Search(repo, "hive")
	if err != nil || len(hive) != 1 {
		t.Fatal(hive, err)
	}
	selectHits, err := proj.Search(repo, "SELECT")
	if err != nil || len(selectHits) != 0 {
		t.Fatal(selectHits, err)
	}
}

func TestT8ReaderAspectSelect(t *testing.T) {
	s := testkit.NewSetup(t, "")
	root := testkit.MustHead(t, s.Repo, "")
	commit, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []repository.Operation{
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "t", AspectName: "structure"}, Value: map[string]any{"pk": []any{"id"}}},
			{Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindAspect, ObjectID: "t", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := s.Reader.ReadAddress(s.RepositoryID, kernel.Address{Kind: kernel.KindAspect, ObjectID: "t", AspectName: "structure"}, commit)
	if err != nil || unit.Value.(map[string]any)["pk"].([]any)[0] != "id" {
		t.Fatalf("%#v %v", unit, err)
	}
	assembled, err := s.Reader.Read(kernel.KnowledgeRef{Repository: s.RepositoryID, Object: "t"}, commit, &repository.AspectSelector{Exclude: []string{"ownership"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := assembled.Value.(map[string]any)["ownership"]; ok {
		t.Fatalf("%#v", assembled.Value)
	}
}
