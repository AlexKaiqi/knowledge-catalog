package index

import (
	"context"
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

// CheckSearchProjectionAt reports the observed Snapshot projection metadata
// and validates the same fixed-basis preconditions as SearchAt. It never probes
// a query, retrieves candidates, hydrates objects, or builds a projection.
// Metadata remains available with a readiness error so callers can distinguish
// a building, failed, retired, or absent projection. Success does not establish
// authorization, support for a particular query, or dynamic State readiness.
func (idx *Index) CheckSearchProjectionAt(repo knowledge.Repository, commit kernel.CommitID) (Meta, error) {
	if commit == "" {
		return Meta{}, kernel.Fail(kernel.ErrUsageInvalid, "projection readiness requires an explicit fixed commit")
	}
	eng, release, err := idx.acquireEngineForCommit(authorityEngineID(repo), commit)
	if err != nil {
		return Meta{}, err
	}
	defer release()
	meta, err := eng.LoadMeta()
	if err != nil {
		return Meta{}, err
	}
	return meta, requireSearchProjection(repo, eng, meta, commit)
}

// SearchAt evaluates SEARCH at a frozen commit without rewinding the live engine.
func (idx *Index) SearchAt(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return idx.SearchAtContext(context.Background(), repo, commit, req)
}

func (idx *Index) SearchAtContext(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	ctx, cancel := WithSearchBudget(ctx, SearchBudget{})
	defer cancel()
	if ctx.Err() != nil && context.Cause(ctx) != errSearchBudgetDeadline {
		return retrieval.SearchResult{}, ctx.Err()
	}
	if commit == "" {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "search requires an explicit fixed commit")
	}
	eng, release, err := idx.acquireEngineForCommitContext(ctx, authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	defer release()
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	if err := requireSearchProjection(repo, eng, meta, commit); err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	return idx.searchEngineAtContext(ctx, repo, eng, commit, req, retrieval.SearchView{
		Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
	}, nil)
}

// SemanticWindowAtContext serves one approximate k-NN window on the fixed
// repository version (RETRIEVAL.md §8.1). Eligibility (a ready vector
// projection) is checked before any recall work; the result keeps partial
// completeness and the vector lane's approximate evidence, and the caller
// (knowledgeapp) adds the envelope disclosures. No continuation: a window is
// one bounded top-K request, never paged.
func (idx *Index) SemanticWindowAtContext(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest, queryVector []float32) (retrieval.SearchResult, error) {
	ctx, cancel := WithSearchBudget(ctx, SearchBudget{})
	defer cancel()
	if ctx.Err() != nil && context.Cause(ctx) != errSearchBudgetDeadline {
		return retrieval.SearchResult{}, ctx.Err()
	}
	if commit == "" {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "semantic recall requires an explicit fixed commit")
	}
	if len(queryVector) == 0 {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "semantic recall requires a query vector")
	}
	eng, release, err := idx.acquireEngineForCommitContext(ctx, authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	defer release()
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	if err := requireSearchProjection(repo, eng, meta, commit); err != nil {
		return retrieval.SearchResult{}, searchPreparationError(ctx, err)
	}
	window, ok := eng.(SemanticWindowRetriever)
	if !ok {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"search projection for %s has no vector window", repo.ID())
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	windowReq := req
	windowReq.Continuation = ""
	page, err := window.SemanticWindowContext(ctx, RetrieveRequest{Search: windowReq, Spec: spec}, queryVector)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	result := retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}},
		Completeness: retrieval.CompletenessPartial,
		Hits:         []retrieval.KnowledgeHit{},
	}
	result.Stats.Candidates += len(page.Candidates)
	unscopedCount := len(page.Candidates)
	page, err = filterCandidateScope(ctx, repo.ID(), commit, page)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	result.Stats.Dropped += unscopedCount - len(page.Candidates)
	if err := idx.appendCandidatePage(repo, commit, page, req, spec, false, nil, &result); err != nil {
		return retrieval.SearchResult{}, err
	}
	return result, nil
}

