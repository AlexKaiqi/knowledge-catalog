package observability

import (
	"encoding/json"
	"fmt"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

const (
	RetrievalOperatorSearch   = "SEARCH"
	RetrievalOperatorRelation = "RELATION"
	RetrievalOperatorTraverse = "TRAVERSE"
)

type RetrievalLaneEvidence struct {
	Provider      string           `json:"provider"`
	Lane          string           `json:"lane"`
	Guarantee     string           `json:"guarantee"`
	LocalRank     int              `json:"localRank,omitempty"`
	LocalScore    float64          `json:"localScore,omitempty"`
	MatchedFields []RefineFieldRef `json:"matchedFields,omitempty"`
}

type RetrievalCandidate struct {
	ObjectRef    objectmodel.PinnedObjectRef   `json:"objectRef"`
	Sources      []objectmodel.SourceBasis     `json:"sources,omitempty"`
	Rank         int                           `json:"rank"`
	ValueDigest  kernel.Digest                 `json:"valueDigest,omitempty"`
	Evidence     []RetrievalLaneEvidence       `json:"evidence,omitempty"`
	Observations []objectmodel.UnitObservation `json:"observations,omitempty"`
	MatchedRoles []string                      `json:"matchedRoles,omitempty"`
}

type RetrievalExecution struct {
	Candidates           int   `json:"candidates"`
	Hydrated             int   `json:"hydrated"`
	Dropped              int   `json:"dropped"`
	DroppedAuthorization int   `json:"droppedAuthorization"`
	PlanMillis           int64 `json:"planMillis"`
	ProbeMillis          int64 `json:"probeMillis"`
	HydrateMillis        int64 `json:"hydrateMillis"`
}

type RetrievalEvent struct {
	SchemaVersion    int                  `json:"schemaVersion"`
	EvidenceID       string               `json:"evidenceId,omitempty"`
	AccessEvidenceID string               `json:"accessEvidenceId"`
	OccurredAt       string               `json:"occurredAt"`
	Identity         IdentityContext      `json:"identity"`
	Trace            TraceContext         `json:"trace,omitempty"`
	Action           string               `json:"action"`
	RequestID        string               `json:"requestId,omitempty"`
	Dataset          string               `json:"dataset,omitempty"`
	Operator         string               `json:"operator"`
	LogicalRequest   any                  `json:"logicalRequest"`
	RequestDigest    kernel.Digest        `json:"requestDigest"`
	SearchView       RefineSearchView     `json:"searchView"`
	HadContinuation  bool                 `json:"hadContinuation"`
	HasMore          bool                 `json:"hasMore"`
	Completeness     string               `json:"completeness,omitempty"`
	Claims           []string             `json:"claims,omitempty"`
	Execution        RetrievalExecution   `json:"execution"`
	CandidateDigest  kernel.Digest        `json:"candidateDigest"`
	Candidates       []RetrievalCandidate `json:"candidates"`
	Outcome          string               `json:"outcome"`
	Error            map[string]any       `json:"error,omitempty"`
}

func (e RetrievalEvent) Validate() error {
	if e.SchemaVersion != 1 {
		return fmt.Errorf("unsupported retrieval evidence schemaVersion")
	}
	if e.EvidenceID == "" || !strings.HasPrefix(e.EvidenceID, "rt_") {
		return fmt.Errorf("retrieval evidence requires recorder-managed evidenceId")
	}
	if e.AccessEvidenceID == "" {
		return fmt.Errorf("retrieval evidence requires accessEvidenceId")
	}
	if err := e.Identity.Validate(); err != nil {
		return err
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	switch e.Operator {
	case RetrievalOperatorSearch, RetrievalOperatorRelation, RetrievalOperatorTraverse:
	default:
		return fmt.Errorf("retrieval evidence requires a supported operator")
	}
	if e.LogicalRequest == nil {
		return fmt.Errorf("retrieval evidence requires logicalRequest")
	}
	if _, err := json.Marshal(e.LogicalRequest); err != nil || e.RequestDigest != kernel.CanonicalDigest(e.LogicalRequest) {
		return fmt.Errorf("retrieval evidence requires canonical request digest")
	}
	seen := map[objectmodel.ObjectRef]struct{}{}
	for i, candidate := range e.Candidates {
		if candidate.Rank != i+1 {
			return fmt.Errorf("retrieval candidate requires rank and pinned ObjectRef")
		}
		if err := validateRetrievalCandidateBasis(candidate, e.Dataset, e.SearchView); err != nil {
			return err
		}
		if _, duplicate := seen[candidate.ObjectRef.ObjectRef]; duplicate {
			return fmt.Errorf("retrieval candidate ref is duplicated")
		}
		seen[candidate.ObjectRef.ObjectRef] = struct{}{}
		if candidate.ValueDigest == "" {
			return fmt.Errorf("retrieval candidate requires canonical value digest")
		}
		for _, lane := range candidate.Evidence {
			if strings.TrimSpace(lane.Provider) == "" || strings.TrimSpace(lane.Lane) == "" {
				return fmt.Errorf("retrieval lane requires provider and lane")
			}
		}
	}
	if e.CandidateDigest != kernel.CanonicalDigest(e.Candidates) {
		return fmt.Errorf("retrieval evidence requires canonical candidate digest")
	}
	if e.Outcome != "COMPLETED" && e.Outcome != "ERROR" {
		return fmt.Errorf("retrieval outcome must be COMPLETED or ERROR")
	}
	if e.Outcome == "COMPLETED" && len(e.SearchView.Snapshots) == 0 {
		return fmt.Errorf("completed retrieval evidence requires fixed SearchView")
	}
	if e.Outcome == "ERROR" && len(e.Error) == 0 {
		return fmt.Errorf("failed retrieval evidence requires error")
	}
	return nil
}

func validateRetrievalCandidateBasis(candidate RetrievalCandidate, dataset string, view RefineSearchView) error {
	ref := candidate.ObjectRef
	if ref.Object == "" {
		return fmt.Errorf("retrieval candidate requires an object identity")
	}
	if len(candidate.Sources) == 0 {
		if ref.Repository == "" || ref.Commit == "" {
			return fmt.Errorf("retrieval candidate requires pinned ObjectRef")
		}
		if basis, member := view.Snapshots[ref.Repository]; !member || basis != ref.Commit {
			return fmt.Errorf("retrieval candidate is outside SearchView")
		}
		return nil
	}
	if strings.TrimSpace(dataset) == "" || ref.Repository != "" || ref.Commit != "" || len(candidate.Sources) < 2 {
		return fmt.Errorf("composed retrieval candidate requires a Dataset, object identity and source vector without single-source coordinates")
	}
	return validateRetrievalSources(candidate.Sources, view)
}

func validateRetrievalSources(sources []objectmodel.SourceBasis, view RefineSearchView) error {
	repositories := map[kernel.RepositoryID]bool{}
	aspects := map[string]bool{}
	for _, source := range sources {
		if strings.TrimSpace(string(source.Repository)) == "" || strings.TrimSpace(string(source.Commit)) == "" || repositories[source.Repository] {
			return fmt.Errorf("composed retrieval candidate requires unique pinned sources")
		}
		if basis, member := view.Snapshots[source.Repository]; !member || basis != source.Commit {
			return fmt.Errorf("composed retrieval candidate source is outside SearchView")
		}
		if len(source.Aspects) == 0 {
			return fmt.Errorf("composed retrieval candidate source requires Aspects")
		}
		repositories[source.Repository] = true
		for _, aspect := range source.Aspects {
			if strings.TrimSpace(aspect) == "" || aspects[aspect] {
				return fmt.Errorf("composed retrieval candidate requires disjoint nonempty Aspects")
			}
			aspects[aspect] = true
		}
	}
	return nil
}

type RetrievalQuery struct {
	EvidenceID string
	TraceID    string
	Operator   string
	Provider   string
	Outcome    string
	Limit      int
}
