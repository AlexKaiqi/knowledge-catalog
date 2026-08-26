package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
	"kc/knowledge/reader"
)

func TestRelationsFindsSameCanonicalFromBothEndpoints(t *testing.T) {
	s := testkit.NewSetup(t, "")
	base := testkit.MustHead(t, s.Repo, "")
	relation := map[string]any{
		"relationId": "rel-contains-1", "relationType": "contains", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "container", "objectRef": "DatabaseSchema:tpch"},
			map[string]any{"role": "member", "objectRef": "Table:orders"},
		},
	}
	receipt, err := s.Writer.Commit("relations", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []knowledge.Operation{{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindRelation, ObjectID: "rel-contains-1"}, Value: relation}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []knowledge.ObjectID{"DatabaseSchema:tpch", "Table:orders"} {
		hits, err := s.Reader.Relations(s.RepositoryID, receipt.Result.CommitID, reader.RelationQuery{Endpoint: endpoint})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].ObjectID != "rel-contains-1" || hits[0].Commit != receipt.Result.CommitID {
			t.Fatalf("endpoint %s: %#v", endpoint, hits)
		}
	}
	filtered, err := s.Reader.Relations(s.RepositoryID, receipt.Result.CommitID, reader.RelationQuery{Endpoint: "Table:orders", RelationType: "contains", Role: "member"})
	if err != nil || len(filtered) != 1 || len(filtered[0].MatchedRoles) != 1 || filtered[0].MatchedRoles[0] != "member" {
		t.Fatalf("filtered: %#v %v", filtered, err)
	}
}
