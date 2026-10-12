package invocation

import (
	"fmt"

	"kc/invoke"
	"kc/kernel"
	knowledge "kc/knowledge"
	reader "kc/knowledge/reader"
	"kc/knowledge/writer"
	knowledgeapp "kc/knowledgeapp"
	"kc/snapshot"
	"os"
	"sort"
)

func VerbWriterHead(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := RequireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	ref := cx.TargetRef("ref")
	commit, err := repo.Head(ref)
	if err != nil {
		return nil, err
	}
	return map[string]any{"repository": repositoryID, "commit": commit}, nil
}

func VerbDiff(cx *Invocation) (any, error) {
	if cx.Flag("changeset") != "" || cx.Flag("payload") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "diff uses --dir, not --changeset")
	}
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	dir, err := cx.Require("dir")
	if err != nil {
		return nil, err
	}
	preview, base, current, err := PrepareDesiredIngest(cx, repositoryID, dir)
	if err != nil {
		return nil, err
	}
	return DesiredDiffResult(repositoryID, base, preview.ChangeSet.Operations, current), nil
}

func VerbPut(cx *Invocation) (any, error) {
	value, ok, err := LoadJSONFlag(cx.Flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("put requires --file or --value")
	}
	op, err := WriteOperation(cx.Flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return CommitOne(cx, []knowledge.Operation{op})
}

func VerbRemove(cx *Invocation) (any, error) {
	op, err := WriteOperation(cx.Flags, knowledge.OpRemove, nil)
	if err != nil {
		return nil, err
	}
	return CommitOne(cx, []knowledge.Operation{op})
}

// CommitOne is the single-operation COMMIT path behind put and remove. The
// bulk data-plane option belongs to the directory/changeset commit surface;
// single-operation commits reject it instead of silently ignoring the flag.
func CommitOne(cx *Invocation, operations []knowledge.Operation) (any, error) {
	if invoke.FlagBool(cx.Flags, "bulk") {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "--bulk applies to writer commit, not single-operation writes")
	}
	SetTelemetryChangeCounts(cx.Observation, operations)
	rawRepositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	if err := RequireSnapshotRepository(cx, rawRepositoryID); err != nil {
		return nil, err
	}
	repositoryID := kernel.RepositoryID(rawRepositoryID)
	commandID, err := cx.Require("command-id")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     repositoryID,
		TargetRef:            cx.TargetRef("ref"),
		BaseCommit:           kernel.CommitID(cx.Flag("base")),
		ExpectedTargetCommit: kernel.CommitID(cx.Flag("expected")),
		Operations:           operations,
		Message:              cx.Flag("message"),
		Provenance:           OriginFrom(cx.Flags),
	})
}

func VerbCommit(cx *Invocation) (any, error) {
	if invoke.DatasetOf(cx.Flags) != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "writer commit requires one Repository; Dataset file views are read-only")
	}
	if cx.Flag("dir") != "" {
		if cx.Flag("changeset") != "" || cx.Flag("payload") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --dir cannot be combined with --changeset")
		}
		return CommitDesiredDir(cx)
	}
	file := cx.Flag("changeset")
	payload := cx.Flag("payload")
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
	raw, err := DecodeChangeSet(body, label)
	if err != nil {
		return nil, err
	}
	if invoke.FlagBool(cx.Flags, "bulk") {
		raw.BulkIngest = true
	}
	SetTelemetryChangeCounts(cx.Observation, raw.Operations)
	if err := RequireSnapshotRepository(cx, string(raw.TargetRepository)); err != nil {
		return nil, err
	}
	commandID, err := cx.Require("command-id")
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
				BulkIngest:           raw.BulkIngest,
			},
		})
}

func CommitDesiredDir(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	dir, err := cx.Require("dir")
	if err != nil {
		return nil, err
	}
	commandID, err := cx.Require("command-id")
	if err != nil {
		return nil, err
	}
	preview, _, current, err := PrepareDesiredIngest(cx, repositoryID, dir)
	if err != nil {
		return nil, err
	}
	changeSet, _ := writer.OmitUnchanged(preview.ChangeSet, current)
	if len(changeSet.Operations) == 0 {
		if receipt, ok := ReplayedDesiredReceipt(cx, commandID); ok {
			return receipt, nil
		}
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "desired state already matches the current version")
	}
	SetTelemetryChangeCounts(cx.Observation, changeSet.Operations)
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
				BulkIngest:           invoke.FlagBool(cx.Flags, "bulk"),
			},
		})
}

func VerbReceipt(cx *Invocation) (any, error) {
	commandID, err := cx.Require("command-id")
	if err != nil {
		return nil, err
	}
	entry, ok := cx.WS.Commands.Lookup(commandID)
	if !ok {
		return nil, fmt.Errorf("unknown command-id %s", commandID)
	}
	return entry, nil
}

// RequireSnapshotRepository rejects registry targets and requires the named
// repository to be a Snapshot Repository of the active workspace. It is the
// shared prelude of every local desired-state write surface.
func RequireSnapshotRepository(cx *Invocation, repositoryID string) error {
	if repositoryID == cx.WS.File.Registry.ID {
		return kernel.Fail(kernel.ErrTargetRepositoryDenied, "registry authority %s is not a member Snapshot Repository", repositoryID)
	}
	_, err := RequireRepo(cx.WS, repositoryID)
	return err
}

