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

// SemanticWindowProjection is the optional vector-window capability of a
// search projection. A projection that does not implement it has no derived
// vector surface, and semantic recall must fail closed instead of falling
// back to lexical (docs/RETRIEVAL.md §8.1).
type SemanticWindowProjection interface {
	SemanticWindowAtContext(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID, request retrieval.SearchRequest, queryVector []float32) (retrieval.SearchResult, error)
}

// embedSemanticQuery forms the request-time query vector. The query text is
// the validated MATCH text of the request; the projection and provider bind
// the vector to one model identity, which the result claims must disclose.
func embedSemanticQuery(ctx context.Context, embedder retrieval.Embedder, request retrieval.SearchRequest) ([]float32, retrieval.EmbeddingResult, error) {
	if embedder == nil {
		return nil, retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"semantic recall requires a configured embedding provider")
	}
	text := retrieval.SemanticQueryText(request)
	if text == "" {
		return nil, retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrUsageInvalid,
			"semantic recall requires a MATCH text query")
	}
	result, err := embedder.Embed(ctx, retrieval.EmbeddingRequest{Texts: []string{text}})
	if err != nil {
		return nil, retrieval.EmbeddingResult{}, err
	}
	if len(result.Vectors) != 1 || len(result.Vectors[0]) == 0 {
		return nil, retrieval.EmbeddingResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"embedding provider returned no usable query vector")
	}
	return result.Vectors[0], result, nil
}

// semanticWindowClaims names the model identity inside the approximate
// envelope. The index interval claim is added by the vector projection when
// it knows its generation freshness.
func semanticWindowClaims(embedding retrieval.EmbeddingResult) []string {
	return []string{"recall=semantic: query vector by " + embedding.Provider + "/" + embedding.Model}
}

type SearchExecutor struct {
	Repositories RepositoryLookup
	Projection   SearchProjection
	Embedder     retrieval.Embedder
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
	if request.Query.Recall == retrieval.RecallSemantic {
		return e.executeSemantic(ctx, repo, request)
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

// executeSemantic runs the vector window on the fixed repo version. The state
// view is a lexical-only surface today; it has no vector derivation, so the
// gate fails closed instead of approximating over an unprepared index.
func (e SearchExecutor) executeSemantic(ctx context.Context, repo knowledge.Repository, request SearchRequest) (retrieval.SearchResult, error) {
	vector, embedding, err := embedSemanticQuery(ctx, e.Embedder, request.Query)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	requiresState, err := e.Projection.RequiresState(repo, request.Commit, request.Query)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if requiresState {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"semantic recall is not served by the state projection")
	}
	window, ok := e.Projection.(SemanticWindowProjection)
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"semantic recall requires a ready vector projection")
	}
	result, err := window.SemanticWindowAtContext(ctx, repo, request.Commit, request.Query, vector)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := retrieval.MarkApproximate(&result, semanticWindowClaims(embedding)...); err != nil {
		return retrieval.SearchResult{}, err
	}
	return result, nil
}