func requireSearchProjection(repo knowledge.Repository, eng Engine, meta Meta, commit kernel.CommitID) error {
	if meta.State == ProjectionStateBuilding || meta.State == ProjectionStateUpdating {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "search projection for %s is being built", repo.ID())
	}
	if meta.State != ProjectionStateReady || meta.Basis == "" {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"search projection for %s is not available", repo.ID())
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

// SearchStateAt evaluates a request against an already published State
// projection and hydrates candidates from its same-revision Serving State.
func (idx *Index) SearchStateAt(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return idx.SearchStateAtRevision(repo, commit, "", req)
}

// SearchStateAtRevision additionally pins the revision selected by an outer
// Workspace SearchView. If a refresh wins between planning and member
// execution, the request fails instead of mixing observation bases.
func (idx *Index) SearchStateAtRevision(repo knowledge.Repository, commit kernel.CommitID, revision string, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	return idx.SearchStateAtRevisionContext(context.Background(), repo, commit, revision, req)
}

func (idx *Index) SearchStateAtRevisionContext(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID, revision string, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	ctx, cancel := WithSearchBudget(ctx, SearchBudget{})
	defer cancel()
	if ctx.Err() != nil && context.Cause(ctx) != errSearchBudgetDeadline {
		return retrieval.SearchResult{}, ctx.Err()
	}
	idx.stateMu.RLock()
	defer idx.stateMu.RUnlock()
	state := idx.states[stateStoreKey(repo.ID(), commit)]
	if state == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "State projection for %s at %s is not prepared", repo.ID(), commit)
	}
	if revision != "" && state.revision != revision {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "dynamic SearchView revision changed; restart the search")
	}
	eng, err := idx.stateEngineAt(authorityEngineID(repo), commit)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	view := retrieval.SearchView{
		Snapshots:           map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit},
		ProjectionRevisions: map[kernel.RepositoryID]string{repo.ID(): state.revision},
	}
	return idx.searchEngineAtContext(ctx, repo, eng, commit, req, view, state)
}

