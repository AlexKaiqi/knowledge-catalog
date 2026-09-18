package knowledgeapp

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

type searchRepository struct{ knowledge.Repository }

func (*searchRepository) ID() kernel.RepositoryID { return "kr://app/search" }

type searchLookup struct{ repo knowledge.Repository }

func (l searchLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (knowledge.Repository, error) {
	if id != l.repo.ID() {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing repository")
	}
	return l.repo, nil
}

type searchProjection struct {
	stateRequired bool
	stateReady    bool
	snapshotCalls int
	stateCalls    int
}

func (p *searchProjection) RequiresState(knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (bool, error) {
	return p.stateRequired, nil
}
func (p *searchProjection) StateView(kernel.RepositoryID, kernel.CommitID) (string, bool) {
	return "revision", p.stateReady
}
func (p *searchProjection) SearchAtContext(context.Context, knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (retrieval.SearchResult, error) {
	p.snapshotCalls++
	return retrieval.SearchResult{}, nil
}
func (p *searchProjection) SearchStateAtRevisionContext(context.Context, knowledge.Repository, kernel.CommitID, string, retrieval.SearchRequest) (retrieval.SearchResult, error) {
	p.stateCalls++
	return retrieval.SearchResult{}, nil
}

func TestSearchExecutorSelectsOneTypedProjectionPath(t *testing.T) {
	repo := &searchRepository{}
	projection := &searchProjection{}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection}
	if _, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if projection.snapshotCalls != 1 || projection.stateCalls != 0 {
		t.Fatalf("snapshot search calls = %d/%d", projection.snapshotCalls, projection.stateCalls)
	}

	projection.stateRequired, projection.stateReady = true, false
	_, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || projection.stateCalls != 0 {
		t.Fatalf("unprepared state search = %v, calls=%d", err, projection.stateCalls)
	}
	projection.stateReady = true
	if _, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if projection.stateCalls != 1 {
		t.Fatalf("state search calls = %d", projection.stateCalls)
	}
}
