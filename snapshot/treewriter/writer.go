// Package treewriter is the layer ⓪ write service for literal path bytes.
// It knows TreeChangeSet, CAS, command replay and Snapshot advancement, but no
// object_id, Address, Aspect, Schema or other Knowledge semantics.
package treewriter

import (
	"encoding/json"

	"kc/internal/journal"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/commandlog"
)

const Surface = "RAW_WRITE"

type Writer struct {
	store     *snapshot.Registry
	commands  *commandlog.Ledger
	journal   journal.Journal
	author    string
	requestID string
	ruleID    string
}

func New(store *snapshot.Registry, commands *commandlog.Ledger) (*Writer, error) {
	if commands == nil {
		var err error
		commands, err = commandlog.New(nil)
		if err != nil {
			return nil, err
		}
	}
	return &Writer{store: store, commands: commands}, nil
}

func (w *Writer) SetJournal(j journal.Journal) { w.journal = j }

func (w *Writer) SetStamp(as, requestID, ruleID string) {
	w.author = as
	w.requestID = requestID
	w.ruleID = ruleID
}

func (w *Writer) Lookup(commandID string) (snapshot.TreeChangeSet, Receipt, bool) {
	entry, ok := w.commands.Lookup(commandID)
	if !ok || entry.Request.Kind != Surface || len(entry.Request.TreeChangeSet) == 0 {
		return snapshot.TreeChangeSet{}, Receipt{}, false
	}
	var cs snapshot.TreeChangeSet
	var receipt Receipt
	if json.Unmarshal(entry.Request.TreeChangeSet, &cs) != nil || json.Unmarshal(entry.Receipt, &receipt) != nil {
		return snapshot.TreeChangeSet{}, Receipt{}, false
	}
	return cs, receipt, true
}

func (w *Writer) Commit(commandID string, cs snapshot.TreeChangeSet) (receipt Receipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(cs.TargetRepository), "targetRef": cs.TargetRef}
	defer func() { err = journal.Finish(w.journal, journal.LayerSystem, "treewriter", Surface, refs, err) }()
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
	entry, replayed, err := w.commands.Execute(commandID, digest, commandlog.Request{
		Kind: Surface, TreeChangeSet: rawRequest,
	}, func() (any, error) {
		tree, ok := snapshot.TreeStoreOf(target)
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
			Surface:     Surface,
			Disposition: DispositionApplied,
			Result: Result{
				RepositoryID: cs.TargetRepository,
				CommitID:     newCommit,
				TargetRef:    cs.TargetRef,
				OldCommit:    oldCommit,
				NewCommit:    newCommit,
			},
		}
		w.store.NotifyAdvanced(snapshot.Advanced{Store: target, From: oldCommit, To: newCommit})
		return applied, nil
	})
	if err != nil {
		return Receipt{}, err
	}
	if err := json.Unmarshal(entry.Receipt, &receipt); err != nil {
		return Receipt{}, err
	}
	if replayed {
		receipt.Disposition = DispositionReplayed
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(receipt.Result.NewCommit)
	return receipt, nil
}
