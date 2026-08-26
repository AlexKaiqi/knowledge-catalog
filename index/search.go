package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

func (idx *Index) Search(repo knowledge.Repository, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if meta.Basis == "" {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "projection for %s is empty; write or index-sync first", repo.ID())
	}
	return idx.searchEngine(repo, eng, meta.Basis, req)
}

// SearchAt evaluates SEARCH at a frozen commit without rewinding the live engine.
func (idx *Index) SearchAt(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	if commit == "" {
		return idx.Search(repo, req)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return retrieval.SearchResult{}, err
	}
	eng, err := idx.engineForCommit(repo.ID(), commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	return idx.searchEngine(repo, eng, commit, req)
}

func (idx *Index) searchEngine(repo knowledge.Repository, eng Engine, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	result := retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}},
		Completeness: retrieval.CompletenessComplete,
		Hits:         []retrieval.KnowledgeHit{},
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	var identity ProviderIdentity
	if provider, ok := eng.(ProviderIdentity); ok {
		identity = provider
	}
	plan, err := PlanRetrieval(eng, identity, req, spec)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	resolved := plan.Search
	needsResidual := false
	for _, fragment := range plan.Fragments {
		capability := fragment.Capability
		if capability.Guarantee == GuaranteeUnsupported {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
		if capability.Guarantee == GuaranteeSuperset {
			needsResidual = true
		}
		if capability.Guarantee == GuaranteeApproximate || capability.Coverage < 1 {
			result.Completeness = retrieval.CompletenessPartial
			result.Claims = append(result.Claims, "provider guarantee="+string(capability.Guarantee))
		}
	}
	viewDigest := retrieval.SearchViewDigest(result.SearchView)
	queryDigest := retrieval.SearchQueryDigest(resolved)
	projectionDigest := kernel.CanonicalDigest(plan.Projection)
	continuation := ""
	if resolved.Continuation != "" {
		state, err := retrieval.DecodeContinuation(resolved.Continuation)
		if err != nil || state.Scope != "repository" || state.Query != queryDigest || state.SearchView != viewDigest || state.Projection != projectionDigest {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
		}
		continuation = state.Position
	}
	resolved.Continuation = ""
	nextContinuation := ""
	for {
		pageReq := resolved
		if resolved.Limit > 0 {
			pageReq.Limit = resolved.Limit - len(result.Hits)
			if pageReq.Limit <= 0 {
				break
			}
		}
		page, err := eng.Retrieve(RetrieveRequest{Search: pageReq, Spec: spec, Continuation: continuation})
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		candidateIDs := make([]knowledge.ObjectID, 0, len(page.Candidates))
		for _, candidate := range page.Candidates {
			if (candidate.Repository == "" || candidate.Repository == repo.ID()) && candidate.Basis == commit {
				candidateIDs = append(candidateIDs, candidate.ObjectID)
			}
		}
		hydrated, err := hydrateMany(repo, commit, candidateIDs)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		for _, candidate := range page.Candidates {
			if candidate.Repository != "" && candidate.Repository != repo.ID() {
				result.Completeness = retrieval.CompletenessPartial
				result.Claims = append(result.Claims, "candidate repository mismatch")
				continue
			}
			candidate.Repository = repo.ID()
			if candidate.Basis != commit {
				result.Completeness = retrieval.CompletenessPartial
				result.Claims = append(result.Claims, "candidate basis mismatch")
				continue
			}
			value, ok := hydrated[candidate.ObjectID]
			if !ok {
				result.Completeness = retrieval.CompletenessPartial
				result.Claims = append(result.Claims, "candidate removed before hydrate: "+string(candidate.ObjectID))
				continue
			}
			if needsResidual {
				matched, err := matchesResidual(repo, value, resolved, spec)
				if err != nil {
					return retrieval.SearchResult{}, err
				}
				if !matched {
					continue
				}
			}
			result.Hits = append(result.Hits, retrieval.KnowledgeHit{Knowledge: value, Version: retrieval.VersionOf(value), Evidence: candidate.Evidence})
			if resolved.Limit > 0 && len(result.Hits) >= resolved.Limit {
				break
			}
		}
		nextContinuation = page.Continuation
		if page.Exhausted || page.Continuation == "" || (resolved.Limit > 0 && len(result.Hits) >= resolved.Limit) {
			break
		}
		continuation = page.Continuation
	}
	if resolved.Limit > 0 && len(result.Hits) >= resolved.Limit && nextContinuation != "" {
		result.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
			Scope: "repository", Query: queryDigest, SearchView: viewDigest,
			Projection: projectionDigest, Position: nextContinuation,
		})
	}
	return result, nil
}

func hydrateMany(repo knowledge.Repository, commit kernel.CommitID, objectIDs []knowledge.ObjectID) (map[knowledge.ObjectID]knowledge.KnowledgeValue, error) {
	if batch, ok := repo.(knowledge.BatchReadStore); ok {
		return batch.ReadMany(objectIDs, commit)
	}
	out := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	for _, objectID := range objectIDs {
		if _, duplicate := out[objectID]; duplicate {
			continue
		}
		value, err := repo.Read(objectID, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[objectID] = value
	}
	return out, nil
}
