package control

import (
	"context"
	"errors"
	"strings"

	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func (c *Controller) stateBases(id kernel.RepositoryID, heads map[kernel.RepositoryID]repositoryHead) []kernel.CommitID {
	seen := map[kernel.CommitID]bool{}
	bases := []kernel.CommitID{}
	if head, ok := heads[id]; ok && head.failure == nil {
		seen[head.commit] = true
		bases = append(bases, head.commit)
	}
	for _, basis := range c.demands[id] {
		if !seen[basis] {
			seen[basis] = true
			bases = append(bases, basis)
		}
	}
	return bases
}

func (c *Controller) catchUpState(ctx context.Context, heads map[kernel.RepositoryID]repositoryHead) error {
	source := c.stateRuntime()
	if source == nil || c.lookup == nil {
		return nil
	}
	ctx = workerObservationContext{ctx}
	// Apply queued changes before cold recovery: a cold notice already pulls
	// the full declaration basis, so the readiness pass need not observe twice.
	joined := c.applyStateNotices(ctx, heads, source)
	for _, id := range c.demandRepositories() {
		repo, err := c.repository(id)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		for _, basis := range c.stateBases(id, heads) {
			if err := ctx.Err(); err != nil {
				return errors.Join(joined, err)
			}
			has, err := c.index.HasStateBindings(repo, basis)
			if err != nil {
				joined = errors.Join(joined, err)
				continue
			}
			if !has {
				continue
			}
			ready, err := c.index.StateProjectionReady(ctx, repo, basis)
			if err != nil {
				joined = errors.Join(joined, err)
				continue
			}
			if ready {
				continue
			}
			_, err = c.index.RefreshState(ctx, repo, basis, source)
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (c *Controller) applyStateNotices(ctx context.Context, heads map[kernel.RepositoryID]repositoryHead, source index.ObservationSource) error {
	notices, err := c.store.pendingNotices()
	if err != nil {
		return err
	}
	var joined error
	for _, record := range notices {
		notice := record.ChangeNotice
		if err := ctx.Err(); err != nil {
			return errors.Join(joined, err)
		}
		if _, active := c.demands[notice.Repository]; !active {
			joined = errors.Join(joined, c.store.finishNotice(record, nil))
			continue
		}
		head, known := heads[notice.Repository]
		repo, lookupErr := c.repository(notice.Repository)
		applyErr := errors.Join(head.failure, lookupErr)
		if !known {
			applyErr = errors.Join(applyErr, kernel.Fail(kernel.ErrPreconditionFailed, "State notice HEAD authority has not been resolved"))
		}
		// A failed HEAD lane keeps the durable notice pending, even when an
		// independently readable fixed declaration can be refreshed now.
		if lookupErr == nil {
			for _, basis := range c.stateBases(notice.Repository, heads) {
				has, err := c.index.HasStateBindings(repo, basis)
				if err != nil {
					applyErr = errors.Join(applyErr, err)
					continue
				}
				if has {
					_, err = c.refreshStateBasis(ctx, repo, basis, notice, source)
					applyErr = errors.Join(applyErr, err)
				}
			}
		}
		joined = errors.Join(joined, applyErr, c.store.finishNotice(record, applyErr))
	}
	return joined
}

func (c *Controller) refreshStateBasis(ctx context.Context, repo objectmodel.Repository, basis kernel.CommitID, notice index.ChangeNotice, source index.ObservationSource) (index.StateSync, error) {
	if notice.Address == nil || strings.TrimSpace(string(notice.Address.ObjectID)) == "" {
		return c.index.RefreshState(ctx, repo, basis, source)
	}
	return c.index.RefreshStateObjects(ctx, repo, basis, source, []objectmodel.ObjectID{notice.Address.ObjectID})
}

// Explicit request-local notice keeps its selected ref. Background notices
// instead refresh every effective declaration basis through refreshStateBasis.
func (c *Controller) refreshNotice(ctx context.Context, notice index.ChangeNotice, source index.ObservationSource) (index.StateSync, error) {
	repo, err := c.repository(notice.Repository)
	if err != nil {
		return index.StateSync{}, err
	}
	head, err := repo.Head(snapshot.RefOrDefault(notice.Ref))
	if err != nil {
		return index.StateSync{}, err
	}
	return c.refreshStateBasis(ctx, repo, head, notice, source)
}
