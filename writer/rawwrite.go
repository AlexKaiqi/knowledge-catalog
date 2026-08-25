package writer

import (
	"kc/kernel"
	"kc/knowledge"
	snapshotpkg "kc/snapshot"
)

// RawWrite is the raw-path write surface: literal path bytes, no Address, no
// object_id, no schema_ref check — the shape a filesystem-style consumer
// (e.g. a virtual workspace for an external agent harness, docs/COMPOSITION.md)
// actually writes with. It still goes through Writer: same target/CAS/
// idempotency/journal machinery as Commit, just against snapshot.TreeStore instead
// of Operations/Knowledge. CommitReceipt is reused as-is; the shape does not
// need a distinct type.
//
// ErrCapabilityUnsatisfied when the target does not implement TreeStore
// (e.g. a remote-only member that only has Knowledge, if such a thing ever
// exists — today every TreeStore implementer is also a snapshot.Store with
// a git-shaped tree, so this mainly guards a future adapter that has neither).
func (w *Writer) RawWrite(commandID string, cs snapshotpkg.TreeChangeSet) (receipt CommitReceipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(cs.TargetRepository), "targetRef": cs.TargetRef}
	defer func() { err = w.note(string(knowledge.SurfaceTreeWrite), refs, err) }()
	if cs.TargetRepository == "" {
		return CommitReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "raw write requires a target repository")
	}
	if cs.TargetRef == "" {
		return CommitReceipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "raw write requires a target ref")
	}
	if len(cs.Changes) == 0 {
		return CommitReceipt{}, kernel.Fail(kernel.ErrUsageInvalid, "raw changeset has no changes")
	}
	target, err := w.store.Require(cs.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return CommitReceipt{}, err
	}
	if target.Archived() {
		return CommitReceipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", cs.TargetRepository)
	}
	digest := string(kernel.CanonicalDigest(cs))
	if prior, replayed, err := w.conflictOrReplay(commandID, digest); err != nil {
		return CommitReceipt{}, err
	} else if replayed {
		receipt = prior.(CommitReceipt)
		receipt.Disposition = DispositionReplayed
		refs["disposition"] = string(receipt.Disposition)
		refs["newCommit"] = string(receipt.Result.NewCommit)
		return receipt, nil
	}
	tree, ok := snapshotpkg.TreeStoreOf(target)
	if !ok {
		return CommitReceipt{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not support raw path writes", cs.TargetRepository)
	}
	oldCommit, err := target.Head(cs.TargetRef)
	if err != nil {
		return CommitReceipt{}, err
	}
	if cs.Author == "" {
		cs.Author = w.author
	}
	if cs.RequestID == "" {
		cs.RequestID = w.requestID
	}
	if cs.RuleID == "" {
		cs.RuleID = w.ruleID
	}
	newCommit, err := tree.ApplyTreeCommit(cs)
	if err != nil {
		return CommitReceipt{}, err
	}
	receipt = CommitReceipt{
		ReceiptRef:  "receipt:" + string(knowledge.SurfaceTreeWrite) + ":" + string(newCommit),
		CommandID:   commandID,
		Surface:     string(knowledge.SurfaceTreeWrite),
		Disposition: DispositionApplied,
		Result: CommitResult{
			RepositoryID: cs.TargetRepository,
			CommitID:     newCommit,
			TargetRef:    cs.TargetRef,
			OldCommit:    oldCommit,
			NewCommit:    newCommit,
		},
	}
	if err := w.remember(commandID, digest, receipt, WriterRequest{Kind: string(knowledge.SurfaceTreeWrite), TreeChangeSet: &cs}); err != nil {
		return CommitReceipt{}, err
	}
	w.store.NotifyAdvanced(snapshotpkg.Advanced{
		Store: target,
		From:  oldCommit,
		To:    newCommit,
	})
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(newCommit)
	return receipt, nil
}
