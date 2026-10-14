package invocation

import (
	"strconv"
	"strings"

	retrieval "kc/discovery"
	"kc/internal/journal"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
)

type traverseObservationRequest struct {
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	MinHops      int    `json:"minHops"`
	MaxHops      int    `json:"maxHops"`
	Limit        int    `json:"limit,omitempty"`
}

type relationObservationRequest struct {
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func RecordRetrievalEvidence(home, command string, flags map[string]invoke.FlagValue, result any, accessEvidenceID string, callErr error) (string, int64, error) {
	switch command {
	case "knowledge-search", "search-rerank", "knowledge-relations", "knowledge-traverse":
	default:
		return "", -1, nil
	}
	if accessEvidenceID == "" {
		return "", -1, nil
	}
	event, err := RetrievalEventFrom(command, flags, result, accessEvidenceID, callErr)
	if err != nil {
		return "", -1, err
	}
	store := observability.NewFileStore(home)
	id, recErr := store.RecordRetrievalReceipt(event)
	return id, store.LastAppendBytes(), recErr
}

func RetrievalEventFrom(command string, flags map[string]invoke.FlagValue, result any, accessEvidenceID string, callErr error) (observability.RetrievalEvent, error) {
	identity, err := IdentityContextFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	trace, err := TraceContextFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	requestID, err := RequestIDFrom(flags)
	if err != nil {
		return observability.RetrievalEvent{}, err
	}
	event := observability.RetrievalEvent{
		AccessEvidenceID: accessEvidenceID, Identity: identity, Trace: trace,
		Action: ActionOf(command, flags), RequestID: requestID, Dataset: invoke.DatasetOf(flags),
		Outcome: "COMPLETED", Candidates: []observability.RetrievalCandidate{}, Claims: []string{},
		SearchView: observability.RefineSearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
	}
	if command == "knowledge-traverse" {
		event.Operator = observability.RetrievalOperatorTraverse
		maxHops, maxErr := TraverseHops(flags)
		if maxErr != nil {
			return observability.RetrievalEvent{}, maxErr
		}
		minHops := 0
		if raw := strings.TrimSpace(invoke.FlagString(flags, "min-hops")); raw != "" {
			parsed, parseErr := strconv.Atoi(raw)
			if parseErr != nil || parsed < 0 {
				return observability.RetrievalEvent{}, kernel.Fail(kernel.ErrUsageInvalid, "--min-hops must be a non-negative number")
			}
			minHops = parsed
		}
		limit, limitErr := LimitFrom(flags, retrieval.DefaultTraversePageLimit)
		if limitErr != nil {
			return observability.RetrievalEvent{}, limitErr
		}
		event.HadContinuation = strings.TrimSpace(invoke.FlagString(flags, "continuation")) != ""
		event.LogicalRequest = traverseObservationRequest{
			Endpoint: invoke.FlagString(flags, "object"), RelationType: invoke.FlagString(flags, "relation-type"),
			Role: invoke.FlagString(flags, "role"), Direction: strings.ToUpper(invoke.FlagString(flags, "direction")),
			MinHops: minHops, MaxHops: maxHops, Limit: limit,
		}
	} else if command == "knowledge-relations" {
		event.Operator = observability.RetrievalOperatorRelation
		limit, limitErr := LimitFrom(flags, 0)
		if limitErr != nil {
			return observability.RetrievalEvent{}, limitErr
		}
		event.HadContinuation = strings.TrimSpace(invoke.FlagString(flags, "continuation")) != ""
		event.LogicalRequest = relationObservationRequest{
			Endpoint: invoke.FlagString(flags, "object"), RelationType: invoke.FlagString(flags, "relation-type"),
			Role: invoke.FlagString(flags, "role"), Direction: strings.ToUpper(invoke.FlagString(flags, "direction")), Limit: limit,
		}
	} else {
		event.Operator = observability.RetrievalOperatorSearch
		request, requestErr := SearchRequestFromFlags(flags)
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
		if _, ok := source.(SearchRerankResult); ok {
			retrievalErr = nil
		}
	}
	switch typed := source.(type) {
	case retrieval.SearchResult:
		populateSearchRetrievalEvent(&event, typed)
	case retrieval.RelationPage:
		populateRelationRetrievalEvent(&event, typed)
	case retrieval.TraversePage:
		populateTraverseRetrievalEvent(&event, typed)
	case SearchRerankResult:
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
	if observed, ok := result.(ObservedAccessResult); ok {
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
		ref := objectmodel.PinnedKnowledgeRef{
			KnowledgeRef: objectmodel.KnowledgeRef{Repository: hit.Knowledge.Repository, Object: hit.Knowledge.Address.ObjectID},
			Commit:       hit.Knowledge.Commit,
		}
		event.Candidates = append(event.Candidates, observability.RetrievalCandidate{
			KnowledgeRef: ref, Rank: i + 1, ValueDigest: kernel.CanonicalDigest(hit.Knowledge.Value),
			Evidence: retrievalLaneEvidence(hit.Evidence), Observations: append([]objectmodel.UnitObservation(nil), hit.Version.Observations...),
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
			KnowledgeRef: objectmodel.PinnedKnowledgeRef{KnowledgeRef: hit.KnowledgeRef, Commit: hit.Commit}, Rank: i + 1,
			ValueDigest: kernel.CanonicalDigest(hit.Relation), Evidence: retrievalLaneEvidence(hit.Evidence),
			MatchedRoles: append([]string(nil), hit.MatchedRoles...),
		})
	}
}

// populateTraverseRetrievalEvent records the closure delta as the candidate
// window in delivery order: reached node identities first (the delivered
// record is repository/object/depth, so that tuple is the value digest),
// then the selected relation bodies with their own storage commits. The
// start object is never a node, so it never appears as a candidate.
// Completeness follows the page contract: an exhausted closure page is
// complete, a budget-paused page is partial.
func populateTraverseRetrievalEvent(event *observability.RetrievalEvent, result retrieval.TraversePage) {
	event.SearchView = observability.RefineSearchView{
		Snapshots: result.SearchView.Snapshots, ProjectionRevisions: result.SearchView.ProjectionRevisions,
	}
	event.HasMore = result.Continuation != ""
	if result.Exhausted {
		event.Completeness = "complete"
	} else {
		event.Completeness = "partial"
	}
	event.Claims = append([]string(nil), result.Claims...)
	event.Execution = observability.RetrievalExecution{Candidates: len(result.Nodes) + len(result.Edges)}
	rank := 0
	for _, node := range result.Nodes {
		rank++
		event.Candidates = append(event.Candidates, observability.RetrievalCandidate{
			KnowledgeRef: objectmodel.PinnedKnowledgeRef{
				KnowledgeRef: objectmodel.KnowledgeRef{Repository: node.Repository, Object: node.ObjectID},
				Commit:       result.SearchView.Snapshots[node.Repository],
			},
			Rank:        rank,
			ValueDigest: kernel.CanonicalDigest(node),
		})
	}
	for _, edge := range result.Edges {
		rank++
		event.Candidates = append(event.Candidates, observability.RetrievalCandidate{
			KnowledgeRef: objectmodel.PinnedKnowledgeRef{
				KnowledgeRef: objectmodel.KnowledgeRef{Repository: edge.Repository, Object: edge.ObjectID},
				Commit:       edge.Commit,
			},
			Rank:        rank,
			ValueDigest: kernel.CanonicalDigest(edge.Relation),
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
	if observed, ok := result.(ObservedAccessResult); ok {
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
	case SearchRerankResult:
		output.Retrieval.RetrievalEvidenceID = evidenceID
		return output
	default:
		return result
	}
}

func WithRetrievalObservationSource(result any, source any) any {
	if observed, ok := result.(ObservedAccessResult); ok {
		observed.RetrievalSource = source
		return observed
	}
	return result
}
