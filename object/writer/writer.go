package writer

import (
	"encoding/json"
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// Writer is the write face: one target, one Surface, one command_id.
// COMMIT/PROPOSAL target a Snapshot (layer ⓪); ChangeSet PUT/REMOVE is layer ②.
//
// Operations, by Surface:
//
//	COMMIT:    CommitIntent, Commit
//	PROPOSAL:  Propose
//
// Preview (not a Surface): Ingest, Reconcile. Confirm with Commit.
type Writer struct {
	store         snapshot.Registry
	commands      snapshot.CommandLedger
	journal       journal.Journal
	author        string
	requestID     string
	ruleID        string
	systemPublish bool
}

func NewWriter(store snapshot.Registry, commands snapshot.CommandLedger) (*Writer, error) {
	if commands == nil {
		var err error
		commands, err = snapshot.NewCommandLedger(nil)
		if err != nil {
			return nil, err
		}
	}
	return &Writer{store: store, commands: commands}, nil
}

// NewSystemPublicationWriter is the bootstrap write authority supplied to the
// complete system publisher. Ordinary request writers remain immutable for
// System Repository; plugins never receive this constructor's Registry.
func NewSystemPublicationWriter(store snapshot.Registry) (*Writer, error) {
	w, err := NewWriter(store, nil)
	if err != nil {
		return nil, err
	}
	w.systemPublish = true
	return w, nil
}

func (w *Writer) SetJournal(j journal.Journal) { w.journal = j }

func (w *Writer) SetStamp(as, requestID, ruleID string) {
	w.author = as
	w.requestID = requestID
	w.ruleID = ruleID
}

func (w *Writer) Stamp() (as, requestID, ruleID string) {
	return w.author, w.requestID, w.ruleID
}

func (w *Writer) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(w.journal, journal.LayerSystem, "writer", cmd, refs, err)
}

func (w *Writer) refuseRuntimeSystemMutation(repo kernel.RepositoryID) error {
	if repo != objectmodel.SystemRepositoryID || w.systemPublish {
		return nil
	}
	return kernel.Fail(kernel.ErrForbidden, "System Repository is immutable")
}

func validateChangeSet(cs objectmodel.ChangeSet) error {
	if cs.TargetRepository == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target repository")
	}
	if cs.TargetRef == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target ref")
	}
	if len(cs.Operations) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "changeset has no operations")
	}
	if err := objectmodel.ValidateProvenance(cs.Provenance); err != nil {
		return err
	}
	for _, op := range cs.Operations {
		if err := objectmodel.AssertWritable(op.Address); err != nil {
			return err
		}
		if op.Op == objectmodel.OpPut {
			if err := objectmodel.ValidateValueSource(op.ValueSource); err != nil {
				return err
			}
			if op.ValueSource.Normalized() != nil {
				return kernel.Fail(kernel.ErrUsageInvalid, "Bound State is declared on Domain Schema origin, not an instance value_source")
			}
			if op.Address.Kind == objectmodel.KindRelation {
				if _, err := objectmodel.DecodeRelation(op.Address, op.Value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Writer) applySnapshot(commandID string, surface objectmodel.Surface, cs objectmodel.ChangeSet) (receipt CommitReceipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(cs.TargetRepository), "targetRef": cs.TargetRef}
	defer func() {
		operationErr := err
		evidenceErr := w.note(string(surface), refs, operationErr)
		if operationErr != nil {
			err = evidenceErr
		}
		// Once the ledger receipt and Canonical commit are accepted, a
		// secondary evidence sink cannot turn success into a false write
		// failure. Evidence health is monitored independently by its owner.
	}()
	if err := w.refuseRuntimeSystemMutation(cs.TargetRepository); err != nil {
		return CommitReceipt{}, err
	}
	if err := validateChangeSet(cs); err != nil {
		return CommitReceipt{}, err
	}
	repo, err := w.store.Require(cs.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return CommitReceipt{}, err
	}
	if repo.Archived() {
		return CommitReceipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", cs.TargetRepository)
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
	kind := string(surface)
	entry, replayed, err := w.commands.Execute(commandID, digest, snapshot.CommandRequest{
		Kind: kind, RepositoryID: string(cs.TargetRepository), TargetRef: cs.TargetRef,
		BaseCommit: string(cs.BaseCommit), ExpectedTargetCommit: string(cs.ExpectedTargetCommit),
		OperationCount: len(cs.Operations),
	}, func() (any, error) {
		if err := validateSchemaRefs(repo, cs); err != nil {
			return nil, snapshot.WriteNotApplied(err)
		}
		oldCommit, err := repo.Head(cs.TargetRef)
		if err != nil {
			return nil, snapshot.WriteNotApplied(err)
		}
		commitID, err := applyObjectCommit(commandID, repo, cs)
		if err != nil {
			return nil, err
		}
		applied := CommitReceipt{
			ReceiptRef:  "receipt:" + strings.ToLower(kind) + ":" + string(commitID),
			CommandID:   commandID,
			Surface:     kind,
			Disposition: DispositionApplied,
			Result: CommitResult{
				RepositoryID: cs.TargetRepository,
				CommitID:     commitID,
				TargetRef:    cs.TargetRef,
				OldCommit:    oldCommit,
				NewCommit:    commitID,
			},
		}
		if surface == objectmodel.SurfaceCommit {
			w.store.NotifyAdvanced(snapshot.Advanced{Store: repo, From: oldCommit, To: commitID})
		}
		return applied, nil
	})
	if err != nil {
		return CommitReceipt{}, err
	}
	if err := json.Unmarshal(entry.Receipt, &receipt); err != nil {
		return CommitReceipt{}, err
	}
	if replayed {
		receipt.Disposition = DispositionReplayed
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(receipt.Result.NewCommit)
	return receipt, nil
}
