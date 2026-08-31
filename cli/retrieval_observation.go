package cli

import (
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
	"kc/retrieval"
)

type relationObservationRequest struct {
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func recordRetrievalEvidence(home, command string, flags map[string]FlagValue, result any, accessEvidenceID string, callErr error) (string, error) {
	if accessEvidenceID == "" || (command != "search" && command != "search-rerank" && command != "relations") {
		return "", nil
	}
	event, err := retrievalEventFrom(command, flags, result, accessEvidenceID, callErr)
	if err != nil {
		return "", err
	}
	return observability.NewFileStore(home).RecordRetrievalReceipt(event)
}

func retrievalEventFrom(command string, flags map[string]FlagValue, result any, accessEvidenceID string, callErr error) (observability.RetrievalEvent, error) {
	identity, err := identityContextFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	trace, err := traceContextFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	requestID, err := requestIDFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	event := observability.RetrievalEvent{
		AccessEvidenceID: accessEvidenceID, Identity: identity, Trace: trace,
		Action: actionOf(command, flags), RequestID: requestID, Workspace: workspaceIDOf(flags),
		Outcome: "COMPLETED", Candidates: []observability.RetrievalCandidate{}, Claims: []string{},
		SearchView: observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
	}
	if command == "relations" {
		event.Operator = observability.RetrievalOperatorRelation
		limit, limitErr := limitFrom(flags, 0)
		if limitErr != nil {
			return observability.RetrievalEvent{}, limitErr
		}
		event.HadContinuation = strings.TrimSpace(FlagString(flags, "continuation")) != ""
		event.LogicalRequest = relationObservationRequest{
			Endpoint: FlagString(flags, "object"), RelationType: FlagString(flags, "relation-type"),
			Role: FlagString(flags, "role"), Direction: strings.ToUpper(FlagString(flags, "direction")), Limit: limit,
		}
	} else {
		event.Operator = observability.RetrievalOperatorSearch
		request, requestErr := searchRequestFromFlags(flags)
		if requestErr != nil {
			return observability.RetrievalEvent{}, requestErr
		}
		event.HadContinuation = request.Continuation != ""
		request.Continuation = ""
		event.LogicalRequest = request
	}
	event.RequestDigest = kernel.CanonicalDigest(event.LogicalRequest)

	source := retrievalObservationSource(result)
	retrievalErr := callErr
	// search:rerank is a two-stage operation. Once a SearchResult exists, the
	// retrieval stage completed even if the downstream semantic refine failed.
	if command == "search-rerank" {
		if _, ok := source.(retrieval.SearchResult); ok {
			retrievalErr = nil
		}
		if _, ok := source.(searchRerankResult); ok {
			retrievalErr = nil
		}
	}
	switch typed := source.(type) {
	case retrieval.SearchResult:
		populateSearchRetrievalEvent(&event, typed)
	case retrieval.RelationPage:
		populateRelationRetrievalEvent(&event, typed)
	case searchRerankResult:
		populateSearchRetrievalEvent(&event, typed.Retrieval)
	}
	if retrievalErr != nil {
		event.Outcome = "ERROR"
		event.Error = journal.ErrorOf(retrievalErr)
	}
	event.CandidateDigest = kernel.CanonicalDigest(event.Candidates)
	return event, nil
}

func retrievalObservationSource(result any) any {
	if observed, ok := result.(observedAccessResult); ok {
		if observed.RetrievalSource != nil {
			return observed.RetrievalSource
		}
		return observed.Output
	}
	return result
}

func populateSearchRetrievalEvent(event *observability.RetrievalEvent, result retrieval.SearchResult) {
	event.SearchView = observability.RefineSearchView{
		Snapshots: result.SearchView.Snapshots, ProjectionRevisions: result.SearchView.ProjectionRevisions,
	}
	event.HasMore = result.Continuation != ""
	event.Completeness = string(result.Completeness)
	event.Claims = append([]string(nil), result.Claims...)
	event.Execution = observability.RetrievalExecution{
		Candidates: result.Stats.Candidates, Hydrated: result.Stats.Hydrated, Dropped: result.Stats.Dropped,
		DroppedAuthorization: result.Stats.DroppedAuthorization,
		PlanMillis:           result.Stats.PlanDuration.Milliseconds(), ProbeMillis: result.Stats.ProbeDuration.Milliseconds(),
		HydrateMillis: result.Stats.HydrateDuration.Milliseconds(),
	}
	for i, hit := range result.Hits {
		ref := knowledge.PinnedKnowledgeRef{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: hit.Knowledge.Repository, Object: hit.Knowledge.Address.ObjectID},
			Commit:       hit.Knowledge.Commit,
		}
		event.Candidates = append(event.Candidates, observability.RetrievalCandidate{
			KnowledgeRef: ref, Rank: i + 1, ValueDigest: kernel.CanonicalDigest(hit.Knowledge.Value),
			Evidence: retrievalLaneEvidence(hit.Evidence), Observations: append([]knowledge.UnitObservation(nil), hit.Version.Observations...),
		})
	}
}

