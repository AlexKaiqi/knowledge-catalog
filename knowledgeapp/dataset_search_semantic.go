package knowledgeapp

import (
	"context"
	"fmt"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// executeSemantic serves one semantic window per fixed member (RETRIEVAL.md
// §8.1): the query vector is fetched once, every member contributes its
// single bounded top-K window at its pinned commit, and the windows merge by
// rank interleave in member order — k-NN scores are only locally comparable,
// so no member may claim global ordering. A member without a vector surface
// (including State-projection members) fails the whole request closed; no
// member may silently drop to lexical recall. The merged result is always
// approximate and never paged.
func (e DatasetSearchExecutor) executeSemantic(ctx context.Context, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	if err := e.Authorize(ctx); err != nil {
		return retrieval.SearchResult{}, err
	}
	vector, embedding, err := embedSemanticQuery(ctx, e.Embedder, req)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	memberWindows, ok := e.Projection.(SemanticWindowProjection)
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"semantic recall on a dataset requires member vector windows")
	}
	serving, _, err := e.Resolve(ctx)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if serving == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset serving is unavailable")
	}
	budgetCtx, cancel := index.WithSearchBudget(ctx, index.SearchBudget{})
	defer cancel()
	pin := serving.Pin()
	budgetCtx = index.WithCandidateScope(budgetCtx, kernel.CanonicalDigest(pin.Items), func(id kernel.RepositoryID, at kernel.CommitID, object knowledge.ObjectID) (bool, error) {
		if pin.Repositories[id] != at {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "candidate is outside dataset basis")
		}
		return serving.Contains(id, object)
	})
	plan, err := retrieval.PlanAccess(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return e.Repositories.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}, pin)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = retrieval.DefaultSearchLimit
	}
	memberReq := req
	memberReq.Limit = limit
	memberReq.Continuation = ""
	windows := make([][]retrieval.KnowledgeHit, 0, len(plan.Specs))
	for _, spec := range plan.Specs {
		repo, err := e.Repositories.Require(spec.Repository, kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		required, err := e.Projection.RequiresState(repo, spec.Commit, req)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		if required {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"member %s is served from a State projection without a vector window", spec.Repository)
		}
		memberResult, err := memberWindows.SemanticWindowAtContext(budgetCtx, repo, spec.Commit, memberReq, vector)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		windows = append(windows, memberResult.Hits)
	}
	merged := mergeMemberWindows(windows, limit)
	for i := range merged {
		hit, err := e.Deliver(ctx, merged[i])
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		merged[i] = hit
	}
	result := retrieval.SearchResult{
		SearchView: retrieval.SearchView{Snapshots: cloneCommitMap(pin.Repositories)},
		Hits:       merged,
	}
	claims := append(semanticWindowClaims(embedding),
		fmt.Sprintf("recall=semantic: %d member windows merged by rank interleave", len(plan.Specs)))
	if err := retrieval.MarkApproximate(&result, claims...); err != nil {
		return retrieval.SearchResult{}, err
	}
	return result, nil
}

func cloneCommitMap(source map[kernel.RepositoryID]kernel.CommitID) map[kernel.RepositoryID]kernel.CommitID {
	out := make(map[kernel.RepositoryID]kernel.CommitID, len(source))
	for id, commit := range source {
		out[id] = commit
	}
	return out
}

// mergeMemberWindows interleaves member windows by index: round one takes
// each member's first hit in member order, round two the second, and so on,
// until the page limit is reached. Ties stay in member order.
func mergeMemberWindows(windows [][]retrieval.KnowledgeHit, limit int) []retrieval.KnowledgeHit {
	merged := make([]retrieval.KnowledgeHit, 0, limit)
	seen := map[string]bool{}
	for round := 0; len(merged) < limit; round++ {
		advanced := false
		for _, window := range windows {
			if round >= len(window) {
				continue
			}
			advanced = true
			hit := window[round]
			key := string(hit.Knowledge.Repository) + "\x00" + string(hit.Knowledge.Address.ObjectID)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, hit)
			if len(merged) >= limit {
				break
			}
		}
		if !advanced {
			break
		}
	}
	return merged
}
