package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"kc/controlplane"
	"kc/kernel"
	"kc/knowledge"
)

// Maintenance verbs: PROPOSAL → Preview → validate → Merge. Merge consults the
// gate list; a gate is a check over the pinned Preview, not a hook.
//
// The plane itself is stateless, so proposal / preview / validation ids are kept
// in the local home between commands. requireProposal and requirePreview are the
// only readers of that store.

func controlVerbs() map[string]command {
	return map[string]command{
		"propose":           {stage: stageGoverned, run: verbPropose},
		"preview":           {stage: stageGoverned, run: verbPreview},
		"validate":          {stage: stageGoverned, run: verbValidate},
		"record-validation": {stage: stageGoverned, run: verbRecordValidation},
		"merge":             {stage: stageGoverned, run: verbMerge},
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
	setTelemetryChangeCounts(cx.Flags, operations)
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
	proposal, err := cx.WS.ControlPlane.Propose(controlplane.ProposeInput{
		ProposalID:   proposalID,
		RepositoryID: kernel.RepositoryID(repositoryID),
		TargetRef:    targetRef,
		CandidateRef: candidate,
		BaseCommit:   kernel.CommitID(base),
		Operations:   operations,
		Rationale:    cx.flag("message"),
		Provenance:   originFrom(cx.Flags),
	})
	if err != nil {
		return nil, err
	}
	cx.WS.Control.Proposals[proposal.ProposalID] = proposal
	if err := PersistControl(cx.WS); err != nil {
		return nil, err
	}
	return proposal, nil
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
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	preview, err := plane.CreatePreview(workspaceID, proposal)
	if err != nil {
		return nil, err
	}
	cx.WS.Control.Previews[preview.PreviewID] = preview
	if err := PersistControl(cx.WS); err != nil {
		return nil, err
	}
	return preview, nil
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
	report, err := plane.ValidateStructure(preview)
	if err != nil {
		return nil, err
	}
	cx.WS.Control.Validations[report.ReportID] = report.ValidationReport
	if err := PersistControl(cx.WS); err != nil {
		return nil, err
	}
	return report, nil
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
	report, err := plane.RecordValidation(preview, suite, outcome)
	if err != nil {
		return nil, err
	}
	cx.WS.Control.Validations[report.ReportID] = report
	if err := PersistControl(cx.WS); err != nil {
		return nil, err
	}
	return report, nil
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
	required := cx.WS.mergeRequired(proposal.TargetRepository)
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
	commitID, err := plane.Merge(proposal, preview, validation)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"commitId":   commitID,
		"proposalId": proposal.ProposalID,
		"previewId":  preview.PreviewID,
		"repository": proposal.TargetRepository,
		"targetRef":  proposal.TargetRef,
		"gate": map[string]any{
			"status":   "PASSED",
			"basis":    preview.PreviewID,
			"required": required,
		},
	}, nil
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
	ws.attachMergeGate(plane)
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
		if err := json.Unmarshal([]byte(payload), &operations); err != nil || len(operations) == 0 {
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
		if json.Unmarshal(body, &asOps) == nil && len(asOps) > 0 {
			return asOps, nil
		}
		var wrapped struct {
			Operations []knowledge.Operation `json:"operations"`
		}
		if err := json.Unmarshal(body, &wrapped); err != nil || len(wrapped.Operations) == 0 {
			return nil, fmt.Errorf("changeset must include operations")
		}
		return wrapped.Operations, nil
	}
	value, ok, err := loadJSONFlag(flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("propose requires --file/--value or --changeset")
	}
	op, err := writeOperation(flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return []knowledge.Operation{op}, nil
}
