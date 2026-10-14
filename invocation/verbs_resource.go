package invocation

import (
	"context"
	discoveryservice "kc/discovery/service"
	"kc/kernel"
	knowledgeserving "kc/liveaccess"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	reader "kc/object/reader"

	retrieval "kc/discovery"
	"kc/observability"
	"kc/snapshot/dataset"
)

type RerankApplicationRequest struct {
	Candidates []objectmodel.KnowledgeRef
	Spec       retrieval.SemanticOperatorSpec
}

// SearchAndRerankWorkspace is the MVP physical composition. SEARCH owns the
// bounded candidate window and its lane evidence; RERANK consumes those
// already-authorized, hydrated values in one listwise call at the same
// SearchView. It does not resolve the Workspace or read Canonical a second time.
func SearchAndRerankWorkspace(cx *Invocation, spec retrieval.SemanticOperatorSpec, reranker retrieval.Reranker) (any, error) {
	if reranker == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "semantic reranker is not configured")
	}
	if err := retrieval.ValidateSemanticOperatorSpec(spec); err != nil {
		return nil, err
	}
	if spec.Operator != retrieval.OpSemanticRerank {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "search:rerank requires SEMANTIC_RERANK")
	}
	raw, err := SearchWorkspace(cx)
	if err != nil {
		return nil, err
	}
	search, ok := raw.(retrieval.SearchResult)
	if !ok {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "workspace SEARCH returned an invalid result")
	}
	if len(search.Hits) == 0 {
		view := search.SearchView
		return SearchRerankResult{
			Retrieval: search,
			Rerank:    retrieval.SemanticRerankResult{SearchView: &view, Groups: []retrieval.RankGroup{}, Unjudged: []objectmodel.KnowledgeRef{}, Complete: true},
		}, nil
	}
	candidates := make([]retrieval.Candidate, 0, len(search.Hits))
	for i, hit := range search.Hits {
		if hit.Knowledge.Value == nil {
			continue
		}
		candidates = append(candidates, retrieval.Candidate{
			Ref:   objectmodel.KnowledgeRef{Repository: hit.Knowledge.Repository, Object: hit.Knowledge.Address.ObjectID},
			Value: hit.Knowledge.Value, Observations: append([]objectmodel.UnitObservation(nil), hit.Version.Observations...),
			OriginalRank: i + 1, RetrievalEvidence: append([]retrieval.LaneEvidence(nil), hit.Evidence...),
		})
	}
	if len(candidates) == 0 {
		view := search.SearchView
		return SearchRerankResult{
			Retrieval: search,
			Rerank:    retrieval.SemanticRerankResult{SearchView: &view, Groups: []retrieval.RankGroup{}, Unjudged: []objectmodel.KnowledgeRef{}, Complete: true},
		}, nil
	}
	reranked, execution, err := retrieval.ExecuteRerankRecorded(cx.Context, reranker, retrieval.RerankRequest{
		SearchView: search.SearchView, Spec: spec, Candidates: candidates,
	})
	if err != nil {
		observed, observedErr := ObservedRerankExecution(cx, nil, KnowledgeAccesses(search), execution, err)
		return WithRetrievalObservationSource(observed, search), observedErr
	}
	result := SearchRerankResult{Retrieval: search, Rerank: reranked}
	return ObservedRerankExecution(cx, result, KnowledgeAccesses(search), execution, nil)
}

// RerankWorkspace is an application-level Refine operation. It resolves the
// Workspace once, authorizes every explicit candidate, Canonical-reads each
// value at that pin and only then invokes the injected semantic provider.
func RerankWorkspace(cx *Invocation, request RerankApplicationRequest, reranker retrieval.Reranker) (any, error) {
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

	declarations, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
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
	seen := map[objectmodel.KnowledgeRef]struct{}{}
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
		if !AllowedRepoRead(cx.Home, cx.Flags, string(ref.Repository), string(ref.Object)) {
			return nil, kernel.Fail(kernel.ErrForbidden, "one or more rerank candidates are not authorized")
		}
		repo, err := cx.WS.Reader.Require(ref.Repository, kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return nil, err
		}
		raw, err := cx.WS.Reader.Read(ref, commit, nil)
		if err != nil {
			return nil, err
		}
		hydrated, err := liveservice.HydrateRepositoryValue(cx.Context, repo, raw, cx.State, requestContext)
		if err != nil {
			return nil, err
		}
		view.Snapshots[ref.Repository] = commit
		candidates = append(candidates, retrieval.Candidate{
			Ref: ref, Value: hydrated.Value,
			Observations: append([]objectmodel.UnitObservation(nil), hydrated.Observations...),
		})
		accesses = append(accesses, observability.KnowledgeAccess{
			KnowledgeRef: objectmodel.PinnedKnowledgeRef{KnowledgeRef: ref, Commit: commit},
			Observations: append([]objectmodel.UnitObservation(nil), hydrated.Observations...),
		})
	}

	result, execution, err := retrieval.ExecuteRerankRecorded(cx.Context, reranker, retrieval.RerankRequest{
		SearchView: view, Spec: request.Spec, Candidates: candidates,
	})
	if err != nil {
		return ObservedRerankExecution(cx, nil, accesses, execution, err)
	}
	return ObservedRerankExecution(cx, result, accesses, execution, nil)
}

