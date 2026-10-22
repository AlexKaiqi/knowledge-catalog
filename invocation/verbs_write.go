package invocation

import (
	"fmt"
	objectservice "kc/object/service"

	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	reader "kc/object/reader"
	"kc/object/writer"

	"kc/snapshot"
	"os"
	"sort"
	"strings"
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
	empty, known, err := snapshot.ProbeEmptyTree(repo, commit)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"repository": repositoryID, "commit": commit}
	if known {
		result["empty"] = empty
	}
	return result, nil
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
	if cx.Flag("reference") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "put requires an object URN")
	}
	if _, err := cx.Require("command-id"); err != nil {
		return nil, err
	}
	if err := NormalizeReferenceFlags(cx.Flags); err != nil {
		return nil, err
	}
	value, ok, err := LoadJSONFlag(cx.Flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok && cx.Flag("value-source") == "" {
		return nil, fmt.Errorf("put requires --file or --value")
	}
	return maintainReference(cx, objectmodel.OpPut, value)
}

func VerbRemove(cx *Invocation) (any, error) {
	if cx.Flag("reference") == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "remove requires an object URN")
	}
	if _, err := cx.Require("command-id"); err != nil {
		return nil, err
	}
	if err := NormalizeReferenceFlags(cx.Flags); err != nil {
		return nil, err
	}
	return maintainReference(cx, objectmodel.OpRemove, nil)
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
	return (objectservice.CommitExecutor{Writer: cx.WS.Writer}).Execute(cx.Context,
		objectservice.CommitRequest{
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
	if err := RequireSnapshotRepository(cx, repositoryID); err != nil {
		return nil, err
	}
	preview, err := IngestDesired(cx.Flags, dir, repositoryID, cx.TargetRef("ref"), kernel.CommitID(cx.Flag("base")))
	if err != nil {
		return nil, err
	}
	changeSet := preview.ChangeSet
	SetTelemetryChangeCounts(cx.Observation, changeSet.Operations)
	return (objectservice.CommitExecutor{Writer: cx.WS.Writer}).Execute(cx.Context,
		objectservice.CommitRequest{
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

// BranchRef spells a branch flag as a full Ref. A bare name is a branch under
// refs/heads/; a full Ref is kept as given.
func BranchRef(name string) string {
	if name == "" || strings.HasPrefix(name, "refs/") {
		return name
	}
	return "refs/heads/" + name
}

func VerbWriterBranch(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	name, err := cx.Require("name")
	if err != nil {
		return nil, err
	}
	if cx.Flag("from") != "" && cx.Flag("commit") != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --from or --commit")
	}
	if err := RequireSnapshotRepository(cx, repositoryID); err != nil {
		return nil, err
	}
	return cx.WS.Writer.CreateBranch(writer.BranchIntent{
		TargetRepository: kernel.RepositoryID(repositoryID),
		Ref:              BranchRef(name),
		FromRef:          BranchRef(cx.Flag("from")),
		FromCommit:       kernel.CommitID(cx.Flag("commit")),
	})
}

func VerbWriterMerge(cx *Invocation) (any, error) {
	repositoryID, err := cx.Require("repo")
	if err != nil {
		return nil, err
	}
	from, err := cx.Require("from")
	if err != nil {
		return nil, err
	}
	commandID, err := cx.Require("command-id")
	if err != nil {
		return nil, err
	}
	if err := RequireSnapshotRepository(cx, repositoryID); err != nil {
		return nil, err
	}
	return cx.WS.Writer.Merge(commandID, writer.MergeIntent{
		TargetRepository:     kernel.RepositoryID(repositoryID),
		SourceRef:            BranchRef(from),
		SourceCommit:         kernel.CommitID(cx.Flag("commit")),
		TargetRef:            snapshot.RefOrDefault(BranchRef(cx.Flag("into"))),
		ExpectedTargetCommit: kernel.CommitID(cx.Flag("expected")),
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
	// Receipt permission exposes command status, not private recovery material.
	entry.Request.TreeChangeSet = nil
	entry.Request.ApplicationRequest = nil
	entry.Request.Authority = ""
	entry.Execution = ""
	entry.Phase = ""
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

func DesiredDiffResult(repository string, commit kernel.CommitID, ops []objectmodel.Operation, current map[string]string) map[string]any {
	return map[string]any{
		"repository": repository,
		"commit":     commit,
		"changes":    ClassifyDesiredChanges(ops, current),
	}
}

func DecodeChangeSet(body []byte, label string) (objectmodel.CommitChangeSet, error) {
	var raw objectmodel.CommitChangeSet
	if err := kernel.UnmarshalJSON(body, &raw); err != nil {
		return objectmodel.CommitChangeSet{}, fmt.Errorf("%s is not valid JSON", label)
	}
	if raw.TargetRepository != "" && raw.Operations != nil {
		return raw, nil
	}
	var wrapped struct {
		ChangeSet objectmodel.CommitChangeSet `json:"changeSet"`
	}
	if err := kernel.UnmarshalJSON(body, &wrapped); err != nil || wrapped.ChangeSet.TargetRepository == "" || wrapped.ChangeSet.Operations == nil {
		return objectmodel.CommitChangeSet{}, fmt.Errorf("changeset must include targetRepository and operations")
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
	preview.ChangeSet.BulkIngest = invoke.FlagBool(flags, "bulk")
	return preview, nil
}

func CurrentDigests(r *reader.Reader, repositoryID kernel.RepositoryID, commit kernel.CommitID, ops []objectmodel.Operation) (map[string]string, error) {
	out := map[string]string{}
	if r == nil || commit == "" {
		return out, nil
	}
	for _, op := range ops {
		resolution, err := r.ResolveAddress(repositoryID, op.Address, commit)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return nil, err
		}
		if resolution.Status == objectmodel.StatusUnresolved || resolution.Digest == "" {
			continue
		}
		out[objectmodel.AddressKey(op.Address)] = string(resolution.Digest)
	}
	return out, nil
}

func ClassifyDesiredChanges(ops []objectmodel.Operation, current map[string]string) []map[string]any {
	changes := make([]map[string]any, 0)
	for _, op := range ops {
		row := map[string]any{"objectId": string(op.Address.ObjectID)}
		if op.Address.AspectName != "" {
			row["aspectName"] = op.Address.AspectName
		}
		if op.Address.MemberKey != "" {
			row["memberKey"] = op.Address.MemberKey
		}
		if op.Op == objectmodel.OpRemove {
			row["change"] = "remove"
			changes = append(changes, row)
			continue
		}
		digest := string(kernel.CanonicalDigest(op.Value))
		existing, ok := current[objectmodel.AddressKey(op.Address)]
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
