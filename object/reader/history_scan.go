package reader

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"time"
)

func (r *treeRepository) ScanHistory(ctx context.Context, id objectmodel.ObjectID, start kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, objectmodel.HistoryScanRequestTimeout)
	defer cancel()
	return r.WithContext(ctx).(*treeRepository).scanHistory(ctx, id, start, query)
}

func (r *treeRepository) scanHistory(ctx context.Context, id objectmodel.ObjectID, start kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {

	if err := ctx.Err(); err != nil {
		return objectmodel.HistoryScanPage{}, err
	}
	if query.Limit < 1 || query.Budget < 1 || query.Budget > objectmodel.MaxHistoryScanBudget || start == "" || query.TimeBudget < 0 || query.TimeBudget > objectmodel.MaxHistoryScanDuration {
		return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrUsageInvalid, "history requires a fixed commit, positive limit and bounded scan budget")
	}
	history, ok := snapshot.HistoryStoreOf(r.base)
	if !ok {
		return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no commit history", r.ID())
	}
	progress := objectmodel.HistoryScanProgress{Algorithm: objectmodel.LocalStateAlgorithm, Object: id, Repository: r.ID(), Binding: r.StoreDigest(), Start: start, After: query.After, Skipping: query.After != ""}
	if query.Progress != nil {
		progress = *query.Progress
		if err := progress.Validate(id, objectmodel.ReadSource{Repository: r.ID(), Commit: start, Binding: r.StoreDigest()}, query.After); err != nil {
			return objectmodel.HistoryScanPage{}, err
		}
	}
	page := objectmodel.HistoryScanPage{Revisions: []objectmodel.ObjectRevision{}}
	duration := query.TimeBudget
	if duration == 0 {
		duration = objectmodel.DefaultHistoryScanDuration
	}
	deadline := time.Now().Add(duration)
	yield := func() (objectmodel.HistoryScanPage, error) {
		if page.Scanned == 0 {
			return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "history scan made no progress within its time budget")
		}
		page.Progress = &progress
		return page, nil
	}
	anchor := start
	overlap := false
	if progress.Position != "" {
		anchor = progress.Position
		overlap = true
	}
	emit := func(revision objectmodel.ObjectRevision) {
		if progress.Skipping {
			if revision.Commit == progress.After {
				progress.Skipping = false
			}
			return
		}
		page.Revisions = append(page.Revisions, revision)
	}
	for page.Scanned < query.Budget {
		if err := ctx.Err(); err != nil {
			return objectmodel.HistoryScanPage{}, err
		}
		if time.Now().After(deadline) {
			return yield()
		}
		size := query.Budget - page.Scanned + 1
		if size > commitHistoryPageSize {
			size = commitHistoryPageSize
		}
		if size < 2 {
			size = 2
		}
		commits, err := history.CommitHistory(anchor, size)
		if err != nil {
			return objectmodel.HistoryScanPage{}, err
		}
		if err := validateHistoryWindow(commits, anchor, size); err != nil {
			return objectmodel.HistoryScanPage{}, err
		}
		offset := 0
		if overlap {
			offset = 1
		}
		for _, at := range commits[offset:] {
			if page.Scanned == query.Budget {
				break
			}
			if err := ctx.Err(); err != nil {
				return objectmodel.HistoryScanPage{}, err
			}
			if time.Now().After(deadline) {
				return yield()
			}
			revision, state, err := r.objectRevisionAt(id, at)
			if err != nil {
				return objectmodel.HistoryScanPage{}, err
			}
			if err := ctx.Err(); err != nil {
				return objectmodel.HistoryScanPage{}, err
			}
			if progress.Position != "" && (progress.Pending.Status != revision.Status || progress.State != state) {
				emit(progress.Pending)
			}
			progress.Position, progress.Pending, progress.State = at, revision, state
			page.Scanned++
			if len(page.Revisions) >= query.Limit {
				page.Progress = &progress
				return page, nil
			}
		}
		if len(commits) == 1 {
			if progress.Pending.Status == objectmodel.StatusResolved {
				emit(progress.Pending)
			}
			page.Complete = true
			return page, nil
		}
		anchor = progress.Position
		overlap = true
	}
	page.Progress = &progress
	return page, nil
}

func validateHistoryWindow(commits []kernel.CommitID, anchor kernel.CommitID, limit int) error {
	if len(commits) == 0 || len(commits) > limit || commits[0] != anchor {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "commit history did not return its requested basis")
	}
	seen := map[kernel.CommitID]bool{}
	for _, at := range commits {
		if at == "" || seen[at] {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "commit history returned an invalid chain")
		}
		seen[at] = true
	}
	return nil
}

func (r *Reader) ScanHistory(ctx context.Context, repository kernel.RepositoryID, id objectmodel.ObjectID, at kernel.CommitID, query objectmodel.HistoryScanQuery) (objectmodel.HistoryScanPage, error) {
	repo, err := r.Require(repository, kernel.ErrObjectRefUnresolved)
	if err != nil {
		return objectmodel.HistoryScanPage{}, err
	}
	scanner, ok := repo.(objectmodel.HistoryScanner)
	if !ok {
		return objectmodel.HistoryScanPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s has no bounded object history", repository)
	}
	return scanner.ScanHistory(ctx, id, at, query)
}
