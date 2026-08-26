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
			map[string]any{"role": "container", "objectRef": "DatabaseSchema:tpch"},
			map[string]any{"role": "member", "objectRef": "Table:orders"},
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
}
