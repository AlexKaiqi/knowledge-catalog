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

type knowledgeWriter interface {
	objectmodel.Repository
	ApplyKnowledgeCommit(objectmodel.ChangeSet) (kernel.CommitID, error)
}

func PutAt(t *testing.T, repo knowledgeWriter, base kernel.CommitID, ops []objectmodel.Operation) kernel.CommitID {
	t.Helper()
	head, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
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
			"fields": map[string]any{"body": map[string]any{"access": []any{"text"}}},
		},
	}
}

var indexTestSequence atomic.Uint64

func MakeIndexRepository(t *testing.T, base string) *testkit.KnowledgeRepository {
	t.Helper()
	return testkit.MakeRepository(t, uniqueRepositoryID(base))
}

func uniqueRepositoryID(base string) string {
	return fmt.Sprintf("%s/index-test-%d-%d", strings.TrimRight(base, "/"), time.Now().UnixNano(), indexTestSequence.Add(1))
}

func RelationFixture(t *testing.T) (*testkit.KnowledgeRepository, kernel.CommitID) {
	t.Helper()
	repo := testkit.MakeRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	commit, err := repo.ApplyKnowledgeCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation:owned"}, Value: RelationBody(repo.ID(), "relation:owned", "Table:a")},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: "relation:false-positive"}, Value: RelationBody(repo.ID(), "relation:false-positive", "Table:other")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, commit
}

func RelationBody(repository kernel.RepositoryID, id, endpoint string) map[string]any {
	return map[string]any{
		"relationId": id, "relationType": "owned-by", "direction": "DIRECTED",
		"endpoints": []any{
			map[string]any{"role": "subject", "objectRef": map[string]any{"repository": string(repository), "object": endpoint}},
			map[string]any{"role": "owner", "objectRef": map[string]any{"repository": string(repository), "object": "Team:finance"}},
		},
	}
}

func CommittedSearchablePolicy(t *testing.T) (*testkit.KnowledgeRepository, kernel.CommitID, kernel.CommitID) {
	t.Helper()
	repo := MakeIndexRepository(t, "kr://acme/public/core")
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	head := PutAt(t, repo, root, []objectmodel.Operation{
		PolicyBodySchema(),
		testkit.PutEntity("policy/P-1", map[string]any{"body": "needs a runbook"}, "")[0],
	})
	return repo, root, head
}
