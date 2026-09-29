package invocation

import (
	"fmt"
	"os"

	"kc/controlplane"
	kchome "kc/home"
	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	knowledgeapp "kc/knowledgeapp"
)

func VerbPropose(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := RequireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	targetRef := cx.TargetRef("target")
	operations, err := ProposeOperations(cx.Flags)
	if err != nil {
		return nil, err
	}
	SetTelemetryChangeCounts(cx.Observation, operations)
	proposalID, err := cx.Require("proposal-id")
	if err != nil {
		return nil, err
	}
	candidate, err := cx.Require("candidate")
	if err != nil {
		return nil, err
	}
	base := cx.Flag("base")
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
			Rationale:    cx.Flag("message"),
			Provenance:   OriginFrom(cx.Flags),
		},
	})
}

func VerbPreview(cx *Invocation) (any, error) {
	plane, err := PlaneFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	proposal, err := cx.RequireProposal("proposal")
	if err != nil {
		return nil, err
	}
	if err := PrepareKnowledgePinContext(cx.Flags); err != nil {
		return nil, err
	}
	if cx.Flag("dataset") == "" && cx.Flag("pin") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance preview create requires --dataset")
	}
	cat, err := PickCatalog(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveOrReplay(cx.WS, cx.Home, cat, cx.Flag("dataset"), cx.Flags)
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

// VerbValidate runs the built-in structural checks: members attached, commits
// present. It does not run an external test suite.
func VerbValidate(cx *Invocation) (any, error) {
	plane, err := PlaneFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	preview, err := cx.RequirePreview("preview")
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

// VerbRecordValidation only binds an outcome someone else produced. It never
// runs the suite, so PASSED here means "the caller asserts PASSED".
func VerbRecordValidation(cx *Invocation) (any, error) {
	plane, err := PlaneFor(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	outcome, err := cx.Require("outcome")
	if err != nil {
		return nil, err
	}
	if outcome != "PASSED" && outcome != "FAILED" {
		return nil, fmt.Errorf("--outcome must be PASSED or FAILED")
	}
	suite, err := cx.Require("suite")
	if err != nil {
		return nil, err
	}
	preview, err := cx.RequirePreview("preview")
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

func VerbMerge(cx *Invocation) (any, error) {
	proposal, err := cx.RequireProposal("proposal")
	if err != nil {
		return nil, err
	}
	preview, err := cx.RequirePreview("preview")
	if err != nil {
		return nil, err
	}
	required := cx.WS.MergeRequired(proposal.TargetRepository)
	var validation controlplane.ValidationReport
	if id := cx.Flag("validation"); id != "" {
		stored, ok := cx.WS.Control.Validations[id]
		if !ok {
			return nil, fmt.Errorf("unknown validation %s", id)
		}
		validation = stored
	} else if len(required) == 0 {
		return nil, fmt.Errorf("merge needs stored --proposal, --preview and --validation ids")
	}
	plane, err := PlaneFor(cx.WS, cx.Flags)
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
			ObserveGate:    invoke.NoObservationTelemetry(cx.Observation).Gate,
		},
	)
}

// PlaneFor binds the plane to the named Catalog, reusing the default plane when
// no --catalog was given so the merge gate stays attached.
func PlaneFor(ws *kchome.Home, flags map[string]invoke.FlagValue) (*controlplane.ControlPlane, error) {
	cat, err := PickCatalog(ws, flags)
	if err != nil {
		return nil, err
	}
	if invoke.FlagString(flags, "catalog") == "" {
		return ws.ControlPlane, nil
	}
	plane := controlplane.New(ws.Store, ws.Writer, cat)
	plane.SetJournal(ws.Journal)
	ws.AttachMergeGate(plane)
	return plane, nil
}

// ProposeOperations reads the change set for a PROPOSAL: a whole operations file,
// or a single PUT assembled from --value plus the address flags.
func ProposeOperations(flags map[string]invoke.FlagValue) ([]knowledge.Operation, error) {
	file := invoke.FlagString(flags, "changeset")
	payload := invoke.FlagString(flags, "payload")
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
	value, ok, err := LoadJSONFlag(flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "governance proposal create requires --value, --file or --changeset")
	}
	op, err := WriteOperation(flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return []knowledge.Operation{op}, nil
}
