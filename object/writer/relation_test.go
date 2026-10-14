package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
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
	_, err := s.Writer.Commit("relation-valid", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "rel-1"}, Value: valid}},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid := map[string]any{"relationId": "wrong", "relationType": "contains", "direction": "DIRECTED", "endpoints": valid["endpoints"]}
	_, err = s.Writer.Commit("relation-invalid", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main",
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "rel-2"}, Value: invalid}},
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
	foreign := testkit.MakeRepository(t, "kr://acme/other")
	foreignHead := testkit.MustHead(t, foreign, "")
	receipt, err := s.Writer.Commit("relation-cross-repository", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: head, ExpectedTargetCommit: head,
		Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "rel-cross"}, Value: crossRepository}},
	})
	if err != nil {
		t.Fatalf("cross-repository reference must not require endpoint authority: %v", err)
	}
	value, err := s.Repo.Read("rel-cross", receipt.Result.CommitID)
	if err != nil || kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(crossRepository) {
		t.Fatalf("relation did not retain endpoint identity: %#v %v", value, err)
	}
	if got := testkit.MustHead(t, foreign, ""); got != foreignHead {
		t.Fatalf("writing a reference changed endpoint authority: %s != %s", got, foreignHead)
	}
}

func TestCrossRepositoryRelationStillRejectsMalformedEnvelope(t *testing.T) {
	s := testkit.NewSetup(t, "")
	endpoint := map[string]any{"role": "member", "objectRef": map[string]any{"repository": "kr://external/unattached", "object": "object/a"}}
	for name, endpoints := range map[string][]any{
		"one endpoint": {endpoint},
		"duplicate":    {endpoint, endpoint},
		"unqualified":  {endpoint, map[string]any{"role": "owner", "objectRef": map[string]any{"object": "object/b"}}},
	} {
		t.Run(name, func(t *testing.T) {
			head := testkit.MustHead(t, s.Repo, "")
			_, err := s.Writer.Commit(name, objectmodel.CommitChangeSet{TargetRepository: s.RepositoryID, TargetRef: "refs/heads/main", BaseCommit: head, ExpectedTargetCommit: head, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/invalid"}, Value: map[string]any{"relationId": "relation/invalid", "relationType": "contains", "direction": "DIRECTED", "endpoints": endpoints}}}})
			testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
			if testkit.MustHead(t, s.Repo, "") != head {
				t.Fatal("invalid relation changed Snapshot HEAD")
			}
		})
	}
}
