package observability

import (
	"encoding/json"
	"fmt"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

const (
	RetrievalOperatorSearch   = "SEARCH"
	RetrievalOperatorRelation = "RELATION"
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
	KnowledgeRef knowledge.PinnedKnowledgeRef `json:"knowledgeRef"`
	Rank         int                          `json:"rank"`
	ValueDigest  kernel.Digest                `json:"valueDigest,omitempty"`
	Evidence     []RetrievalLaneEvidence      `json:"evidence,omitempty"`
	Observations []knowledge.UnitObservation  `json:"observations,omitempty"`
	MatchedRoles []string                     `json:"matchedRoles,omitempty"`
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
	Workspace        string               `json:"workspace,omitempty"`
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
	if e.Operator != RetrievalOperatorSearch && e.Operator != RetrievalOperatorRelation {
		return fmt.Errorf("retrieval evidence requires a supported operator")
	}
	if e.LogicalRequest == nil {
		return fmt.Errorf("retrieval evidence requires logicalRequest")
	}
	if _, err := json.Marshal(e.LogicalRequest); err != nil || e.RequestDigest != kernel.CanonicalDigest(e.LogicalRequest) {
		return fmt.Errorf("retrieval evidence requires canonical request digest")
	}
	seen := map[knowledge.KnowledgeRef]struct{}{}
	for i, candidate := range e.Candidates {
		if candidate.Rank != i+1 || candidate.KnowledgeRef.Repository == "" || candidate.KnowledgeRef.Commit == "" || candidate.KnowledgeRef.Object == "" {
			return fmt.Errorf("retrieval candidate requires rank and pinned KnowledgeRef")
		}
		if _, duplicate := seen[candidate.KnowledgeRef.KnowledgeRef]; duplicate {
			return fmt.Errorf("retrieval candidate ref is duplicated")
		}
		seen[candidate.KnowledgeRef.KnowledgeRef] = struct{}{}
		basis, member := e.SearchView.Snapshots[candidate.KnowledgeRef.Repository]
		if !member || basis != candidate.KnowledgeRef.Commit {
			return fmt.Errorf("retrieval candidate is outside SearchView")
		}
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

type RetrievalQuery struct {
	EvidenceID string
	TraceID    string
	Operator   string
	Provider   string
	Outcome    string
	Limit      int
}
