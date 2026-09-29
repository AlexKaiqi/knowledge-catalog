package invocation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"kc/kernel"
	knowledge "kc/knowledge"
	reader "kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/knowledgeapp"
	"kc/observability"
	"kc/retrieval"
	"strings"
)

type RerankApplicationRequest struct {
	Candidates []knowledge.KnowledgeRef
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
		return SearchRerankResult{
			Retrieval: search,
			Rerank:    retrieval.SemanticRerankResult{SearchView: &view, Groups: []retrieval.RankGroup{}, Unjudged: []knowledge.KnowledgeRef{}, Complete: true},
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

// VerbResourceAccess hydrates an Aspect Binding through the wall-side
// resource-access/v1 runtime. ResourceDescriptor operations use invoke.
func VerbResourceAccess(cx *Invocation) (any, error) {
	if cx.Flag("operation") != "" || cx.Flag("input") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"access hydrates a Binding with --aspect; use kc invoke for a ResourceDescriptor operation")
	}
	resolved, err := VerbResolveBinding(cx)
	if err != nil {
		return nil, err
	}
	lookup, err := ResourceLookup(cx)
	if err != nil {
		return nil, err
	}
	bindings := []reader.ResolvedBinding{}
	switch value := resolved.(type) {
	case reader.ResolvedBinding:
		bindings = append(bindings, value)
	case []reader.ResolvedBinding:
		bindings = append(bindings, value...)
	default:
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resolved Binding has an unsupported shape")
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
	results := make([]knowledgeserving.StateObservation, 0, len(bindings))
	for _, binding := range bindings {
		observation, err := lookup.LookupState(cx.Context, knowledgeserving.StateLookupRequest{
			Binding: binding, SchemaRef: binding.SchemaRef, Origin: binding.Origin,
			Identity: identity, Trace: trace, RequestID: requestID,
		})
		if err != nil {
			return nil, err
		}
		results = append(results, observation)
	}
	return map[string]any{"bindings": bindings, "observations": results}, nil
}

func VerbResourceInvoke(cx *Invocation) (any, error) {
	if cx.Flag("aspect") != "" || cx.Flag("member") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"invoke calls a ResourceDescriptor operation; use kc access --aspect for Binding hydration")
	}
	operation, err := cx.Require("operation")
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "invoke requires --operation and --input")
	}
	return AccessResourceOperation(cx, operation)
}

func AccessResourceOperation(cx *Invocation, operation string) (any, error) {
	if cx.Flag("aspect") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid,
			"invoke calls a ResourceDescriptor operation; use kc access --aspect for Binding hydration")
	}
	rawInput, err := cx.Require("input")
	if err != nil {
		return nil, err
	}
	var input any
	decoder := json.NewDecoder(strings.NewReader(rawInput))
	decoder.UseNumber()
	if err := decoder.Decode(&input); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must be one JSON value: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--input must contain exactly one JSON value")
	}

	descriptor, err := PinnedResourceDescriptor(cx)
	if err != nil {
		return nil, err
	}
	value, ok := descriptor.Value.(map[string]any)
	if !ok || StringValue(value["kind"]) != "ResourceDescriptor" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "object %s is not a ResourceDescriptor", descriptor.ObjectID)
	}
	runtime := strings.TrimSpace(StringValue(value["runtime"]))
	protocol := strings.TrimSpace(StringValue(value["protocol"]))
	origin := strings.TrimSpace(StringValue(value["origin"]))
	if origin == "" {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare origin", descriptor.ObjectID)
	}
	access, ok := value["access"].(map[string]any)
	if !ok || runtime == "" || protocol == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s requires runtime, protocol and access", descriptor.ObjectID)
	}
	operationValue, ok := access[operation].(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "ResourceDescriptor %s does not declare operation %s", descriptor.ObjectID, operation)
	}
	call := strings.TrimSpace(StringValue(operationValue["call"]))
	if call == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "ResourceDescriptor %s operation %s requires call", descriptor.ObjectID, operation)
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
	return accessor.AccessResource(cx.Context, ResourceOperationRequest{
		Descriptor: ResourceDescriptorCoordinate{
			ObjectID: descriptor.ObjectID, Repository: descriptor.Repository, Commit: descriptor.Commit,
		},
		Runtime: runtime, Protocol: protocol, Origin: origin, Operation: operation, Call: call, Input: input,
		Identity: StateRuntimeIdentity{
			Principal: identity.Principal, OnBehalfOf: identity.OnBehalfOf, RequestID: requestID,
			TraceID: trace.TraceID, SpanID: trace.SpanID, ParentSpanID: trace.ParentSpanID,
		},
	})
}

func PinnedResourceDescriptor(cx *Invocation) (PinnedDescriptor, error) {
	result, err := VerbRead(cx)
	if err != nil {
		return PinnedDescriptor{}, err
	}
	switch value := result.(type) {
	case []knowledgeserving.ReadResult:
		if len(value) != 1 {
			return PinnedDescriptor{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved,
				"ResourceDescriptor %s resolved to %d values", cx.Flag("object"), len(value))
		}
		return PinnedDescriptor{
			Repository: value[0].Repository, Commit: value[0].Commit,
			ObjectID: value[0].ObjectID, Value: value[0].Value,
		}, nil
	case knowledge.KnowledgeValue:
		return PinnedDescriptor{
			Repository: value.Repository, Commit: value.Commit,
			ObjectID: value.Address.ObjectID, Value: value.Value,
		}, nil
	default:
		return PinnedDescriptor{}, fmt.Errorf("ResourceDescriptor read returned %T", result)
	}
}

type StateRuntimeIdentity struct {
	Principal    string `json:"principal"`
	OnBehalfOf   string `json:"onBehalfOf,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}
type ResourceDescriptorCoordinate struct {
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}
type ResourceOperationRequest struct {
	Descriptor ResourceDescriptorCoordinate `json:"descriptor"`
	Runtime    string                       `json:"runtime"`
	Protocol   string                       `json:"protocol"`
	Origin     string                       `json:"-"`
	Operation  string                       `json:"operation"`
	Call       string                       `json:"call"`
	Input      any                          `json:"input"`
	Identity   StateRuntimeIdentity         `json:"identity"`
}
type ResourceOperationAccessor interface {
	AccessResource(context.Context, ResourceOperationRequest) (any, error)
}

type PinnedDescriptor struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	ObjectID   knowledge.ObjectID
	Value      any
}

func HydrateSearchHit(ctx context.Context, logical *knowledgeserving.Service, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	return knowledgeapp.HydrateSearchHit(ctx, logical, hit)
}
