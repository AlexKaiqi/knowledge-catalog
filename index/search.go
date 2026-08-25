package index

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func (idx *Index) Search(repo knowledge.Repository, req reader.SearchRequest) (reader.SearchResult, error) {
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return reader.SearchResult{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return reader.SearchResult{}, err
	}
	if meta.Basis == "" {
		return reader.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "projection for %s is empty; write or index-sync first", repo.ID())
	}
	return idx.searchEngine(repo, eng, meta.Basis, req)
}

// SearchAt evaluates SEARCH at a frozen commit without rewinding the live engine.
func (idx *Index) SearchAt(repo knowledge.Repository, commit kernel.CommitID, req reader.SearchRequest) (reader.SearchResult, error) {
	if commit == "" {
		return idx.Search(repo, req)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return reader.SearchResult{}, err
	}
	eng, err := idx.engineForCommit(repo.ID(), commit)
	if err != nil {
		return reader.SearchResult{}, err
	}
	return idx.searchEngine(repo, eng, commit, req)
}

func (idx *Index) searchEngine(repo knowledge.Repository, eng Engine, commit kernel.CommitID, req reader.SearchRequest) (reader.SearchResult, error) {
	result := reader.SearchResult{
		View:         reader.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}},
		Completeness: reader.CompletenessComplete,
		Hits:         []reader.KnowledgeHit{},
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return reader.SearchResult{}, err
	}
	var identity ProviderIdentity
	if provider, ok := eng.(ProviderIdentity); ok {
		identity = provider
	}
	plan, err := PlanRetrieval(eng, identity, req, spec)
	if err != nil {
		return reader.SearchResult{}, err
	}
	resolved := plan.Search
	needsResidual := false
	for _, fragment := range plan.Fragments {
		capability := fragment.Capability
		if capability.Guarantee == GuaranteeUnsupported {
			return reader.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
		if capability.Guarantee == GuaranteeSuperset {
			needsResidual = true
		}
		if capability.Guarantee == GuaranteeApproximate || capability.Coverage < 1 {
			result.Completeness = reader.CompletenessPartial
			result.Claims = append(result.Claims, "provider guarantee="+string(capability.Guarantee))
		}
	}
	viewDigest := reader.SearchViewDigest(result.View)
	queryDigest := reader.SearchQueryDigest(resolved)
	projectionDigest := kernel.CanonicalDigest(plan.Projection)
	continuation := ""
	if resolved.Continuation != "" {
		state, err := reader.DecodeContinuation(resolved.Continuation)
		if err != nil || state.Scope != "repository" || state.Query != queryDigest || state.View != viewDigest || state.Projection != projectionDigest {
			return reader.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this search view")
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
			return reader.SearchResult{}, err
		}
		for _, candidate := range page.Candidates {
			if candidate.Repository != "" && candidate.Repository != repo.ID() {
				result.Completeness = reader.CompletenessPartial
				result.Claims = append(result.Claims, "candidate repository mismatch")
				continue
			}
			candidate.Repository = repo.ID()
			if candidate.Basis != commit {
				result.Completeness = reader.CompletenessPartial
				result.Claims = append(result.Claims, "candidate basis mismatch")
				continue
			}
			value, err := repo.Read(candidate.ObjectID, commit)
			if err != nil {
				if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
					result.Completeness = reader.CompletenessPartial
					result.Claims = append(result.Claims, "candidate removed before hydrate: "+string(candidate.ObjectID))
					continue
				}
				return reader.SearchResult{}, err
			}
			if needsResidual {
				matched, err := matchesResidual(repo, value, resolved, spec)
				if err != nil {
					return reader.SearchResult{}, err
				}
				if !matched {
					continue
				}
			}
			result.Hits = append(result.Hits, reader.KnowledgeHit{Knowledge: value, Version: reader.VersionOf(value), Evidence: candidate.Evidence})
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
		result.Continuation = reader.EncodeContinuation(reader.ContinuationState{
			Scope: "repository", Query: queryDigest, View: viewDigest,
			Projection: projectionDigest, Position: nextContinuation,
		})
	}
	return result, nil
}
