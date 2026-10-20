package control

import (
	"context"
	"errors"

	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type repositoryHead struct {
	repository objectmodel.Repository
	commit     kernel.CommitID
	failure    error
}

func (c *Controller) repository(ctx context.Context, id kernel.RepositoryID) (objectmodel.Repository, error) {
	if c.lookup == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller requires a repository lookup")
	}
	repo, err := c.lookup(id)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller has no object repository")
	}
	if repo.ID() != id {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "projection repository lookup changed source identity")
	}
	return objectmodel.BindContext(ctx, repo), nil
}

func (c *Controller) reconcile(ctx context.Context, only kernel.RepositoryID) (map[kernel.RepositoryID]repositoryHead, error) {
	heads := map[kernel.RepositoryID]repositoryHead{}
	targets, err := c.store.List()
	if err != nil {
		return heads, err
	}
	known := map[kernel.RepositoryID]index.ProjectionTarget{}
	for _, target := range targets {
		known[target.Repository] = target
	}
	var joined error
	for _, id := range c.demandRepositories(only) {
		if err := ctx.Err(); err != nil {
			return heads, errors.Join(joined, err)
		}
		repo, err := c.repository(ctx, id)
		var head kernel.CommitID
		if err == nil {
			head, err = repo.Head(snapshot.DefaultRef)
			if err == nil && head == "" {
				err = kernel.Fail(kernel.ErrVersionUnresolved, "projection controller requires a published HEAD")
			}
		}
		if err := ctx.Err(); err != nil {
			return heads, errors.Join(joined, err)
		}
		if err != nil {
			heads[id] = repositoryHead{repository: repo, failure: err}
			joined = errors.Join(joined, err, c.store.failHead(id, err))
			continue
		}
		heads[id] = repositoryHead{repository: repo, commit: head}
		needed := c.needsHead(repo, head, known[id])
		if err := ctx.Err(); err != nil {
			return heads, errors.Join(joined, err)
		}
		if needed {
			joined = errors.Join(joined, c.Desire(id, head))
		}
	}
	return heads, joined
}

func (c *Controller) needsHead(repo objectmodel.Repository, head kernel.CommitID, target index.ProjectionTarget) bool {
	if target.DesiredCommit != head || target.AppliedCommit != head || target.Status != index.TargetReady {
		return true
	}
	desc, err := c.index.Describe(repo)
	return err != nil || desc.BasisCommit != head || desc.State != index.ProjectionStateReady
}

func (c *Controller) applyPending(ctx context.Context, heads map[kernel.RepositoryID]repositoryHead) error {
	targets, err := c.store.List()
	if err != nil {
		return err
	}
	var joined error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return errors.Join(joined, err)
		}
		head, active := heads[target.Repository]
		if !active || head.failure != nil || target.DesiredCommit != head.commit {
			continue
		}
		if target.Status == index.TargetReady && target.AppliedCommit == head.commit {
			continue
		}
		result, applyErr := c.index.Ensure(ctx, head.repository, head.commit)
		if err := ctx.Err(); err != nil {
			// Shutdown interrupted this attempt; the durable target keeps its
			// pending status so the next start resumes it, not a failure record.
			return errors.Join(joined, err)
		}
		joined = errors.Join(joined, applyErr, c.store.finish(target.Repository, head.commit, result, applyErr))
	}
	return joined
}

func (c *Controller) catchUpFixed(ctx context.Context, only kernel.RepositoryID) error {
	var joined error
	for _, id := range c.demandRepositories(only) {
		if len(c.demands[id]) == 0 {
			continue
		}
		repo, err := c.repository(ctx, id)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		for _, basis := range c.demands[id] {
			if err := ctx.Err(); err != nil {
				return errors.Join(joined, err)
			}
			joined = errors.Join(joined, c.PrepareServingBasis(ctx, repo, basis))
		}
	}
	return joined
}
