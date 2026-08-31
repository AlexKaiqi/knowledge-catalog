package observability

import (
	"fmt"
	"strings"

	"kc/knowledge"
)

type FeedbackEvent struct {
	SchemaVersion       int                      `json:"schemaVersion,omitempty"`
	OccurredAt          string                   `json:"occurredAt"`
	Identity            IdentityContext          `json:"identity"`
	Trace               TraceContext             `json:"trace"`
	SubmissionTrace     *TraceContext            `json:"submissionTrace,omitempty"`
	Workspace           string                   `json:"workspace,omitempty"`
	Outcome             string                   `json:"outcome"`
	Message             string                   `json:"message,omitempty"`
	RetrievalEvidenceID string                   `json:"retrievalEvidenceId,omitempty"`
	RefineEvidenceID    string                   `json:"refineEvidenceId,omitempty"`
	LabelSource         string                   `json:"labelSource,omitempty"`
	Answer              string                   `json:"answer,omitempty"`
	SelectedRefs        []knowledge.KnowledgeRef `json:"selectedRefs,omitempty"`
	IdealGroups         []RefineRankGroup        `json:"idealGroups,omitempty"`
}

func (e FeedbackEvent) Validate() error {
	if e.SchemaVersion != 0 && e.SchemaVersion != 1 {
		return fmt.Errorf("unsupported feedback schemaVersion")
	}
	if err := e.Identity.Validate(); err != nil {
		return err
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	if e.SubmissionTrace != nil {
		if err := e.SubmissionTrace.Validate(); err != nil {
			return err
		}
	}
	if e.Trace.TraceID == "" {
		return fmt.Errorf("feedback requires traceId")
	}
	if strings.TrimSpace(e.Outcome) == "" {
		return fmt.Errorf("feedback outcome is required")
	}
	structured := e.Answer != "" || len(e.SelectedRefs) > 0 || len(e.IdealGroups) > 0
	if structured && e.RefineEvidenceID == "" && e.RetrievalEvidenceID == "" {
		return fmt.Errorf("structured retrieval feedback requires retrievalEvidenceId or refineEvidenceId")
	}
	if (e.RefineEvidenceID != "" || e.RetrievalEvidenceID != "") && e.LabelSource != "agent" && e.LabelSource != "user" && e.LabelSource != "human-review" {
		return fmt.Errorf("retrieval feedback requires a declared labelSource")
	}
	if len(e.Answer) > 32<<10 {
		return fmt.Errorf("feedback answer is too long")
	}
	seen := map[knowledge.KnowledgeRef]struct{}{}
	for _, ref := range e.SelectedRefs {
		if ref.Repository == "" || ref.Object == "" {
			return fmt.Errorf("selectedRefs require repository and object")
		}
		if _, duplicate := seen[ref]; duplicate {
			return fmt.Errorf("selectedRefs contain a duplicate")
		}
		seen[ref] = struct{}{}
	}
	seen = map[knowledge.KnowledgeRef]struct{}{}
	for i, group := range e.IdealGroups {
		if group.Rank != i+1 || len(group.Refs) == 0 {
			return fmt.Errorf("idealGroups require consecutive non-empty ranks")
		}
		for _, ref := range group.Refs {
			if ref.Repository == "" || ref.Object == "" {
				return fmt.Errorf("idealGroups require repository and object")
			}
			if _, duplicate := seen[ref]; duplicate {
				return fmt.Errorf("idealGroups contain a duplicate ref")
			}
			seen[ref] = struct{}{}
		}
	}
	return nil
}