func ObservedRerankExecution(cx *Invocation, output any, accesses []observability.KnowledgeAccess, execution retrieval.RerankExecutionRecord, callErr error) (any, error) {
	if len(execution.Candidates) == 0 {
		return output, callErr
	}
	event, err := RefineEventFromExecution(cx, execution, callErr)
	if err != nil {
		return nil, err
	}
	return ObservedAccessResult{Output: output, Knowledge: accesses, Refine: &event}, callErr
}

func RefineEventFromExecution(cx *Invocation, execution retrieval.RerankExecutionRecord, callErr error) (observability.RefineEvent, error) {
	identity, err := IdentityContextFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	trace, err := TraceContextFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	requestID, err := RequestIDFrom(cx.Flags)
	if err != nil {
		return observability.RefineEvent{}, err
	}
	workspace, err := cx.SetID()
	if err != nil {
		return observability.RefineEvent{}, err
	}
	fields := []string{}
	if execution.Spec.EvaluationProjection != nil {
		fields = append(fields, execution.Spec.EvaluationProjection.Fields...)
	}
	event := observability.RefineEvent{
		Identity: identity, Trace: trace, Action: ActionOf("rerank", cx.Flags), RequestID: requestID, Dataset: workspace,
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
			KnowledgeRef: objectmodel.PinnedKnowledgeRef{KnowledgeRef: candidate.Ref, Commit: commit},
			Value:        candidate.Value, ValueDigest: kernel.CanonicalDigest(candidate.Value), OriginalRank: candidate.OriginalRank,
			RetrievalEvidence: lanes, Observations: append([]objectmodel.UnitObservation(nil), candidate.Observations...),
		})
	}
	if execution.ProviderResult != nil {
		provider := execution.ProviderResult
		groups := make([]observability.RefineRankGroup, len(provider.Groups))
		for i, group := range provider.Groups {
			groups[i] = observability.RefineRankGroup{Rank: group.Rank, Refs: append([]objectmodel.KnowledgeRef(nil), group.Refs...)}
		}
		event.ModelOutput = &observability.RefineModelOutput{
			Provider: provider.Provider, Model: provider.Model, ModelRevision: provider.ModelRevision,
			PromptRevision: provider.PromptRevision, DurationMillis: execution.ProviderDuration.Milliseconds(),
			Groups: groups, Unjudged: append([]objectmodel.KnowledgeRef(nil), provider.Unjudged...),
		}
	}
	if callErr != nil {
		fault := kernel.Normalize(callErr)
		event.Outcome = "ERROR"
		event.Error = map[string]any{"code": fault.Code, "message": fault.Message}
	}
	return event, nil
}

// VerbResourceAccess hydrates an Aspect Binding through the wall-side
// resource-access/v1 runtime. ResourceDescriptor operations use invoke.
func VerbResourceAccess(cx *Invocation) (any, error) {
	if cx.Flag("operation") != "" || cx.Flag("input") != "" || cx.Flag("implementation") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"access hydrates a Binding with --aspect; use kc invoke for an object Interface operation")
	}
	address, err := bindingAddress(cx)
	if err != nil {
		return nil, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return OnTarget(cx, func(declarations *reader.Serving, _ dataset.Service) (any, error) {
		return resourceAccessAt(cx, declarations, address, lookup, request)
	}, func(repository kernel.RepositoryID, commit kernel.CommitID) (any, error) {
		return resourceAccessAt(cx, repositoryServingAt(cx, repository, commit, "resource-access"), address, lookup, request)
	})
}

