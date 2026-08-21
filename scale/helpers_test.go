package scale_test

import (
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func putAt(t *testing.T, repo repository.Repository, base kernel.CommitID, ops []repository.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyCommit(repository.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func policyBodySchema() repository.Operation {
	return repository.Operation{
		Op: repository.OpPut, Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: "schema/policy.body"},
		Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
		},
	}
}

func objectIDs(hits []repository.KnowledgeValue) []string {
	out := make([]string, len(hits))
	for i, hit := range hits {
		out[i] = string(hit.Address.ObjectID)
	}
	return out
}

func mustSearchErr(t *testing.T, idx *index.Index, repo repository.Repository, req reader.SearchRequest) error {
	t.Helper()
	_, err := idx.Search(repo, req)
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}
