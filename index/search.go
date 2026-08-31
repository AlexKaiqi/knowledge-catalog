package index

import (
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// SearchAt evaluates SEARCH at a frozen commit without rewinding the live engine.
func (idx *Index) SearchAt(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	if commit == "" {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "search requires an explicit fixed commit")
	}
	eng, release, err := idx.acquireEngineForCommit(repo.ID(), commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer release()
	meta, err := eng.LoadMeta()
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := requireSearchProjection(repo, eng, meta, commit); err != nil {
		return retrieval.SearchResult{}, err
	}
	return idx.searchEngine(repo, eng, commit, req)
}

func requireSearchProjection(repo knowledge.Repository, eng Engine, meta Meta, commit kernel.CommitID) error {
	if meta.State == ProjectionStateBuilding || meta.State == ProjectionStateUpdating {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "search projection for %s is being built", repo.ID())
	}
	if meta.State != ProjectionStateReady || meta.Basis == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"search projection for %s is not available; run operations projection sync", repo.ID())
	}
	if commit != "" && meta.Basis != commit {
		return kernel.Fail(kernel.ErrPreconditionFailed,
			"search projection basis %s does not match fixed commit %s", meta.Basis, commit)
	}
	spec, err := specAtCommit(repo, meta.Basis)
	if err != nil {
		return err
	}
	if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "search projection metadata does not match its fixed basis")
	}
	return nil
}

func (idx *Index) searchEngine(repo knowledge.Repository, eng Engine, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return idx.searchEngineAt(repo, eng, commit, req, retrieval.SearchView{
		Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
	}, nil)
}

// SearchStateAt evaluates a request against an already published State
// projection and hydrates candidates from its same-revision Serving State.
func (idx *Index) SearchStateAt(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return idx.SearchStateAtRevision(repo, commit, "", req)
}

// SearchStateAtRevision additionally pins the revision selected by an outer
// Workspace SearchView. If a refresh wins between planning and member
// execution, the request fails instead of mixing observation bases.
func (idx *Index) SearchStateAtRevision(repo knowledge.Repository, commit kernel.CommitID, revision string, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	idx.stateMu.RLock()
	defer idx.stateMu.RUnlock()
	state := idx.states[stateStoreKey(repo.ID(), commit)]
	if state == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "State projection for %s at %s is not prepared", repo.ID(), commit)
	}
	if revision != "" && state.revision != revision {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "dynamic SearchView revision changed; restart the search")
	}
	eng, err := idx.stateEngineAt(repo.ID(), commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	view := retrieval.SearchView{
		Snapshots:           map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		ProjectionRevisions: map[kernel.RepositoryID]string{repo.ID(): state.revision},
	}
	return idx.searchEngineAt(repo, eng, commit, req, view, state)
}

func (idx *Index) searchEngineAt(repo knowledge.Repository, eng Engine, commit kernel.CommitID, req retrieval.SearchRequest, view retrieval.SearchView, state *stateProjection) (retrieval.SearchResult, error) {
	result := retrieval.SearchResult{
		SearchView:   view,
		Completeness: retrieval.CompletenessComplete,
		Hits:         []retrieval.KnowledgeHit{},
	}
	planStarted := time.Now()
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
	needsResidual, err := applySearchGuarantees(plan, &result)
	if err != nil {
		return retrieval.SearchResult{}, err
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
	result.Stats.PlanDuration = time.Since(planStarted)
	nextContinuation := ""
	for {
		pageReq := resolved
		if resolved.Limit > 0 {
			pageReq.Limit = resolved.Limit - len(result.Hits)
			if pageReq.Limit <= 0 {
				break
			}
		}
		probeStarted := time.Now()
		page, err := eng.Retrieve(RetrieveRequest{Search: pageReq, Spec: spec, Continuation: continuation})
		result.Stats.ProbeDuration += time.Since(probeStarted)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		if !page.Exhausted && (page.Continuation == "" || page.Continuation == continuation) {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "search provider returned a missing or non-advancing continuation")
		}
		result.Stats.Candidates += len(page.Candidates)
		hydrateStarted := time.Now()
		if err := appendCandidatePage(repo, commit, page, resolved, spec, needsResidual, state, &result); err != nil {
			return retrieval.SearchResult{}, err
		}
		result.Stats.HydrateDuration += time.Since(hydrateStarted)
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

func applySearchGuarantees(plan RetrievalPlan, result *retrieval.SearchResult) (bool, error) {
	needsResidual := false
	for _, fragment := range plan.Fragments {
		capability := fragment.Capability
		if capability.Guarantee == GuaranteeUnsupported {
			return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
		if capability.Guarantee == GuaranteeSuperset {
			needsResidual = true
		}
		if capability.Guarantee == GuaranteeApproximate || capability.Coverage < 1 {
			result.Completeness = retrieval.CompletenessPartial
			result.Stats.MarkPartial("unsupported")
			result.Claims = append(result.Claims, "provider guarantee="+string(capability.Guarantee))
		}
	}
	return needsResidual, nil
}

// appendCandidatePage enforces the untrusted-provider boundary before a hit
// becomes public: repository and basis must match, Canonical is re-read from
// that exact commit, and superset providers are filtered against Canonical.
func appendCandidatePage(repo knowledge.Repository, commit kernel.CommitID, page CandidatePage, resolved retrieval.SearchRequest, spec retrieval.AccessSpec, needsResidual bool, state *stateProjection, result *retrieval.SearchResult) error {
	candidateIDs := make([]knowledge.ObjectID, 0, len(page.Candidates))
	for _, candidate := range page.Candidates {
		if candidate.Repository != "" && candidate.Repository != repo.ID() {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"search candidate repository %s does not match fixed repository %s", candidate.Repository, repo.ID())
		}
		if candidate.Basis != commit {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"search candidate basis %s does not match fixed commit %s", candidate.Basis, commit)
		}
		candidateIDs = append(candidateIDs, candidate.ObjectID)
	}
	hydrated := map[knowledge.ObjectID]knowledge.KnowledgeValue{}
	versions := map[knowledge.ObjectID][]knowledge.UnitObservation{}
	if state != nil {
		for _, id := range candidateIDs {
			record, ok, err := state.store.Get(id)
			if err != nil {
				return err
			}
			if ok {
				hydrated[id] = record.Value
				versions[id] = record.Observations
			}
		}
	} else {
		var err error
		hydrated, err = hydrateMany(repo, commit, candidateIDs)
		if err != nil {
			return err
		}
	}
	result.Stats.Hydrated += len(page.Candidates)
	for _, candidate := range page.Candidates {
		candidate.Repository = repo.ID()
		value, ok := hydrated[candidate.ObjectID]
		if !ok {
			return kernel.Fail(kernel.ErrPreconditionFailed,
				"search candidate %s is missing from repository %s at fixed commit %s", candidate.ObjectID, repo.ID(), commit)
		}
		if needsResidual {
			matched, err := matchesResidual(repo, value, versions[candidate.ObjectID], resolved, spec)
			if err != nil {
				return err
			}
			if !matched {
				result.Stats.Dropped++
				continue
			}
		}
		version := retrieval.VersionOf(value)
		version.Observations = append([]knowledge.UnitObservation(nil), versions[candidate.ObjectID]...)
		result.Hits = append(result.Hits, retrieval.KnowledgeHit{Knowledge: value, Version: version, Evidence: candidate.Evidence})
		if resolved.Limit > 0 && len(result.Hits) >= resolved.Limit {
			break
		}
	}
	return nil
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
