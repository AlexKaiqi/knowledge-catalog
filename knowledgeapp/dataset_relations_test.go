package knowledgeapp

import (
	"context"
	"fmt"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/retrieval"
)

type relationTestRepository struct {
	knowledge.Repository
	id kernel.RepositoryID
}

func (r relationTestRepository) ID() kernel.RepositoryID { return r.id }

type relationTestLookup map[kernel.RepositoryID]knowledge.Repository

func (l relationTestLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (knowledge.Repository, error) {
	if repo, ok := l[id]; ok {
		return repo, nil
	}
	return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "repository not selected")
}

type relationTestProjection struct {
	t     *testing.T
	calls []kernel.RepositoryID
}

func (p *relationTestProjection) RelationsAtContext(_ context.Context, repo knowledge.Repository, at kernel.CommitID, req retrieval.RelationPageRequest) (retrieval.RelationPage, error) {
	p.calls = append(p.calls, repo.ID())
	if at != kernel.CommitID(string(repo.ID())+"-fixed") || req.Query.Endpoint.Repository != "kr://a/entities" {
		p.t.Fatalf("storage and endpoint bases were conflated: %s %s %#v", repo.ID(), at, req.Query)
	}
	page := retrieval.RelationPage{Exhausted: true}
	if repo.ID() != req.Query.Endpoint.Repository {
		page.Hits = []retrieval.RelationHit{{Repository: repo.ID(), Commit: at, ObjectID: "relation/one"}}
	}
	return page, nil
}

func TestDatasetRelationsQueryAllMembersAndBindContinuationToScope(t *testing.T) {
	repos := relationTestLookup{}
	pin := reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{}}
	for _, id := range []kernel.RepositoryID{"kr://a/entities", "kr://b/graph", "kr://z/graph"} {
		repos[id] = relationTestRepository{id: id}
		pin.Repositories[id] = kernel.CommitID(string(id) + "-fixed")
	}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	lookup := func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return repos.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}
	projection := &relationTestProjection{t: t}
	executor := DatasetRelationsExecutor{Serving: reader.Open(lookup, pin), Repositories: repos, Projection: projection}
	req := retrieval.RelationPageRequest{Query: retrieval.RelationQuery{Endpoint: knowledge.KnowledgeRef{Repository: "kr://a/entities", Object: "object/a"}}, Limit: 1}
	first, err := executor.Execute(context.Background(), req)
	if err != nil || len(first.Hits) != 1 || first.Hits[0].Repository != "kr://b/graph" || first.Continuation == "" || first.Exhausted {
		t.Fatalf("first cross-repository page: %#v %v", first, err)
	}
	req.Continuation = first.Continuation
	second, err := executor.Execute(context.Background(), req)
	if err != nil || len(second.Hits) != 1 || second.Hits[0].Repository != "kr://z/graph" || !second.Exhausted {
		t.Fatalf("second cross-repository page: %#v %v", second, err)
	}
	if got := fmt.Sprint(projection.calls); got != "[kr://a/entities kr://b/graph kr://z/graph]" {
		t.Fatalf("repeated or skipped member queries: %s", got)
	}
	pin.Items[0].Target = "changed-layout"
	executor.Serving = reader.Open(lookup, pin)
	if _, err := executor.Execute(context.Background(), req); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("continuation crossed Dataset scope: %v", err)
	}
	if len(projection.calls) != 3 {
		t.Fatal("invalid continuation reached projection")
	}
}
