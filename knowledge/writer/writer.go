package writer

import (
	"encoding/json"
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	"kc/snapshot/commandlog"
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
	store     *snapshot.Registry
	commands  *commandlog.Ledger
	journal   journal.Journal
	author    string
	requestID string
	ruleID    string
}

func NewWriter(store *snapshot.Registry, commands *commandlog.Ledger) (*Writer, error) {
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

func (w *Writer) Stamp() (as, requestID, ruleID string) {
	return w.author, w.requestID, w.ruleID
}

func (w *Writer) note(cmd string, refs map[string]any, err error) error {
	return journal.Finish(w.journal, journal.LayerSystem, "writer", cmd, refs, err)
}

func validateChangeSet(cs knowledge.ChangeSet) error {
	if cs.TargetRepository == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target repository")
	}
	if cs.TargetRef == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target ref")
	}
	if len(cs.Operations) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "changeset has no operations")
	}
	if err := knowledge.ValidateProvenance(cs.Provenance); err != nil {
		return err
	}
	for _, op := range cs.Operations {
		if err := knowledge.AssertWritable(op.Address); err != nil {
			return err
		}
		if op.Op == knowledge.OpPut {
			if err := knowledge.ValidateValueSource(op.ValueSource); err != nil {
				return err
			}
			if op.Address.Kind == knowledge.KindRelation {
				if _, err := knowledge.DecodeRelation(op.Address, op.Value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Writer) applySnapshot(commandID string, surface knowledge.Surface, cs knowledge.ChangeSet) (receipt CommitReceipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(cs.TargetRepository), "targetRef": cs.TargetRef}
	defer func() { err = w.note(string(surface), refs, err) }()
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
	rawRequest, err := json.Marshal(cs)
	if err != nil {
		return CommitReceipt{}, err
	}
	kind := string(surface)
	entry, replayed, err := w.commands.Execute(commandID, digest, commandlog.Request{
		Kind: kind, ChangeSet: rawRequest,
	}, func() (any, error) {
		if err := validateSchemaRefs(repo, cs); err != nil {
			return nil, err
		}
		oldCommit, err := repo.Head(cs.TargetRef)
		if err != nil {
			return nil, err
		}
		commitID, err := applyKnowledgeCommit(repo, cs)
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
		if surface == knowledge.SurfaceCommit {
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
