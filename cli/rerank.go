package cli

import (
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/observability"
	"kc/retrieval"
)

type rerankApplicationRequest struct {
	Candidates []knowledge.KnowledgeRef
	Spec       retrieval.SemanticOperatorSpec
}

type searchRerankResult struct {
	Retrieval retrieval.SearchResult         `json:"retrieval"`
	Rerank    retrieval.SemanticRerankResult `json:"rerank"`
}

// searchAndRerankWorkspace is the MVP physical composition. SEARCH owns the
// bounded candidate window and its lane evidence; RERANK consumes those
// already-authorized, hydrated values in one listwise call at the same
// SearchView. It does not resolve the Workspace or read Canonical a second time.
func searchAndRerankWorkspace(cx *invocation, spec retrieval.SemanticOperatorSpec, reranker retrieval.Reranker) (any, error) {
	if reranker == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "semantic reranker is not configured")
	}
	if err := retrieval.ValidateSemanticOperatorSpec(spec); err != nil {
		return nil, err
	}
	if spec.Operator != retrieval.OpSemanticRerank {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search:rerank requires SEMANTIC_RERANK")
	}
	raw, err := searchWorkspace(cx)
	if err != nil {
		return nil, err
	}
	search, ok := raw.(retrieval.SearchResult)
	if !ok {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "workspace SEARCH returned an invalid result")
	}
	if len(search.Hits) == 0 {
		view := search.SearchView
		return searchRerankResult{
			Retrieval: search,
			Rerank:    retrieval.SemanticRerankResult{SearchView: &view, Groups: []retrieval.RankGroup{}, Unjudged: []knowledge.KnowledgeRef{}, Complete: true},
		}, nil
	}
	candidates := make([]retrieval.Candidate, 0, len(search.Hits))
	for i, hit := range search.Hits {
		if hit.Knowledge.Value == nil {
			continue
		}
		candidates = append(candidates, retrieval.Candidate{
			Ref:   knowledge.KnowledgeRef{Repository: hit.Knowledge.Repository, Object: hit.Knowledge.Address.ObjectID},
			Value: hit.Knowledge.Value, Observations: append([]knowledge.UnitObservation(nil), hit.Version.Observations...),
			OriginalRank: i + 1, RetrievalEvidence: append([]retrieval.LaneEvidence(nil), hit.Evidence...),
		})
	}
	if len(candidates) == 0 {
		view := search.SearchView
		return searchRerankResult{
			Retrieval: search,
			Rerank:    retrieval.SemanticRerankResult{SearchView: &view, Groups: []retrieval.RankGroup{}, Unjudged: []knowledge.KnowledgeRef{}, Complete: true},
		}, nil
	}
	reranked, execution, err := retrieval.ExecuteRerankRecorded(cx.Context, reranker, retrieval.RerankRequest{
		SearchView: search.SearchView, Spec: spec, Candidates: candidates,
	})
	if err != nil {
		observed, observedErr := observedRerankExecution(cx, nil, knowledgeAccesses(search), execution, err)
		return withRetrievalObservationSource(observed, search), observedErr
	}
	result := searchRerankResult{Retrieval: search, Rerank: reranked}
	return observedRerankExecution(cx, result, knowledgeAccesses(search), execution, nil)
}

// rerankWorkspace is an application-level Refine operation. It resolves the
// Workspace once, authorizes every explicit candidate, Canonical-reads each
// value at that pin and only then invokes the injected semantic provider.
func rerankWorkspace(cx *invocation, request rerankApplicationRequest, reranker retrieval.Reranker) (any, error) {
	if err := retrieval.ValidateSemanticOperatorSpec(request.Spec); err != nil {
		return nil, err
	}
	if request.Spec.Operator != retrieval.OpSemanticRerank {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "rerank requires SEMANTIC_RERANK")
	}
	if len(request.Candidates) == 0 || len(request.Candidates) > retrieval.MaxRerankCandidates {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"semantic rerank requires between 1 and %d candidates", retrieval.MaxRerankCandidates)
	}
	if reranker == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "semantic reranker is not configured")
	}

	declarations, _, err := openServing(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	pin := declarations.Pin()
	requestContext, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	view := retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}}
	candidates := make([]retrieval.Candidate, 0, len(request.Candidates))
	accesses := make([]observability.KnowledgeAccess, 0, len(request.Candidates))
	seen := map[knowledge.KnowledgeRef]struct{}{}
	for _, ref := range request.Candidates {
		if ref.Repository == "" || ref.Object == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "rerank candidate requires repository and object")
		}
		if _, duplicate := seen[ref]; duplicate {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "rerank candidate %s is duplicated", ref.Object)
		}
		seen[ref] = struct{}{}
		commit, member := pin.Repositories[ref.Repository]
		if !member {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "rerank candidate repository is not in the resolved Workspace")
		}
		if !allowedRepoRead(cx.Home, cx.Flags, string(ref.Repository), string(ref.Object)) {
			return nil, kernel.Fail(kernel.ErrForbidden, "one or more rerank candidates are not authorized")
		}
		repo, err := cx.WS.Reader.Require(ref.Repository, kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return nil, err
		}
		raw, err := repo.Read(ref.Object, commit)
		if err != nil {
			return nil, err
		}
		hydrated, err := knowledgeserving.HydrateRepositoryValue(cx.Context, repo, raw, cx.State, requestContext)
		if err != nil {
			return nil, err
		}
		view.Snapshots[ref.Repository] = commit
		candidates = append(candidates, retrieval.Candidate{
			Ref: ref, Value: hydrated.Value,
			Observations: append([]knowledge.UnitObservation(nil), hydrated.Observations...),
		})
		accesses = append(accesses, observability.KnowledgeAccess{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: commit},
			Observations: append([]knowledge.UnitObservation(nil), hydrated.Observations...),
		})
	}

	result, execution, err := retrieval.ExecuteRerankRecorded(cx.Context, reranker, retrieval.RerankRequest{
		SearchView: view, Spec: request.Spec, Candidates: candidates,
	})
	if err != nil {
		return observedRerankExecution(cx, nil, accesses, execution, err)
	}
	return observedRerankExecution(cx, result, accesses, execution, nil)
}