func resourceAccessAt(cx *Invocation, declarations *reader.Serving, address objectmodel.Address, lookup knowledgeserving.StateLookup, request objectmodel.RequestContext) (any, error) {
	bindings, err := knowledgeserving.ResolveBinding(declarations, address)
	if err != nil {
		return nil, err
	}
	values, err := liveservice.OpenRequest(declarations, lookup, request).ReadAddress(cx.Context, address)
	if err != nil {
		return nil, err
	}
	results := make([]knowledgeserving.StateObservation, 0, len(values))
	for _, value := range values {
		if len(value.Failures) != 0 {
			failure := value.Failures[0]
			return nil, kernel.Fail(failure.Code, "%s", failure.Message)
		}
		if len(value.Observations) != 1 {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "access requires one observed Binding value at %s", objectmodel.AddressKey(address))
		}
		results = append(results, knowledgeserving.StateObservation{Value: value.Value, Basis: value.Observations[0].Basis})
	}
	return ObservedAccessResult{
		Output:    map[string]any{"bindings": bindings, "observations": results},
		Knowledge: KnowledgeAccesses(values),
	}, nil
}

func VerbResourceInvoke(cx *Invocation) (any, error) {
	if cx.Flag("aspect") != "" || cx.Flag("member") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"invoke selects an Interface with --implementation; use kc access --aspect for Binding hydration")
	}
	operation, err := cx.Require("operation")
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "invoke requires --operation and --input")
	}
	if cx.Flag("implementation") != "" {
		return InvokeObjectInterface(cx, operation)
	}
	return AccessResourceOperation(cx, operation)
}

func AccessResourceOperation(cx *Invocation, operation string) (any, error) {
	if cx.Flag("aspect") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"invoke calls a ResourceDescriptor operation; use kc access --aspect for Binding hydration")
	}
	input, err := invocationInput(cx)
	if err != nil {
		return nil, err
	}

	descriptor, err := PinnedResourceDescriptor(cx)
	if err != nil {
		return nil, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	accessor, ok := lookup.(ResourceOperationAccessor)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource-access/v1 runtime does not support descriptor operations")
	}
	identity, err := IdentityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	trace, err := TraceContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	requestID, err := RequestIDFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	request, err := knowledgeserving.DescriptorOperation(ResourceDescriptorCoordinate{
		ObjectID: descriptor.ObjectID, Repository: descriptor.Repository, Commit: descriptor.Commit,
	}, descriptor.Value, operation, input, StateRuntimeIdentity{
		Principal: identity.Principal, OnBehalfOf: identity.OnBehalfOf, RequestID: requestID,
		TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
	})
	if err != nil {
		return nil, err
	}
	result, err := accessor.AccessResource(cx.Context, request)
	if err != nil {
		return nil, err
	}
	return ObservedAccessResult{Output: result, Knowledge: []observability.KnowledgeAccess{{KnowledgeRef: objectmodel.PinnedKnowledgeRef{KnowledgeRef: objectmodel.KnowledgeRef{Repository: descriptor.Repository, Object: descriptor.ObjectID}, Commit: descriptor.Commit}}}}, nil
}

func PinnedResourceDescriptor(cx *Invocation) (PinnedDescriptor, error) {
	objectID, err := cx.Require("object")
	if err != nil {
		return PinnedDescriptor{}, err
	}
	result, err := OnTarget(cx, func(declarations *reader.Serving, _ dataset.Service) (any, error) {
		values, err := declarations.Read(objectmodel.ObjectID(objectID), nil)
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "ResourceDescriptor %s resolved to %d values", objectID, len(values))
		}
		value := values[0]
		return PinnedDescriptor{Repository: value.Repository, Commit: value.Commit, ObjectID: value.ObjectID, Value: value.Value}, nil
	}, func(repository kernel.RepositoryID, commit kernel.CommitID) (any, error) {
		value, err := cx.WS.Reader.Read(objectmodel.KnowledgeRef{Repository: repository, Object: objectmodel.ObjectID(objectID)}, commit, nil)
		return PinnedDescriptor{Repository: value.Repository, Commit: value.Commit, ObjectID: value.Address.ObjectID, Value: value.Value}, err
	})
	if err != nil {
		return PinnedDescriptor{}, err
	}
	return result.(PinnedDescriptor), nil
}

// StateRuntimeIdentity / ResourceOperationRequest 等是 resource-access/v1 的
// wire 形状，实义在 kc/knowledge/serving（与 StateLookup 端口同居一处）；
// invocation 保留别名以维持公开形状（B6 公开 API 不变）。
type StateRuntimeIdentity = knowledgeserving.StateRuntimeIdentity
type ResourceDescriptorCoordinate = knowledgeserving.ResourceDescriptorCoordinate
type ResourceOperationRequest = knowledgeserving.ResourceOperationRequest
type ResourceOperationAccessor = knowledgeserving.ResourceOperationAccessor

type PinnedDescriptor struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	ObjectID   objectmodel.ObjectID
	Value      any
}

func HydrateSearchHit(ctx context.Context, logical *liveservice.Service, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	return discoveryservice.HydrateSearchHit(ctx, logical, hit)
}
