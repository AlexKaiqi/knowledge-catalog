package cli

import (
	"fmt"
	"os"

	"kc/controlplane"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledgeapp"
)

// Maintenance verbs: PROPOSAL → Preview → validate → Merge. Merge consults the
// gate list; a gate is a check over the pinned Preview, not a hook.
//
// The plane itself is stateless, so proposal / preview / validation ids are kept
// in the local home between commands. requireProposal and requirePreview are the
// only readers of that store.

func controlVerbs() map[string]command {
	return map[string]command{
		"governance-proposal-create":   {stage: stageGoverned, run: verbPropose},
		"governance-preview-create":    {stage: stageGoverned, run: verbPreview},
		"governance-preview-validate":  {stage: stageGoverned, run: verbValidate},
		"governance-validation-record": {stage: stageGoverned, run: verbRecordValidation},
		"governance-proposal-merge":    {stage: stageGoverned, run: verbMerge},
	}
}

func verbPropose(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := requireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	targetRef := cx.targetRef("target")
	operations, err := proposeOperations(cx.Flags)
	if err != nil {
		return nil, err
	}
	setTelemetryChangeCounts(cx.Observation, operations)
	proposalID, err := cx.require("proposal-id")
	if err != nil {
		return nil, err
	}
	candidate, err := cx.require("candidate")
	if err != nil {
		return nil, err
	}
	base := cx.flag("base")
	if base == "" {
		head, err := repo.Head(targetRef)
		if err != nil {
			return nil, err
		}
		base = string(head)
	}
	return (knowledgeapp.ProposalExecutor{
		Control: cx.WS.ControlPlane,
		Save: func(proposal controlplane.Proposal) error {
			cx.WS.Control.Proposals[proposal.ProposalID] = proposal
			return PersistControl(cx.WS)
		},
	}).Execute(cx.Context, knowledgeapp.ProposalRequest{
		Input: controlplane.ProposeInput{
			ProposalID:   proposalID,
			RepositoryID: kernel.RepositoryID(repositoryID),
			TargetRef:    targetRef,
			CandidateRef: candidate,
			BaseCommit:   kernel.CommitID(base),
			Operations:   operations,
			Rationale:    cx.flag("message"),
			Provenance:   originFrom(cx.Flags),
		},
	})
}

func verbPreview(cx *invocation) (any, error) {
	plane, err := planeFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	proposal, err := cx.requireProposal("proposal")
	if err != nil {
		return nil, err
	}
	if err := prepareKnowledgePinContext(cx.Flags); err != nil {
		return nil, err
	}
	if cx.flag("dataset") == "" && cx.flag("pin") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance preview create requires --dataset")
	}
	cat, err := pickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveOrReplay(cx.WS, cx.Home, cat, cx.flag("dataset"), cx.Flags)
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.PreviewExecutor{
		Control: plane,
		Save: func(preview controlplane.Preview) error {
			cx.WS.Control.Previews[preview.PreviewID] = preview
			return PersistControl(cx.WS)
		},
	}).Execute(cx.Context, knowledgeapp.PreviewRequest{
		Resolved: resolved,
		Proposal: proposal,
	})
}

// verbValidate runs the built-in structural checks: members attached, commits
// present. It does not run an external test suite.
func verbValidate(cx *invocation) (any, error) {
	plane, err := planeFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	preview, err := cx.requirePreview("preview")
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.ValidatePreviewExecutor{
		Control: plane,
		Save: func(report controlplane.ValidationReport) error {
			cx.WS.Control.Validations[report.ReportID] = report
			return PersistControl(cx.WS)
		},
	}).Execute(cx.Context, knowledgeapp.ValidatePreviewRequest{Preview: preview})
}

