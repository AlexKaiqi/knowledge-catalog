package testfixture

import (
	"fmt"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type objectWriter interface {
	objectmodel.Repository
	ApplyObjectCommit(objectmodel.ChangeSet) (kernel.CommitID, error)
}

func PutAt(t *testing.T, repo objectWriter, base kernel.CommitID, ops []objectmodel.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base, Operations: ops,
	})
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func PolicyBodySchema() objectmodel.Operation {
	return objectmodel.Operation{
		Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy.body"},
		Value: map[string]any{
			"entity": "Policy", "pattern": "record",
			"fields": map[string]any{"body": map[string]any{"type": "string"}},
		},
	}
}

// PolicyBodyIndex declares full-text retrieval over PolicyBodySchema's body;
// the retrieval surface comes only from this declaration.
func PolicyBodyIndex() objectmodel.Operation {
	return testkit.IndexText("schema/policy.body", "body")
}

var indexTestSequence atomic.Uint64

func MakeIndexRepository(t *testing.T, base string) *testkit.ObjectRepository {
	t.Helper()
	return testkit.MakeRepository(t, uniqueRepositoryID(base))
}

func uniqueRepositoryID(base string) string {
	return fmt.Sprintf("%s/index-test-%d-%d", strings.TrimRight(base, "/"), time.Now().UnixNano(), indexTestSequence.Add(1))
}

func RelationFixture(t *testing.T) (*testkit.ObjectRepository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/owned"}, Value: RelationBody("relation/owned", "table/a")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation/false-positive"}, Value: RelationBody("relation/false-positive", "table/other")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, commit
}

func RelationBody(id, endpoint string) map[string]any {
	return map[string]any{
		"relationId": id, "relationType": "owned-by", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "subject", "object": endpoint},
			map[string]any{"role": "owner", "object": "team/finance"},
		},
	}
}

func CommittedSearchablePolicy(t *testing.T) (*testkit.ObjectRepository, kernel.CommitID, kernel.CommitID) {
	t.Helper()
	repo := MakeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := PutAt(t, repo, root, []objectmodel.Operation{
		PolicyBodySchema(),
		PolicyBodyIndex(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	return repo, root, head
}
