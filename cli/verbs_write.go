package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"kc/kernel"
	"kc/repository"
	"kc/writer"
)

// Write surface verbs. COMMIT and PROPOSAL target a SnapshotStore, APPEND
// targets a Stream; the change algebra is only PUT and REMOVE.

func writeVerbs() map[string]command {
	return map[string]command{
		"put":     {stage: stageGoverned, run: verbPut},
		"remove":  {stage: stageGoverned, run: verbRemove},
		"commit":  {stage: stageGoverned, run: verbCommit},
		"append":  {stage: stageGoverned, run: verbAppend},
		"ingest":  {stage: stageGoverned, run: verbIngest},
		"receipt": {stage: stageGoverned, run: verbReceipt},
	}
}

func verbPut(cx *invocation) (any, error) {
	value, ok, err := loadJSONFlag(cx.Flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("put requires --file or --value")
	}
	op, err := writeOperation(cx.Flags, repository.OpPut, value)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []repository.Operation{op})
}

func verbRemove(cx *invocation) (any, error) {
	op, err := writeOperation(cx.Flags, repository.OpRemove, nil)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []repository.Operation{op})
}

// commitOne is the single-operation COMMIT path behind put and remove.
func commitOne(cx *invocation, operations []repository.Operation) (any, error) {
	repositoryID, err := cx.repoFlag()
	if err != nil {
		return nil, err
	}
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

// verbCommit applies a prepared ChangeSet, which is how an inbound connector
// mirrors an external authority: it previews, then hands the file to COMMIT.
func verbCommit(cx *invocation) (any, error) {
	if workspaceIDOf(cx.Flags) != "" {
		if cx.flag("changeset") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --workspace cannot be combined with --changeset")
		}
		return commitWorkspace(cx)
	}
	file, err := cx.require("changeset")
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	raw, err := decodeChangeSet(body, file)
	if err != nil {
		return nil, err
	}
	if _, err := requireRepo(cx.WS, string(raw.TargetRepository)); err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     raw.TargetRepository,
		TargetRef:            repository.RefOrDefault(raw.TargetRef),
		BaseCommit:           raw.BaseCommit,
		ExpectedTargetCommit: raw.ExpectedTargetCommit,
		Operations:           raw.Operations,
		Message:              raw.Message,
		Provenance:           raw.Provenance,
	})
}

func verbAppend(cx *invocation) (any, error) {
	payload, ok, err := loadJSONFlag(cx.Flags, "--payload")
	if err != nil {
		return nil, err
	}
	if !ok {
		payload = map[string]any{}
	}
	eventID, err := cx.require("event-id")
	if err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	repoID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	streamRef, err := cx.require("stream")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.AppendIntent(commandID, writer.AppendIntent{
		TargetRepository: kernel.RepositoryID(repoID),
		StreamRef:        streamRef,
		ExpectedCursor:   cx.flag("cursor"),
		Entries: []repository.AppendEntry{{
			EventID:   eventID,
			EventType: cx.flag("event-type"),
			Payload:   payload,
			SchemaRef: cx.flag("schema-ref"),
		}},
	})
}

// verbIngest previews a directory as a ChangeSet. It is thin orchestration over
// COMMIT, not a collection framework: nothing is written until `kc commit`.
func verbIngest(cx *invocation) (any, error) {
	dir, err := cx.require("dir")
	if err != nil {
		return nil, err
	}
	repoID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := requireRepo(cx.WS, repoID)
	if err != nil {
		return nil, err
	}
	targetRef := cx.targetRef("ref")
	head, err := repo.Head(targetRef)
	if err != nil {
		return nil, err
	}
	preview, err := writer.Ingest(dir, kernel.RepositoryID(repoID), head)
	if err != nil {
		return nil, err
	}
	preview.ChangeSet.TargetRef = targetRef
	if out := cx.flag("out"); out != "" {
		b, err := json.MarshalIndent(preview.ChangeSet, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			return nil, err
		}
	}
	return preview, nil
}

func verbReceipt(cx *invocation) (any, error) {
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	entry, ok := cx.WS.Writer.Lookup(commandID)
	if !ok {
		return nil, fmt.Errorf("unknown command-id %s", commandID)
	}
	return entry, nil
}
