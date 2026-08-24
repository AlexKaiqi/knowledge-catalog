package controlplane

import (
	"kc/catalog"
	"kc/gate"
	"kc/internal/journal"
	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

type Proposal struct {
	ProposalID       string              `json:"proposalId"`
	TargetRepository kernel.RepositoryID `json:"targetRepository"`
	TargetRef        string              `json:"targetRef"`
	CandidateRef     string              `json:"candidateRef"`
	BaseCommit       kernel.CommitID     `json:"baseCommit"`
	CandidateCommit  kernel.CommitID     `json:"candidateCommit"`
	Rationale        string              `json:"rationale,omitempty"`
}

type Preview struct {
	PreviewID    string                                  `json:"previewId"`
	WorkspaceID  string                                  `json:"workspaceId"`
	Repositories map[kernel.RepositoryID]kernel.CommitID `json:"repositories"`
	BaseCommit   kernel.CommitID                         `json:"baseCommit"`
	Candidate    PreviewCandidate                        `json:"candidate"`
}

type PreviewCandidate struct {
	RepositoryID kernel.RepositoryID `json:"repositoryId"`
	CommitID     kernel.CommitID     `json:"commitId"`
}

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

type ProposeInput struct {
	ProposalID   string
	RepositoryID kernel.RepositoryID
	TargetRef    string
	CandidateRef string
	BaseCommit   kernel.CommitID
	Operations   []repository.Operation
	Rationale    string
	Provenance   *kernel.ProvenanceEnvelope
}

type ControlPlane struct {
	store   *repository.Store
	writer  *writer.Writer
	catalog *catalog.Catalog
	journal journal.Journal

	mergeRequired func(repo kernel.RepositoryID) []string
	mergeEvidence func(basisID string) []gate.Evidence
}

func New(store *repository.Store, w *writer.Writer, cat *catalog.Catalog) *ControlPlane {
	return &ControlPlane{store: store, writer: w, catalog: cat}
}

func (cp *ControlPlane) SetJournal(j journal.Journal) { cp.journal = j }

func (cp *ControlPlane) SetMergeGate(required func(repo kernel.RepositoryID) []string, evidence func(basisID string) []gate.Evidence) {
	cp.mergeRequired = required
	cp.mergeEvidence = evidence
}

func (cp *ControlPlane) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(cp.journal, journal.LayerSystem, "controlplane", cmd, refs, err)
}

func (cp *ControlPlane) Propose(input ProposeInput) (Proposal, error) {
	base := input.BaseCommit
	if base == "" {
		repo, err := cp.store.Require(input.RepositoryID, kernel.ErrTargetRepositoryDenied)
		if err != nil {
			return Proposal{}, err
		}
		targetRef := repository.RefOrDefault(input.TargetRef)
		head, err := repo.Head(targetRef)
		if err != nil {
			return Proposal{}, err
		}
		base = head
	}
	receipt, err := cp.writer.Propose("proposal:"+input.ProposalID, writer.ProposeIntent{
		TargetRepository: input.RepositoryID,
		TargetRef:        input.TargetRef,
		CandidateRef:     input.CandidateRef,
		BaseCommit:       base,
		Operations:       input.Operations,
		Message:          input.Rationale,
		Provenance:       input.Provenance,
	})
	if err != nil {
		return Proposal{}, cp.note("propose", map[string]any{"proposalId": input.ProposalID, "repositoryId": string(input.RepositoryID)}, err)
	}
	proposal := Proposal{
		ProposalID:       input.ProposalID,
		TargetRepository: input.RepositoryID,
		TargetRef:        input.TargetRef,
		CandidateRef:     input.CandidateRef,
		BaseCommit:       base,
		CandidateCommit:  receipt.Result.CommitID,
		Rationale:        input.Rationale,
	}
	return proposal, cp.note("propose", map[string]any{
		"proposalId":      proposal.ProposalID,
		"repositoryId":    string(proposal.TargetRepository),
		"candidateCommit": string(proposal.CandidateCommit),
	}, nil)
}

