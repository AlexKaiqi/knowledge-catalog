package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
)

func TestRelationEnvelopeValidatedBeforeCommit(t *testing.T) {
	s := testkit.NewSetup(t, "")
	base := testkit.MustHead(t, s.Repo, "")
	valid := map[string]any{
		"relationId": "rel-1", "relationType": "contains", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "container", "objectRef": map[string]any{"repository": string(s.RepositoryID), "object": "DatabaseSchema:tpch"}},
			map[string]any{"role": "member", "objectRef": map[string]any{"repository": string(s.RepositoryID), "object": "Table:orders"}},
		},
	}
	_, err := s.Writer.Commit("relation-valid", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "rel-1"}, Value: valid}},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid := map[string]any{"relationId": "wrong", "relationType": "contains", "direction": "DIRECTED", "endpoints": valid["endpoints"]}
	_, err = s.Writer.Commit("relation-invalid", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "rel-2"}, Value: invalid}},
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)

	crossRepository := map[string]any{
		"relationId": "rel-cross", "relationType": "contains", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "container", "objectRef": map[string]any{"repository": string(s.RepositoryID), "object": "Table:a"}},
			map[string]any{"role": "member", "objectRef": map[string]any{"repository": "kr://acme/other", "object": "Table:b"}},
		},
	}
	head := testkit.MustHead(t, s.Repo, "")
	_, err = s.Writer.Commit("relation-cross-repository", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "rel-cross"}, Value: crossRepository}},
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}