// PrepareDesiredIngest runs the shared local desired-state pipeline: deny
// registry targets, require a Snapshot Repository, fix the base commit, ingest
// the desired directory, and collect current digests for the change set.
func PrepareDesiredIngest(cx *Invocation, repositoryID, dir string) (writer.IngestPreview, kernel.CommitID, map[string]string, error) {
	if err := RequireSnapshotRepository(cx, repositoryID); err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	base, err := DesiredBaseCommit(cx, kernel.RepositoryID(repositoryID))
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	preview, err := IngestDesired(cx.Flags, dir, repositoryID, cx.TargetRef("ref"), base)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	current, err := CurrentDigests(cx.WS.Reader, kernel.RepositoryID(repositoryID), base, preview.ChangeSet.Operations)
	if err != nil {
		return writer.IngestPreview{}, "", nil, err
	}
	return preview, base, current, nil
}

func DesiredDiffResult(repository string, commit kernel.CommitID, ops []knowledge.Operation, current map[string]string) map[string]any {
	return map[string]any{
		"repository": repository,
		"commit":     commit,
		"changes":    ClassifyDesiredChanges(ops, current),
	}
}

func ReplayedDesiredReceipt(cx *Invocation, commandID string) (any, bool) {
	entry, ok := cx.WS.Writer.Lookup(commandID)
	if !ok || entry.Receipt.CommandID == "" {
		return nil, false
	}
	receipt := entry.Receipt
	receipt.Disposition = writer.DispositionReplayed
	return receipt, true
}

func DecodeChangeSet(body []byte, label string) (knowledge.CommitChangeSet, error) {
	var raw knowledge.CommitChangeSet
	if err := kernel.UnmarshalJSON(body, &raw); err != nil {
		return knowledge.CommitChangeSet{}, fmt.Errorf("%s is not valid JSON", label)
	}
	if raw.TargetRepository != "" && raw.Operations != nil {
		return raw, nil
	}
	var wrapped struct {
		ChangeSet knowledge.CommitChangeSet `json:"changeSet"`
	}
	if err := kernel.UnmarshalJSON(body, &wrapped); err != nil || wrapped.ChangeSet.TargetRepository == "" || wrapped.ChangeSet.Operations == nil {
		return knowledge.CommitChangeSet{}, fmt.Errorf("changeset must include targetRepository and operations")
	}
	return wrapped.ChangeSet, nil
}

func DesiredBaseCommit(cx *Invocation, repositoryID kernel.RepositoryID) (kernel.CommitID, error) {
	if base := cx.Flag("base"); base != "" {
		return kernel.CommitID(base), nil
	}
	repo, err := RequireRepo(cx.WS, string(repositoryID))
	if err != nil {
		return "", err
	}
	head, err := repo.Head(cx.TargetRef("ref"))
	if err != nil {
		return "", err
	}
	return head, nil
}

func IngestDesired(flags map[string]invoke.FlagValue, dir, repoID, targetRef string, base kernel.CommitID) (writer.IngestPreview, error) {
	preview, err := writer.Ingest(dir, kernel.RepositoryID(repoID), base)
	if err != nil {
		return writer.IngestPreview{}, err
	}
	if provenance := OriginFrom(flags); provenance != nil {
		preview.ChangeSet.Provenance = provenance
	}
	preview.ChangeSet.TargetRef = snapshot.RefOrDefault(targetRef)
	preview.ChangeSet.ExpectedTargetCommit = base
	return preview, nil
}

func CurrentDigests(r *reader.Reader, repositoryID kernel.RepositoryID, commit kernel.CommitID, ops []knowledge.Operation) (map[string]string, error) {
	out := map[string]string{}
	if r == nil || commit == "" {
		return out, nil
	}
	for _, op := range ops {
		resolution, err := r.ResolveAddress(repositoryID, op.Address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			return nil, err
		}
		if resolution.Status == knowledge.StatusUnresolved || resolution.Digest == "" {
			continue
		}
		out[knowledge.AddressKey(op.Address)] = string(resolution.Digest)
	}
	return out, nil
}

func ClassifyDesiredChanges(ops []knowledge.Operation, current map[string]string) []map[string]any {
	changes := make([]map[string]any, 0)
	for _, op := range ops {
		row := map[string]any{"objectId": string(op.Address.ObjectID)}
		if op.Address.AspectName != "" {
			row["aspectName"] = op.Address.AspectName
		}
		if op.Address.MemberKey != "" {
			row["memberKey"] = op.Address.MemberKey
		}
		if op.Op == knowledge.OpRemove {
			row["change"] = "remove"
			changes = append(changes, row)
			continue
		}
		digest := string(kernel.CanonicalDigest(op.Value))
		existing, ok := current[knowledge.AddressKey(op.Address)]
		switch {
		case !ok:
			row["change"] = "add"
			changes = append(changes, row)
		case existing != digest:
			row["change"] = "update"
			changes = append(changes, row)
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		left, _ := changes[i]["objectId"].(string)
		right, _ := changes[j]["objectId"].(string)
		if left != right {
			return left < right
		}
		leftAspect, _ := changes[i]["aspectName"].(string)
		rightAspect, _ := changes[j]["aspectName"].(string)
		if leftAspect != rightAspect {
			return leftAspect < rightAspect
		}
		leftMember, _ := changes[i]["memberKey"].(string)
		rightMember, _ := changes[j]["memberKey"].(string)
		return leftMember < rightMember
	})
	return changes
}