func (cp *ControlPlane) CreatePreview(workspaceID string, proposal Proposal) (Preview, error) {
	resolved, err := cp.catalog.ResolveWorkspace(workspaceID)
	if err != nil {
		return Preview{}, err
	}
	if resolved.Repositories[proposal.TargetRepository] != proposal.BaseCommit {
		return Preview{}, kernel.Fail(kernel.ErrValidationBasisMismatch, "proposal base is not the current workspace member")
	}
	overlaid, err := cp.catalog.ResolveWorkspaceOverlay(workspaceID, map[kernel.RepositoryID]kernel.CommitID{
		proposal.TargetRepository: proposal.CandidateCommit,
	})
	if err != nil {
		return Preview{}, cp.note("preview", map[string]any{"proposalId": proposal.ProposalID, "workspace": workspaceID}, err)
	}
	preview := Preview{
		PreviewID:    "preview-" + overlaid.PinID,
		WorkspaceID:  workspaceID,
		Repositories: overlaid.Repositories,
		BaseCommit:   proposal.BaseCommit,
		Candidate: PreviewCandidate{
			RepositoryID: proposal.TargetRepository,
			CommitID:     proposal.CandidateCommit,
		},
	}
	return preview, cp.note("preview", map[string]any{"previewId": preview.PreviewID, "workspace": workspaceID}, nil)
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

func (cp *ControlPlane) Merge(proposal Proposal, preview Preview, validation ValidationReport) (commitID kernel.CommitID, err error) {
	refs := map[string]any{"proposalId": proposal.ProposalID, "previewId": preview.PreviewID, "reportId": validation.ReportID}
	defer func() { err = cp.note("merge", refs, err) }()
	repo, err := cp.store.Require(proposal.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return "", err
	}
	if preview.Candidate.RepositoryID != proposal.TargetRepository ||
		preview.Candidate.CommitID != proposal.CandidateCommit ||
		preview.Repositories[proposal.TargetRepository] != proposal.CandidateCommit ||
		preview.BaseCommit != proposal.BaseCommit {
		return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
	}
	if validation.ReportID != "" && validation.PreviewID != preview.PreviewID {
		return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
	}
	required := []string{}
	if cp.mergeRequired != nil {
		required = cp.mergeRequired(proposal.TargetRepository)
	}
	if len(required) == 0 {
		if validation.PreviewID != preview.PreviewID {
			return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation is not bound to the exact preview")
		}
		if validation.Outcome != "PASSED" {
			return "", kernel.Fail(kernel.ErrValidationBasisMismatch, "validation did not pass")
		}
	} else {
		ev := []gate.Evidence{}
		if cp.mergeEvidence != nil {
			ev = append(ev, cp.mergeEvidence(preview.PreviewID)...)
		}
		if validation.ReportID != "" {
			ev = append(ev, gate.Evidence{
				Name:    validation.SuiteRevision,
				BasisID: validation.PreviewID,
				Outcome: validation.Outcome,
			})
		}
		if err := gate.Check(required, gate.OnBasis(ev, preview.PreviewID)); err != nil {
			return "", err
		}
	}
	if current, ok := repo.GetRef(proposal.CandidateRef); !ok || current != proposal.CandidateCommit {
		return "", kernel.Fail(kernel.ErrCandidateMoved, "candidate advanced after validation")
	}
	commitID, err = repo.Merge(proposal.TargetRef, proposal.CandidateCommit, proposal.BaseCommit)
	if err != nil {
		return "", err
	}
	// nil ObjectIDs means "unknown", which is exactly the case for a member that
	// does not interpret knowledge files: it merged, there is just nothing to diff.
	var ids []kernel.ObjectID
	if knowledge, ok := repository.KnowledgeOf(repo); ok {
		if changed, cerr := repository.ChangedObjectIDs(knowledge, proposal.BaseCommit, commitID); cerr == nil {
			ids = changed
		}
	}
	cp.store.NotifySnapshot(repository.Snapshot{
		Repository: repo,
		From:       proposal.BaseCommit,
		To:         commitID,
		ObjectIDs:  ids,
	})
	refs["commitId"] = string(commitID)
	return commitID, nil
}
