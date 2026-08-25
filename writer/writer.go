package writer

import (
	"strings"

	"kc/internal/journal"
	"kc/kernel"
	"kc/repository"
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
	store       *repository.Store
	log         IdempotencyStore
	idempotency map[string]storedEntry
	journal     journal.Journal
	author      string
	requestID   string
	ruleID      string
}

func NewWriter(store *repository.Store, log IdempotencyStore) (*Writer, error) {
	w := &Writer{store: store, log: log, idempotency: map[string]storedEntry{}}
	if log != nil {
		entries, err := log.Load()
		if err != nil {
			return nil, err
		}
		w.LoadIdempotency(entries)
	}
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

func validateChangeSet(cs repository.CommitChangeSet) error {
	if cs.TargetRepository == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target repository")
	}
	if cs.TargetRef == "" {
		return kernel.Fail(kernel.ErrWriteTargetRequired, "write requires a target ref")
	}
	if len(cs.Operations) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "changeset has no operations")
	}
	if err := kernel.ValidateProvenance(cs.Provenance); err != nil {
		return err
	}
	for _, op := range cs.Operations {
		if err := kernel.AssertWritable(op.Address); err != nil {
			return err
		}
		if op.Op == repository.OpPut {
			if err := repository.ValidateValueSource(op.ValueSource); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) applySnapshot(commandID string, surface repository.Surface, cs repository.CommitChangeSet) (receipt CommitReceipt, err error) {
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
	if prior, replayed, err := w.conflictOrReplay(commandID, digest); err != nil {
		return CommitReceipt{}, err
	} else if replayed {
		receipt = prior.(CommitReceipt)
		receipt.Disposition = DispositionReplayed
		refs["disposition"] = string(receipt.Disposition)
		refs["newCommit"] = string(receipt.Result.NewCommit)
		return receipt, nil
	}
	if err := validateSchemaRefs(repo, cs); err != nil {
		return CommitReceipt{}, err
	}
	oldCommit, err := repo.Head(cs.TargetRef)
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
	commitID, err := repo.ApplyCommit(cs)
	if err != nil {
		return CommitReceipt{}, err
	}
	kind := string(surface)
	receipt = CommitReceipt{
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
	if err := w.remember(commandID, digest, receipt, WriterRequest{Kind: kind, ChangeSet: &cs}); err != nil {
		return CommitReceipt{}, err
	}
	if surface == repository.SurfaceCommit {
		w.store.NotifySnapshot(repository.Snapshot{
			Repository: repo,
			From:       oldCommit,
			To:         commitID,
			ObjectIDs:  repository.UniqueObjectIDs(cs.Operations),
		})
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["newCommit"] = string(commitID)
	return receipt, nil
}
