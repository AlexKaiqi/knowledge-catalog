package writer

import (
	"kc/kernel"
	"kc/repository"
)

// APPEND writes immutable stream entries (layer ⓪). JSONL beside FileGit/Dolt
// is packing; git HEAD does not move. Not a Snapshot method.

// AppendIntent is APPEND before the expected cursor is filled from the stream.
type AppendIntent struct {
	TargetRepository kernel.RepositoryID
	StreamRef        string
	ExpectedCursor   string
	Entries          []repository.AppendEntry
}

func (w *Writer) AppendIntent(commandID string, intent AppendIntent) (AppendReceipt, error) {
	if prior, ok := w.Lookup(commandID); ok && prior.Request.Kind == string(repository.SurfaceAppend) && prior.Request.Entries != nil {
		return w.Append(commandID, repository.AppendEntries{
			TargetRepository: intent.TargetRepository,
			StreamRef:        intent.StreamRef,
			ExpectedCursor:   prior.Request.Entries.ExpectedCursor,
			Entries:          intent.Entries,
		})
	}
	cursor := intent.ExpectedCursor
	if cursor == "" {
		repo, err := w.store.Require(intent.TargetRepository, kernel.ErrTargetRepositoryDenied)
		if err != nil {
			return AppendReceipt{}, err
		}
		if repo.Archived() {
			return AppendReceipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", intent.TargetRepository)
		}
		stream, err := w.store.RequireStream(intent.TargetRepository, kernel.ErrTargetRepositoryDenied)
		if err != nil {
			return AppendReceipt{}, err
		}
		cursor = stream.StreamCursor(intent.StreamRef)
	}
	return w.Append(commandID, repository.AppendEntries{
		TargetRepository: intent.TargetRepository,
		StreamRef:        intent.StreamRef,
		ExpectedCursor:   cursor,
		Entries:          intent.Entries,
	})
}

func (w *Writer) Append(commandID string, ae repository.AppendEntries) (receipt AppendReceipt, err error) {
	refs := map[string]any{"commandId": commandID, "repositoryId": string(ae.TargetRepository), "streamRef": ae.StreamRef}
	defer func() { err = w.note(string(repository.SurfaceAppend), refs, err) }()
	digest := string(kernel.CanonicalDigest(ae))
	if prior, replayed, err := w.conflictOrReplay(commandID, digest); err != nil {
		return AppendReceipt{}, err
	} else if replayed {
		receipt = prior.(AppendReceipt)
		receipt.Disposition = DispositionReplayed
		refs["disposition"] = string(receipt.Disposition)
		refs["cursor"] = receipt.Result.Cursor
		return receipt, nil
	}
	repo, err := w.store.Require(ae.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return AppendReceipt{}, err
	}
	if repo.Archived() {
		return AppendReceipt{}, kernel.Fail(kernel.ErrRepositoryArchived, "repository %s is archived", ae.TargetRepository)
	}
	stream, err := w.store.RequireStream(ae.TargetRepository, kernel.ErrTargetRepositoryDenied)
	if err != nil {
		return AppendReceipt{}, err
	}
	if err := validateAppendSchemaRefs(repo, ae.Entries); err != nil {
		return AppendReceipt{}, err
	}
	recordIDs, err := stream.Append(ae.StreamRef, ae.Entries, ae.ExpectedCursor)
	if err != nil {
		return AppendReceipt{}, err
	}
	receipt = AppendReceipt{
		ReceiptRef:  "receipt:append:" + commandID,
		CommandID:   commandID,
		Surface:     string(repository.SurfaceAppend),
		Disposition: DispositionApplied,
		Result: AppendResult{
			RepositoryID: ae.TargetRepository,
			StreamRef:    ae.StreamRef,
			Cursor:       stream.StreamCursor(ae.StreamRef),
			Appended:     recordIDs,
		},
	}
	if err := w.remember(commandID, digest, receipt, WriterRequest{Kind: string(repository.SurfaceAppend), Entries: &ae}); err != nil {
		return AppendReceipt{}, err
	}
	refs["disposition"] = string(receipt.Disposition)
	refs["cursor"] = receipt.Result.Cursor
	return receipt, nil
}
