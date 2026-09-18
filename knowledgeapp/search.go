package knowledgeapp

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

type RepositoryLookup interface {
	Require(kernel.RepositoryID, kernel.ErrorCode) (knowledge.Repository, error)
}

type SearchProjection interface {
	RequiresState(knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (bool, error)
	StateView(kernel.RepositoryID, kernel.CommitID) (string, bool)
	SearchAtContext(context.Context, knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (retrieval.SearchResult, error)
	SearchStateAtRevisionContext(context.Context, knowledge.Repository, kernel.CommitID, string, retrieval.SearchRequest) (retrieval.SearchResult, error)
}

type SearchRequest struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	Query      retrieval.SearchRequest
}

type SearchExecutor struct {
	Repositories RepositoryLookup
	Projection   SearchProjection
}

func (e SearchExecutor) Execute(ctx context.Context, request SearchRequest) (retrieval.SearchResult, error) {
	if request.Repository == "" || request.Commit == "" {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid,
			"knowledge search requires a repository and fixed commit")
	}
	if e.Repositories == nil || e.Projection == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"knowledge search service is unavailable")
	}
	repo, err := e.Repositories.Require(request.Repository, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	requiresState, err := e.Projection.RequiresState(repo, request.Commit, request.Query)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if requiresState {
		if _, ok := e.Projection.StateView(repo.ID(), request.Commit); !ok {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"State projection is not prepared")
		}
		return e.Projection.SearchStateAtRevisionContext(ctx, repo, request.Commit, "", request.Query)
	}
	return e.Projection.SearchAtContext(ctx, repo, request.Commit, request.Query)
}
