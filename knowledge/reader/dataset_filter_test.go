package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/snapshot"
)

func TestEmptyDatasetItemsDoNotAdmitWholeRepository(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: testkit.PutEntity("policy/A", map[string]any{"body": "secret"}, "docs/a.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	serving := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return s.Repo, nil
	}, reader.KnowledgeSetPin{SetID: "empty", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}})
	values, err := serving.Read("policy/A", nil)
	if err != nil || len(values) != 0 {
		t.Fatalf("empty Items must not admit the whole repository: %#v %v", values, err)
	}
}

func TestPrefixDatasetItemsExcludeSiblingPaths(t *testing.T) {
	s := testkit.NewSetup(t, "")
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: append(
			testkit.PutEntity("listed", map[string]any{"body": "listed payment"}, "policies/listed.yaml"),
			testkit.PutEntity("secret", map[string]any{"body": "secret payment"}, "notes/secret.yaml")...,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.KnowledgeSetPin{
		SetID:        "docs",
		Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit},
		Items: []reader.DatasetItem{{
			Target: "policies", Repository: s.RepositoryID, Commit: commit, Kind: "prefix", Prefix: "policies",
		}},
	}
	serving := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return s.Repo, nil
	}, pin)
	listed, err := serving.Read("listed", nil)
	if err != nil || len(listed) != 1 {
		t.Fatalf("prefix dataset must read listed files: %#v %v", listed, err)
	}
	secret, err := serving.Read("secret", nil)
	if err != nil || len(secret) != 0 {
		t.Fatalf("prefix dataset must not read sibling paths: %#v %v", secret, err)
	}
}
