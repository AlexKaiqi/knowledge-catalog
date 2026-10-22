package service

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"sort"
	"time"
)

func (e ObjectLogExecutor) executeBounded(ctx context.Context, request ObjectLogRequest) (ObjectLogPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Checkpoints == nil {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "durable history continuations are unavailable")
	}
	if request.Limit < 1 || request.Budget < 0 || request.Budget > objectmodel.MaxHistoryScanBudget {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid history limit or scan budget")
	}
	checkpoint := objectmodel.HistoryCheckpoint{Version: 1, Basis: request.Basis, Ref: request.Ref, Open: map[kernel.RepositoryID]*objectmodel.HistoryScanProgress{}}
	candidates := append([]objectmodel.ReadSource(nil), request.Basis.Candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Repository < candidates[j].Repository })
	for _, source := range request.Basis.Candidates {
		checkpoint.Open[source.Repository] = nil
	}
	if err := checkpoint.Validate(); err != nil {
		return ObjectLogPage{}, err
	}
	ref, _ := objectmodel.ParseReference(checkpoint.Basis.Reference)
	if ref.Object != request.Object {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "history basis names a different object")
	}
	if len(request.Pin) > 0 {
		if len(request.Pin) != len(candidates) {
			return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "history coordinates differ from the fixed basis")
		}
		for _, source := range candidates {
			if request.Pin[source.Repository] != source.Commit {
				return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "history coordinates differ from the fixed basis")
			}
		}
	}

	if request.Continuation != "" {
		stored, err := e.Checkpoints.Resolve(ctx, request.Continuation)
		if err != nil {
			return ObjectLogPage{}, err
		}
		if kernel.CanonicalDigest(stored.Basis) != kernel.CanonicalDigest(checkpoint.Basis) || stored.Ref != checkpoint.Ref {
			return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "history continuation does not match the fixed source scope")
		}
		checkpoint = stored
	}
	for _, source := range candidates {
		repository := source.Repository
		if err := ctx.Err(); err != nil {
			return ObjectLogPage{}, err
		}
		if e.Allow != nil && !e.Allow(repository, request.Object) {
			return ObjectLogPage{}, kernel.Fail(kernel.ErrForbidden, "history source access is no longer allowed")
		}
	}
	budget := request.Budget
	if budget == 0 {
		budget = objectmodel.DefaultHistoryScanBudget
	}
	totalScanned := 0
	complete := false
	page := ObjectLogPage{Logs: []reader.ObjectLog{}, Scanned: &totalScanned, Complete: &complete}
	deadline := time.Now().Add(objectmodel.DefaultHistoryScanDuration)
	ctx, cancel := context.WithTimeout(ctx, objectmodel.HistoryScanRequestTimeout)
	defer cancel()
	advanced := false
	for _, source := range candidates {
		repository := source.Repository
		progress, open := checkpoint.Open[repository]
		if !open {
			continue
		}
		if err := ctx.Err(); err != nil {
			return ObjectLogPage{}, err
		}
		if budget == 0 || time.Now().After(deadline) {
			break
		}
		duration := time.Until(deadline)
		if duration <= 0 {
			break
		}
		var input *objectmodel.HistoryScanProgress
		if progress != nil {
			copy := *progress
			input = &copy
		}
		scanned, err := e.ReadScan(ctx, repository, source.Commit, objectmodel.HistoryScanQuery{Limit: request.Limit, Budget: budget, TimeBudget: duration, Progress: input})
		if err != nil {
			return ObjectLogPage{}, err
		}
		if err := validateHistoryScanResult(scanned, progress, budget, request.Limit, request.Object, source); err != nil {
			return ObjectLogPage{}, err
		}
		advanced = true
		totalScanned += scanned.Scanned
		work := scanned.Scanned
		if work == 0 {
			work = 1
		}
		budget -= work
		if scanned.Complete {
			delete(checkpoint.Open, repository)
		} else {
			checkpoint.Open[repository] = scanned.Progress
		}
		if len(scanned.Revisions) > 0 {
			page.Logs = append(page.Logs, reader.ObjectLog{Repository: repository, ObjectID: request.Object, Commit: source.Commit, Revisions: scanned.Revisions})
		}
	}
	if !advanced {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "history page did not advance within its scan budget")
	}
	complete = len(checkpoint.Open) == 0
	if !complete {
		reference, err := e.Checkpoints.Retain(ctx, checkpoint)
		if err != nil {
			return ObjectLogPage{}, err
		}
		if reference == request.Continuation {
			return ObjectLogPage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "history continuation did not advance")
		}
		page.Continuation = reference
	}
	return page, nil
}

func validateHistoryScanResult(page objectmodel.HistoryScanPage, previous *objectmodel.HistoryScanProgress, budget, limit int, id objectmodel.ObjectID, source objectmodel.ReadSource) error {
	if page.Scanned < 0 || page.Scanned > budget || len(page.Revisions) > limit || len(page.Revisions) > page.Scanned+1 || page.Complete && page.Progress != nil || !page.Complete && (page.Progress == nil || page.Scanned == 0 || previous != nil && page.Progress.Position == previous.Position) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "history provider returned invalid or non-advancing progress")
	}
	if page.Progress != nil {
		if err := page.Progress.Validate(id, source, ""); err != nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "history provider returned progress outside its fixed source")
		}
	}
	seen := map[kernel.CommitID]bool{}
	for _, revision := range page.Revisions {
		if revision.Commit == "" || seen[revision.Commit] || revision.Status != objectmodel.StatusResolved && revision.Status != objectmodel.StatusRemoved {
			return kernel.Fail(kernel.ErrPreconditionFailed, "history provider returned an invalid revision")
		}
		seen[revision.Commit] = true
	}
	return nil
}
