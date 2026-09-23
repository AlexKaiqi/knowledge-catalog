package cli

import (
	"fmt"
	"os"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/writer"
	"kc/knowledgeapp"
	"kc/snapshot"
)

// Write surface verbs. COMMIT and PROPOSAL compile Knowledge changes onto one
// Snapshot TreeStore; the algebra is only PUT and REMOVE. Dynamic State/Stream
// values are observations, not Writer surfaces.

func writeVerbs() map[string]command {
	return map[string]command{
		"diff":           {stage: stageGoverned, run: verbDiff},
		"writer-put":     {stage: stageGoverned, run: verbPut},
		"writer-remove":  {stage: stageGoverned, run: verbRemove},
		"writer-commit":  {stage: stageGoverned, run: verbCommit},
		"writer-receipt": {stage: stageGoverned, run: verbReceipt},
		"writer-head":    {stage: stageGoverned, run: verbWriterHead},
	}
}

func verbWriterHead(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := requireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	ref := cx.targetRef("ref")
	commit, err := repo.Head(ref)
	if err != nil {
		return nil, err
	}
	return map[string]any{"repository": repositoryID, "commit": commit}, nil
}

func verbDiff(cx *invocation) (any, error) {
	if cx.flag("changeset") != "" || cx.flag("payload") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "diff uses --dir, not --changeset")
	}
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	dir, err := cx.require("dir")
	if err != nil {
		return nil, err
	}
	preview, base, current, err := prepareDesiredIngest(cx, repositoryID, dir)
	if err != nil {
		return nil, err
	}
	return desiredDiffResult(repositoryID, base, preview.ChangeSet.Operations, current), nil
}

func verbPut(cx *invocation) (any, error) {
	value, ok, err := loadJSONFlag(cx.Flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("put requires --file or --value")
	}
	op, err := writeOperation(cx.Flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []knowledge.Operation{op})
}

func verbRemove(cx *invocation) (any, error) {
	op, err := writeOperation(cx.Flags, knowledge.OpRemove, nil)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []knowledge.Operation{op})
}

// commitOne is the single-operation COMMIT path behind put and remove.
func commitOne(cx *invocation, operations []knowledge.Operation) (any, error) {
	setTelemetryChangeCounts(cx.Observation, operations)
	rawRepositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if err := requireSnapshotRepository(cx, rawRepositoryID); err != nil {
		return nil, err
	}
	repositoryID := kernel.RepositoryID(rawRepositoryID)
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     repositoryID,
		TargetRef:            cx.targetRef("ref"),
		BaseCommit:           kernel.CommitID(cx.flag("base")),
		ExpectedTargetCommit: kernel.CommitID(cx.flag("expected")),
		Operations:           operations,
		Message:              cx.flag("message"),
		Provenance:           originFrom(cx.Flags),
	})
}

func verbCommit(cx *invocation) (any, error) {
	if setIDOf(cx.Flags) != "" {
		if cx.flag("changeset") != "" || cx.flag("dir") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --dataset cannot be combined with --dir or --changeset")
		}
		return commitWorkspace(cx)
	}
	if cx.flag("dir") != "" {
		if cx.flag("changeset") != "" || cx.flag("payload") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --dir cannot be combined with --changeset")
		}
		return commitDesiredDir(cx)
	}
	file := cx.flag("changeset")
	payload := cx.flag("payload")
	if file != "" && payload != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --changeset or typed payload")
	}
	var body []byte
	var err error
	label := file
	if payload != "" {
		body = []byte(payload)
		label = "typed commit payload"
	} else {
		if file == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "writer commit requires --dir or --changeset")
		}
		body, err = os.ReadFile(file)
		if err != nil {
			return nil, err
		}
	}
	raw, err := decodeChangeSet(body, label)
	if err != nil {
		return nil, err
	}
	setTelemetryChangeCounts(cx.Observation, raw.Operations)
	if err := requireSnapshotRepository(cx, string(raw.TargetRepository)); err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	return (knowledgeapp.CommitExecutor{Writer: cx.WS.Writer}).Execute(cx.Context,
		knowledgeapp.CommitRequest{
			CommandID: commandID,
			Intent: writer.CommitIntent{
				TargetRepository:     raw.TargetRepository,
				TargetRef:            snapshot.RefOrDefault(raw.TargetRef),
				BaseCommit:           raw.BaseCommit,
				ExpectedTargetCommit: raw.ExpectedTargetCommit,
				Operations:           raw.Operations,
				Message:              raw.Message,
				Provenance:           raw.Provenance,
			},
		})
}

func commitDesiredDir(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	dir, err := cx.require("dir")
	if err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	preview, _, current, err := prepareDesiredIngest(cx, repositoryID, dir)
	if err != nil {
		return nil, err
	}
	changeSet, _ := writer.OmitUnchanged(preview.ChangeSet, current)
	if len(changeSet.Operations) == 0 {
		if receipt, ok := replayedDesiredReceipt(cx, commandID); ok {
			return receipt, nil
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "desired state already matches the current version")
	}
	setTelemetryChangeCounts(cx.Observation, changeSet.Operations)
	return (knowledgeapp.CommitExecutor{Writer: cx.WS.Writer}).Execute(cx.Context,
		knowledgeapp.CommitRequest{
			CommandID: commandID,
			Intent: writer.CommitIntent{
				TargetRepository:     changeSet.TargetRepository,
				TargetRef:            snapshot.RefOrDefault(changeSet.TargetRef),
				BaseCommit:           changeSet.BaseCommit,
				ExpectedTargetCommit: changeSet.ExpectedTargetCommit,
				Operations:           changeSet.Operations,
				Message:              changeSet.Message,
				Provenance:           changeSet.Provenance,
			},
		})
}

func verbReceipt(cx *invocation) (any, error) {
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	entry, ok := cx.WS.Commands.Lookup(commandID)
	if !ok {
		return nil, fmt.Errorf("unknown command-id %s", commandID)
	}
	return entry, nil
}
