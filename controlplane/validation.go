package controlplane

import (
	"kc/catalog"
	"kc/kernel"
)

type ValidationReport struct {
	ReportID      string `json:"reportId"`
	PreviewID     string `json:"previewId"`
	SuiteRevision string `json:"suiteRevision"`
	Outcome       string `json:"outcome"`
}

type StructureReport struct {
	ValidationReport
	Check catalog.WorkspaceCheck `json:"check"`
}

func (cp *ControlPlane) ValidateStructure(preview Preview) (StructureReport, error) {
	check := cp.catalog.CheckResolved(catalog.ResolvedWorkspace{
		WorkspaceID:  preview.WorkspaceID,
		Repositories: preview.Repositories,
	})
	issues := append([]catalog.WorkspaceIssue{}, check.Issues...)
	repo, ok := cp.store.Get(preview.Candidate.RepositoryID)
	if !ok || !repo.HasCommit(preview.Candidate.CommitID) {
		issues = append(issues, catalog.WorkspaceIssue{
			Repository: preview.Candidate.RepositoryID,
			Code:       kernel.ErrVersionUnresolved,
			Message:    "candidate commit " + string(preview.Candidate.CommitID) + " does not exist",
		})
	}
	outcome := "PASSED"
	if len(issues) > 0 {
		outcome = "FAILED"
	}
	report, err := cp.RecordValidation(preview, "structure", outcome)
	if err != nil {
		return StructureReport{}, err
	}
	check.Outcome = outcome
	check.Issues = issues
	return StructureReport{ValidationReport: report, Check: check}, cp.note("validate", map[string]any{
		"previewId": preview.PreviewID,
		"reportId":  report.ReportID,
		"outcome":   outcome,
	}, nil)
}

func (cp *ControlPlane) RecordValidation(preview Preview, suiteRevision, outcome string) (ValidationReport, error) {
	return cp.RecordValidationOn(preview.PreviewID, suiteRevision, outcome)
}

func (cp *ControlPlane) RecordValidationOn(previewID, suiteRevision, outcome string) (ValidationReport, error) {
	if previewID == "" {
		return ValidationReport{}, cp.note("record-validation", map[string]any{"suite": suiteRevision}, kernel.Fail(kernel.ErrUsageInvalid, "preview id is required"))
	}
	report := ValidationReport{
		ReportID:      "val-" + previewID + "-" + suiteRevision,
		PreviewID:     previewID,
		SuiteRevision: suiteRevision,
		Outcome:       outcome,
	}
	return report, cp.note("record-validation", map[string]any{"reportId": report.ReportID, "outcome": outcome, "suite": suiteRevision, "previewId": previewID}, nil)
}
