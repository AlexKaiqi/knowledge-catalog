package index

import (
	"context"
	"kc/kernel"
	"kc/knowledge"
)

type candidateScopeKey struct{}
type candidateScope struct {
	digest   kernel.Digest
	contains func(kernel.RepositoryID, kernel.CommitID, knowledge.ObjectID) (bool, error)
}

// WithCandidateScope constrains canonical hydration before either the cache,
// authority, or dynamic serving store is read. It never changes a projection.
func WithCandidateScope(ctx context.Context, digest kernel.Digest, contains func(kernel.RepositoryID, kernel.CommitID, knowledge.ObjectID) (bool, error)) context.Context {
	return context.WithValue(ctx, candidateScopeKey{}, candidateScope{digest: digest, contains: contains})
}

func filterCandidateScope(ctx context.Context, repo kernel.RepositoryID, commit kernel.CommitID, page CandidatePage) (CandidatePage, error) {
	scope, ok := ctx.Value(candidateScopeKey{}).(candidateScope)
	if !ok {
		return page, nil
	}
	if scope.contains == nil {
		return CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "candidate scope is incomplete")
	}
	filtered := make([]CandidateRef, 0, len(page.Candidates))
	for _, candidate := range page.Candidates {
		if (candidate.Repository != "" && candidate.Repository != repo) || candidate.Basis != commit {
			return CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "candidate does not match fixed repository basis")
		}
		allowed, err := scope.contains(repo, commit, candidate.ObjectID)
		if err != nil {
			return CandidatePage{}, err
		}
		if allowed {
			filtered = append(filtered, candidate)
		}
	}
	page.Candidates = filtered
	return page, nil
}