func observedRerankExecution(cx *invocation, output any, accesses []observability.KnowledgeAccess, execution retrieval.RerankExecutionRecord, callErr error) (any, error) {
	if len(execution.Candidates) == 0 {
		return output, callErr
	}
	event, err := refineEventFromExecution(cx, execution, callErr)
	if err != nil {
		return nil, err
	}
	return observedAccessResult{Output: output, Knowledge: accesses, Refine: &event}, callErr
}

func attachRefineEvidenceID(result any, evidenceID string) any {
	observed, ok := result.(observedAccessResult)
	if !ok || evidenceID == "" {
		return result
	}
	switch output := observed.Output.(type) {
	case retrieval.SemanticRerankResult:
		if output.Evidence != nil {
			output.Evidence.RefineEvidenceID = evidenceID
		}
		observed.Output = output
	case searchRerankResult:
		if output.Rerank.Evidence != nil {
			output.Rerank.Evidence.RefineEvidenceID = evidenceID
		}
		observed.Output = output
	}
	return observed
}

func refineEventFromExecution(cx *invocation, execution retrieval.RerankExecutionRecord, callErr error) (observability.RefineEvent, error) {
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	trace, err := traceContextFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	requestID, err := requestIDFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	workspace, err := cx.workspaceID()
	if err != nil {
		return observability.RefineEvent{}, err
	}
	fields := []string{}
	if execution.Spec.EvaluationProjection != nil {
		fields = append(fields, execution.Spec.EvaluationProjection.Fields...)
	}
	event := observability.RefineEvent{
		Identity: identity, Trace: trace, Action: actionOf("rerank", cx.Flags), RequestID: requestID, Workspace: workspace,
		SearchView: observability.RefineSearchView{
			Snapshots: execution.SearchView.Snapshots, ProjectionRevisions: execution.SearchView.ProjectionRevisions,
		},
		Spec: observability.RefineSpec{
			SpecRef: execution.Spec.SpecRef, Revision: execution.Spec.Revision, Operator: string(execution.Spec.Operator),
			Criterion: execution.Spec.Criterion, EvaluationFields: fields,
			TopK: execution.Spec.OutputContract.TopK, AllowTies: execution.Spec.OutputContract.AllowTies,
			AllowUnjudged: execution.Spec.OutputContract.AllowUnjudged,
		},
		CandidateDigest: execution.CandidateDigest, ProjectedBytes: execution.ProjectedInputBytes,
		Candidates: []observability.RefineCandidate{}, Outcome: "COMPLETED",
	}
	if query, ok := cx.Flags["_search-request"].(retrieval.SearchRequest); ok {
		event.RetrievalQuery = query
	}
	for _, candidate := range execution.Candidates {
		commit, ok := execution.SearchView.Snapshots[candidate.Ref.Repository]
		if !ok {
			return observability.RefineEvent{}, kernel.Fail(kernel.ErrPreconditionFailed, "refine evidence candidate is outside SearchView")
		}
		lanes := make([]observability.RefineLaneEvidence, len(candidate.RetrievalEvidence))
		for i, lane := range candidate.RetrievalEvidence {
			fields := make([]observability.RefineFieldRef, len(lane.MatchedFields))
			for j, field := range lane.MatchedFields {
				fields[j] = observability.RefineFieldRef{Schema: field.Schema, Aspect: field.Aspect, Path: field.Path}
			}
			lanes[i] = observability.RefineLaneEvidence{
				Provider: lane.Provider, Lane: lane.Lane, Guarantee: lane.Guarantee,
				LocalRank: lane.LocalRank, LocalScore: lane.LocalScore, MatchedFields: fields,
			}
		}
		event.Candidates = append(event.Candidates, observability.RefineCandidate{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: candidate.Ref, Commit: commit},
			Value:        candidate.Value, ValueDigest: kernel.CanonicalDigest(candidate.Value), OriginalRank: candidate.OriginalRank,
			RetrievalEvidence: lanes, Observations: append([]knowledge.UnitObservation(nil), candidate.Observations...),
		})
	}
	if execution.ProviderResult != nil {
		provider := execution.ProviderResult
		groups := make([]observability.RefineRankGroup, len(provider.Groups))
		for i, group := range provider.Groups {
			groups[i] = observability.RefineRankGroup{Rank: group.Rank, Refs: append([]knowledge.KnowledgeRef(nil), group.Refs...)}
		}
		event.ModelOutput = &observability.RefineModelOutput{
			Provider: provider.Provider, Model: provider.Model, ModelRevision: provider.ModelRevision,
			PromptRevision: provider.PromptRevision, DurationMillis: execution.ProviderDuration.Milliseconds(),
			Groups: groups, Unjudged: append([]knowledge.KnowledgeRef(nil), provider.Unjudged...),
		}
	}
	if callErr != nil {
		fault := kernel.Normalize(callErr)
		event.Outcome = "ERROR"
		event.Error = map[string]any{"code": fault.Code, "message": fault.Message}
	}
	return event, nil
}
