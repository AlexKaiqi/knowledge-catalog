package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

func TestSearchDebugContains(t *testing.T) {
	s := testkit.NewSetup(t, "")
	head := s.RootCommitID
	commit, err := s.Repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"body": "production requires a runbook"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.Reader.Search("runbook", s.RepositoryID, commit)
	if err != nil || len(hits) != 1 || hits[0].Address.ObjectID != "policy/P-103" {
		t.Fatalf("%#v %v", hits, err)
	}
	none, err := s.Reader.Search("no-such-token", s.RepositoryID, commit)
	if err != nil || len(none) != 0 {
		t.Fatalf("%#v %v", none, err)
	}
	_, err = s.Reader.Search("runbook", "kr://missing", commit)
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
}