// verbRecordValidation only binds an outcome someone else produced. It never
// runs the suite, so PASSED here means "the caller asserts PASSED".
func verbRecordValidation(cx *invocation) (any, error) {
	plane, err := planeFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	outcome, err := cx.require("outcome")
	if err != nil {
		return nil, err
	}
	if outcome != "PASSED" && outcome != "FAILED" {
		return nil, fmt.Errorf("--outcome must be PASSED or FAILED")
	}
	suite, err := cx.require("suite")
	if err != nil {
		return nil, err
	}
	preview, err := cx.requirePreview("preview")
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.RecordValidationExecutor{
		Control: plane,
		Save: func(report controlplane.ValidationReport) error {
			cx.WS.Control.Validations[report.ReportID] = report
			return PersistControl(cx.WS)
		},
	}).Execute(cx.Context, knowledgeapp.RecordValidationRequest{
		Preview:       preview,
		SuiteRevision: suite,
		Outcome:       outcome,
	})
}

func verbMerge(cx *invocation) (any, error) {
	proposal, err := cx.requireProposal("proposal")
	if err != nil {
		return nil, err
	}
	preview, err := cx.requirePreview("preview")
	if err != nil {
		return nil, err
	}
	required := cx.WS.MergeRequired(proposal.TargetRepository)
	var validation controlplane.ValidationReport
	if id := cx.flag("validation"); id != "" {
		stored, ok := cx.WS.Control.Validations[id]
		if !ok {
			return nil, fmt.Errorf("unknown validation %s", id)
		}
		validation = stored
	} else if len(required) == 0 {
		return nil, fmt.Errorf("merge needs stored --proposal, --preview and --validation ids")
	}
	plane, err := planeFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.MergeProposalExecutor{Control: plane}).Execute(
		cx.Context,
		knowledgeapp.MergeProposalRequest{
			Proposal:       proposal,
			Preview:        preview,
			Validation:     validation,
			RequiredChecks: required,
			ObserveGate:    noOperationTelemetry(cx.Observation).gate,
		},
	)
}

func (cx *invocation) requireProposal(flag string) (controlplane.Proposal, error) {
	id, err := cx.require(flag)
	if err != nil {
		return controlplane.Proposal{}, err
	}
	proposal, ok := cx.WS.Control.Proposals[id]
	if !ok {
		return controlplane.Proposal{}, fmt.Errorf("unknown proposal; run propose first")
	}
	return proposal, nil
}

func (cx *invocation) requirePreview(flag string) (controlplane.Preview, error) {
	id, err := cx.require(flag)
	if err != nil {
		return controlplane.Preview{}, err
	}
	preview, ok := cx.WS.Control.Previews[id]
	if !ok {
		return controlplane.Preview{}, fmt.Errorf("unknown preview; run preview first")
	}
	return preview, nil
}

// planeFor binds the plane to the named Catalog, reusing the default plane when
// no --catalog was given so the merge gate stays attached.
func planeFor(ws *Home, flags map[string]FlagValue) (*controlplane.ControlPlane, error) {
	cat, err := pickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	if FlagString(flags, "catalog") == "" {
		return ws.ControlPlane, nil
	}
	plane := controlplane.New(ws.Store, ws.Writer, cat)
	plane.SetJournal(ws.Journal)
	ws.AttachMergeGate(plane)
	return plane, nil
}

// proposeOperations reads the change set for a PROPOSAL: a whole operations file,
// or a single PUT assembled from --value plus the address flags.
func proposeOperations(flags map[string]FlagValue) ([]knowledge.Operation, error) {
	file := FlagString(flags, "changeset")
	payload := FlagString(flags, "payload")
	if file != "" && payload != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --changeset or typed payload")
	}
	if payload != "" {
		var operations []knowledge.Operation
		if err := kernel.UnmarshalJSON([]byte(payload), &operations); err != nil || len(operations) == 0 {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "typed proposal payload must contain operations")
		}
		return operations, nil
	}
	if file != "" {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var asOps []knowledge.Operation
		if kernel.UnmarshalJSON(body, &asOps) == nil && len(asOps) > 0 {
			return asOps, nil
		}
		var wrapped struct {
			Operations []knowledge.Operation `json:"operations"`
		}
		if err := kernel.UnmarshalJSON(body, &wrapped); err != nil || len(wrapped.Operations) == 0 {
			return nil, fmt.Errorf("changeset must include operations")
		}
		return wrapped.Operations, nil
	}
	value, ok, err := loadJSONFlag(flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance proposal create requires --value, --file or --changeset")
	}
	op, err := writeOperation(flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return []knowledge.Operation{op}, nil
}
