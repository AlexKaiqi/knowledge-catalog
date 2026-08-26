package opensearch_test

import (
	"kc/retrieval"
	"testing"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
)

type knowledgeWriter interface {
	knowledge.Repository
	ApplyKnowledgeCommit(knowledge.ChangeSet) (kernel.CommitID, error)
}

func putAt(t *testing.T, repo knowledgeWriter, base kernel.CommitID, ops []knowledge.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: "refs/heads/main",
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func policyBodySchema() knowledge.Operation {
	return knowledge.Operation{
		Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy.body"},
		Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
		},
	}
}

func objectIDs(hits retrieval.SearchResult) []string {
	out := make([]string, len(hits.Hits))
	for i, hit := range hits.Hits {
		out[i] = string(hit.Knowledge.Address.ObjectID)
	}
	return out
}

func mustSearchErr(t *testing.T, idx *index.Index, repo knowledge.Repository, req retrieval.SearchRequest) error {
	t.Helper()
	_, err := idx.Search(repo, req)
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}