func populateRelationRetrievalEvent(event *observability.RetrievalEvent, result retrieval.RelationPage) {
	event.SearchView = observability.RefineSearchView{
		Snapshots: result.SearchView.Snapshots, ProjectionRevisions: result.SearchView.ProjectionRevisions,
	}
	event.HasMore = result.Continuation != ""
	event.Completeness = "complete"
	event.Claims = append([]string(nil), result.Claims...)
	event.Execution = observability.RetrievalExecution{Candidates: len(result.Hits), Hydrated: len(result.Hits)}
	for i, hit := range result.Hits {
		event.Candidates = append(event.Candidates, observability.RetrievalCandidate{
			KnowledgeRef: knowledge.PinnedKnowledgeRef{KnowledgeRef: hit.KnowledgeRef, Commit: hit.Commit}, Rank: i + 1,
			ValueDigest: kernel.CanonicalDigest(hit.Relation), Evidence: retrievalLaneEvidence(hit.Evidence),
			MatchedRoles: append([]string(nil), hit.MatchedRoles...),
		})
	}
}

func retrievalLaneEvidence(lanes []retrieval.LaneEvidence) []observability.RetrievalLaneEvidence {
	out := make([]observability.RetrievalLaneEvidence, len(lanes))
	for i, lane := range lanes {
		fields := make([]observability.RefineFieldRef, len(lane.MatchedFields))
		for j, field := range lane.MatchedFields {
			fields[j] = observability.RefineFieldRef{Schema: field.Schema, Aspect: field.Aspect, Path: field.Path}
		}
		out[i] = observability.RetrievalLaneEvidence{
			Provider: lane.Provider, Lane: lane.Lane, Guarantee: lane.Guarantee,
			LocalRank: lane.LocalRank, LocalScore: lane.LocalScore, MatchedFields: fields,
		}
	}
	return out
}

func attachRetrievalEvidenceID(result any, evidenceID string) any {
	if evidenceID == "" {
		return result
	}
	if observed, ok := result.(observedAccessResult); ok {
		observed.Output = attachRetrievalEvidenceID(observed.Output, evidenceID)
		return observed
	}
	switch output := result.(type) {
	case retrieval.SearchResult:
		output.RetrievalEvidenceID = evidenceID
		return output
	case retrieval.RelationPage:
		output.RetrievalEvidenceID = evidenceID
		return output
	case searchRerankResult:
		output.Retrieval.RetrievalEvidenceID = evidenceID
		return output
	default:
		return result
	}
}

func withRetrievalObservationSource(result any, source any) any {
	if observed, ok := result.(observedAccessResult); ok {
		observed.RetrievalSource = source
		return observed
	}
	return result
}
