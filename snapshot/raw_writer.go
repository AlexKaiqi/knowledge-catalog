// Package treewriter is the layer ⓪ write service for literal path bytes.
// It knows TreeChangeSet, CAS, command replay and Snapshot advancement, but no
// object_id, Address, Aspect, Schema or other Knowledge semantics.
package snapshot

import (
	"encoding/json"

	"kc/kernel"
)

type rawWriter struct {
	store     Registry
	commands  CommandLedger
	observer  WriteObserver
	author    string
	requestID string
	ruleID    string
}

func NewRawWriter(store Registry, commands CommandLedger) (RawWriter, error) {
	if commands == nil {
		var err error
		commands, err = NewCommandLedger(nil)
		if err != nil {
			return nil, err
		}
	}
	return &rawWriter{store: store, commands: commands}, nil
}

func (w *rawWriter) SetObserver(observer WriteObserver) { w.observer = observer }

func (w *rawWriter) SetStamp(as, requestID, ruleID string) {
	w.author = as
	w.requestID = requestID
	w.ruleID = ruleID
}

func (w *rawWriter) Lookup(commandID string) (TreeChangeSet, Receipt, bool) {
	entry, ok := w.commands.Lookup(commandID)
	if !ok || entry.Request.Kind != RawWriteSurface || len(entry.Request.TreeChangeSet) == 0 {
		return TreeChangeSet{}, Receipt{}, false
	}
	var cs TreeChangeSet
	var receipt Receipt
	if json.Unmarshal(entry.Request.TreeChangeSet, &cs) != nil || json.Unmarshal(entry.Receipt, &receipt) != nil {
		return TreeChangeSet{}, Receipt{}, false
	}
	return cs, receipt, true
}

func (w *rawWriter) Commit(commandID string, cs TreeChangeSet) (receipt Receipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(cs.TargetRepository), "targetRef": cs.TargetRef}
	defer func() {
		if w.observer != nil {
			err = w.observer(RawWriteSurface, refs, err)
		}
	}()
	if cs.TargetRepository == "" {
		return Receipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "raw write requires a target repository")
	}
	if cs.TargetRef == "" {
		return Receipt{}, kernel.Fail(kernel.ErrWriteTargetRequired, "raw write requires a target ref")
	}
	if len(cs.Changes) == 0 {
		return Receipt{}, kernel.Fail(kernel.ErrUsageInvalid, "raw changeset has no changes")
	}
	target, err := w.store.Require(cs.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return Receipt{}, err
	}
	if target.Archived() {
		return Receipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", cs.TargetRepository)
	}
	digest := string(kernel.CanonicalDigest(cs))
	if cs.Author == "" {
		cs.Author = w.author
	}
	if cs.RequestID == "" {
		cs.RequestID = w.requestID
	}
	if cs.RuleID == "" {
		cs.RuleID = w.ruleID
	}
	rawRequest, err := json.Marshal(cs)
	if err != nil {
		return Receipt{}, err
	}
	entry, replayed, err := w.commands.Execute(commandID, digest, CommandRequest{
		Kind: RawWriteSurface, RepositoryID: string(cs.TargetRepository), TargetRef: cs.TargetRef,
		BaseCommit: string(cs.BaseCommit), ExpectedTargetCommit: string(cs.ExpectedTargetCommit),
		OperationCount: len(cs.Changes), TreeChangeSet: rawRequest,
	}, func() (any, error) {
		tree, ok := TreeStoreOf(target)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"repository %s does not support raw path writes", cs.TargetRepository)
		}
		oldCommit, err := target.Head(cs.TargetRef)
		if err != nil {
			return nil, err
		}
		newCommit, err := tree.ApplyTreeCommit(cs)
		if err != nil {
			return nil, err
		}
		applied := Receipt{
			ReceiptRef:  "receipt:raw_write:" + string(newCommit),
			CommandID:   commandID,
			Surface:     RawWriteSurface,
			Disposition: ReceiptApplied,
			Result: ReceiptResult{
				RepositoryID: cs.TargetRepository,
				CommitID:     newCommit,
				TargetRef:    cs.TargetRef,
				OldCommit:    oldCommit,
				NewCommit:    newCommit,
			},
		}
		w.store.NotifyAdvanced(Advanced{Store: target, From: oldCommit, To: newCommit})
		return applied, nil
	})
	if err != nil {
		return Receipt{}, err
	}
	if err := json.Unmarshal(entry.Receipt, &receipt); err != nil {
		return Receipt{}, err
	}
	if replayed {
		receipt.Disposition = ReceiptReplayed
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(receipt.Result.NewCommit)
	return receipt, nil
}