func (idx *Index) searchEngineAtContext(ctx context.Context, repo knowledge.Repository, eng Engine, commit kernel.CommitID, req retrieval.SearchRequest, view retrieval.SearchView, state *stateProjection) (retrieval.SearchResult, error) {
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
	var matchVerifier ResidualMatchVerifier
	if needsResidual && retrieval.SearchHasOp(resolved, retrieval.OpMatch) {
		var ok bool
		matchVerifier, ok = eng.(ResidualMatchVerifier)
		if !ok {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "provider cannot prove analyzed MATCH residual semantics")
		}
	}
	viewDigest := retrieval.SearchViewDigest(result.SearchView)
	queryDigest := retrieval.SearchQueryDigest(resolved)
	projectionDigest := kernel.CanonicalDigest(plan.Projection)
	if scope, ok := ctx.Value(candidateScopeKey{}).(candidateScope); ok {
		projectionDigest = kernel.CanonicalDigest([]any{projectionDigest, scope.digest})
	}
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
	nextContinuation := continuation
	budgetStopped := false
	for {
		pageReq := resolved
		if resolved.Limit > 0 {
			pageReq.Limit = resolved.Limit - len(result.Hits)
			if pageReq.Limit <= 0 {
				break
			}
		}
		reserved, finish, reason, err := reserveSearchPage(ctx, pageReq.Limit)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		if reason != "" {
			markSearchBudget(&result, reason)
			budgetStopped = true
			break
		}
		pageReq.Limit = reserved
		probeStarted := time.Now()
		var page CandidatePage
		providerReq := RetrieveRequest{Search: pageReq, Spec: spec, Continuation: continuation}
		if cancellable, ok := eng.(ContextRetriever); ok {
			page, err = cancellable.RetrieveContext(ctx, providerReq)
		} else {
			page, err = eng.Retrieve(providerReq)
		}
		result.Stats.ProbeDuration += time.Since(probeStarted)
		if err != nil {
			finish(0)
			if reason, canceled := searchContextStatus(ctx); reason != "" {
				markSearchBudget(&result, reason)
				budgetStopped = true
				break
			} else if canceled != nil {
				return retrieval.SearchResult{}, canceled
			}
			return retrieval.SearchResult{}, err
		}
		finish(len(page.Candidates))
		if len(page.Candidates) > reserved {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "search provider exceeded candidate page limit")
		}
		if reason, canceled := searchContextStatus(ctx); reason != "" {
			markSearchBudget(&result, reason)
			budgetStopped = true
			break
		} else if canceled != nil {
			return retrieval.SearchResult{}, canceled
		}
		if !page.Exhausted && (page.Continuation == "" || page.Continuation == continuation) {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "search provider returned a missing or non-advancing continuation")
		}
		result.Stats.Candidates += len(page.Candidates)
		hydrateStarted := time.Now()
		hitsBeforePage := len(result.Hits)
		unscopedCount := len(page.Candidates)
		page, err = filterCandidateScope(ctx, repo.ID(), commit, page)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		result.Stats.Dropped += unscopedCount - len(page.Candidates)
		if err := idx.appendCandidatePage(repo, commit, page, resolved, spec, needsResidual, state, &result, matchVerifier); err != nil {
			if reason, canceled := searchContextStatus(ctx); reason != "" {
				result.Hits = result.Hits[:hitsBeforePage]
				markSearchBudget(&result, reason)
				budgetStopped = true
				break
			} else if canceled != nil {
				return retrieval.SearchResult{}, canceled
			}
			return retrieval.SearchResult{}, err
		}
		result.Stats.HydrateDuration += time.Since(hydrateStarted)
		if reason, canceled := searchContextStatus(ctx); reason != "" {
			result.Hits = result.Hits[:hitsBeforePage]
			markSearchBudget(&result, reason)
			budgetStopped = true
			break
		} else if canceled != nil {
			return retrieval.SearchResult{}, canceled
		}
		nextContinuation = page.Continuation
		if page.Exhausted || page.Continuation == "" || (resolved.Limit > 0 && len(result.Hits) >= resolved.Limit) {
			break
		}
		continuation = page.Continuation
	}
	if budgetStopped || (resolved.Limit > 0 && len(result.Hits) >= resolved.Limit && nextContinuation != "") {
		result.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
			Scope: "repository", Query: queryDigest, SearchView: viewDigest,
			Projection: projectionDigest, Position: nextContinuation,
		})
	}
	return result, nil
}

func applySearchGuarantees(plan RetrievalPlan, result *retrieval.SearchResult) (bool, error) {
	needsResidual := false
	if err := applyCapabilityGuarantee(plan.Composition, &needsResidual, result); err != nil {
		return false, err
	}
	for _, fragment := range plan.Fragments {
		if err := applyCapabilityGuarantee(fragment.Capability, &needsResidual, result); err != nil {
			return false, err
		}
	}
	return needsResidual, nil
}

func applyCapabilityGuarantee(capability Capability, needsResidual *bool, result *retrieval.SearchResult) error {
	if capability.Guarantee == GuaranteeUnsupported {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
	}
	if capability.Guarantee == GuaranteeSuperset {
		*needsResidual = true
	}
	if capability.Guarantee == GuaranteeApproximate || capability.Coverage < 1 {
		result.Completeness = retrieval.CompletenessPartial
		result.Stats.MarkPartial("unsupported")
		result.Claims = append(result.Claims, "provider guarantee="+string(capability.Guarantee))
	}
	return nil
}

// appendCandidatePage enforces the untrusted-provider boundary before a hit
// becomes public: repository and basis must match, Canonical is re-read from
// that exact commit, and superset providers are filtered against Canonical.
func (idx *Index) appendCandidatePage(repo knowledge.Repository, commit kernel.CommitID, page CandidatePage, resolved retrieval.SearchRequest, spec retrieval.AccessSpec, needsResidual bool, state *stateProjection, result *retrieval.SearchResult, verifier ...ResidualMatchVerifier) error {
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
		hydrated, err = idx.hydrateMany(repo, commit, candidateIDs)
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
			matched, err := matchesResidual(repo, value, versions[candidate.ObjectID], resolved, spec, verifier...)
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
