package observability

import (
	"encoding/json"
	"fmt"
	"strings"

	"kc/kernel"
	"kc/knowledge"
)

type RefineSearchView struct {
	Snapshots           map[kernel.RepositoryID]kernel.CommitID `json:"snapshots"`
	ProjectionRevisions map[kernel.RepositoryID]string          `json:"projectionRevisions,omitempty"`
}

type RefineSpec struct {
	SpecRef          string   `json:"specRef"`
	Revision         int      `json:"revision"`
	Operator         string   `json:"operator"`
	Criterion        string   `json:"criterion"`
	EvaluationFields []string `json:"evaluationFields,omitempty"`
	TopK             *int     `json:"topK,omitempty"`
	AllowTies        bool     `json:"allowTies"`
	AllowUnjudged    bool     `json:"allowUnjudged"`
}

type RefineFieldRef struct {
	Schema knowledge.ObjectID `json:"schema"`
	Aspect string             `json:"aspect,omitempty"`
	Path   string             `json:"path"`
}

type RefineLaneEvidence = RetrievalLaneEvidence

type RefineCandidate struct {
	KnowledgeRef      knowledge.PinnedKnowledgeRef `json:"knowledgeRef"`
	Value             any                          `json:"value"`
	ValueDigest       kernel.Digest                `json:"valueDigest"`
	OriginalRank      int                          `json:"originalRank"`
	RetrievalEvidence []RefineLaneEvidence         `json:"retrievalEvidence,omitempty"`
	Observations      []knowledge.UnitObservation  `json:"observations,omitempty"`
}

type RefineRankGroup struct {
	Rank int                      `json:"rank"`
	Refs []knowledge.KnowledgeRef `json:"refs"`
}

type RefineModelOutput struct {
	Provider       string                   `json:"provider"`
	Model          string                   `json:"model"`
	ModelRevision  string                   `json:"modelRevision,omitempty"`
	PromptRevision string                   `json:"promptRevision,omitempty"`
	DurationMillis int64                    `json:"durationMillis"`
	Groups         []RefineRankGroup        `json:"groups"`
	Unjudged       []knowledge.KnowledgeRef `json:"unjudged"`
}

// RefineEvent is non-Canonical semantic inference evidence. Candidate Value is
// exactly the EvaluationProjection visible to the model; unprojected knowledge,
// credentials, provider transport bodies and chain-of-thought are forbidden.
type RefineEvent struct {
	SchemaVersion       int                `json:"schemaVersion"`
	EvidenceID          string             `json:"evidenceId,omitempty"`
	AccessEvidenceID    string             `json:"accessEvidenceId"`
	RetrievalEvidenceID string             `json:"retrievalEvidenceId,omitempty"`
	OccurredAt          string             `json:"occurredAt"`
	Identity            IdentityContext    `json:"identity"`
	Trace               TraceContext       `json:"trace,omitempty"`
	Action              string             `json:"action"`
	RequestID           string             `json:"requestId,omitempty"`
	Workspace           string             `json:"workspace"`
	SearchView          RefineSearchView   `json:"searchView"`
	RetrievalQuery      any                `json:"retrievalQuery,omitempty"`
	Spec                RefineSpec         `json:"spec"`
	CandidateDigest     kernel.Digest      `json:"candidateDigest"`
	ProjectedBytes      int                `json:"projectedBytes"`
	Candidates          []RefineCandidate  `json:"candidates"`
	Outcome             string             `json:"outcome"`
	ModelOutput         *RefineModelOutput `json:"modelOutput,omitempty"`
	Error               map[string]any     `json:"error,omitempty"`
}

func (e RefineEvent) Validate() error {
	if e.SchemaVersion != 1 {
		return fmt.Errorf("unsupported refine evidence schemaVersion")
	}
	if e.EvidenceID == "" || !strings.HasPrefix(e.EvidenceID, "rf_") {
		return fmt.Errorf("refine evidence requires recorder-managed evidenceId")
	}
	if e.AccessEvidenceID == "" || e.Identity.Validate() != nil {
		return fmt.Errorf("refine evidence requires access evidence and valid identity")
	}
	if e.RetrievalEvidenceID != "" && !strings.HasPrefix(e.RetrievalEvidenceID, "rt_") {
		return fmt.Errorf("refine evidence retrievalEvidenceId is invalid")
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	if e.Workspace == "" || e.Spec.SpecRef == "" || e.Spec.Revision <= 0 || strings.TrimSpace(e.Spec.Criterion) == "" {
		return fmt.Errorf("refine evidence requires workspace and frozen semantic spec")
	}
	if e.CandidateDigest == "" || e.ProjectedBytes <= 0 || len(e.Candidates) == 0 {
		return fmt.Errorf("refine evidence requires projected candidate input")
	}
	seen := map[knowledge.KnowledgeRef]struct{}{}
	for i, candidate := range e.Candidates {
		ref := candidate.KnowledgeRef
		if ref.Repository == "" || ref.Commit == "" || ref.Object == "" || candidate.OriginalRank != i+1 {
			return fmt.Errorf("refine candidate requires pinned ref and window rank")
		}
		if _, duplicate := seen[ref.KnowledgeRef]; duplicate {
			return fmt.Errorf("refine candidate ref is duplicated")
		}
		seen[ref.KnowledgeRef] = struct{}{}
		if _, err := json.Marshal(candidate.Value); err != nil || candidate.ValueDigest != kernel.CanonicalDigest(candidate.Value) {
			return fmt.Errorf("refine candidate requires canonical projected value digest")
		}
	}
	if e.Outcome != "COMPLETED" && e.Outcome != "ERROR" {
		return fmt.Errorf("refine outcome must be COMPLETED or ERROR")
	}
	if e.Outcome == "COMPLETED" && (e.ModelOutput == nil || e.ModelOutput.Provider == "" || e.ModelOutput.Model == "") {
		return fmt.Errorf("completed refine evidence requires model output")
	}
	if e.Outcome == "ERROR" && len(e.Error) == 0 {
		return fmt.Errorf("failed refine evidence requires error")
	}
	return nil
}

type RefineQuery struct {
	EvidenceID string
	TraceID    string
	Provider   string
	Model      string
	Outcome    string
	Limit      int
}
