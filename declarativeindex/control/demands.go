package control

import (
	"context"
	"sort"

	"kc/kernel"
)

// Only a complete successful refresh replaces the last verified partition.
// Explicitly empty inventory stays empty; durable queues never fill it in.
func (c *Controller) refreshDemands(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.lookupMu.RLock()
	inventory, serving := c.inventory, c.servingBases
	c.lookupMu.RUnlock()
	next := map[kernel.RepositoryID][]kernel.CommitID{}
	if inventory == nil {
		c.demands = next
		return nil
	}
	ids, err := inventory()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if id == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "index Repository inventory contains an empty identity")
		}
		if _, duplicate := next[id]; duplicate {
			continue
		}
		var bases []kernel.CommitID
		if serving != nil {
			bases, err = serving(id)
			if err != nil {
				return err
			}
		}
		seen := map[kernel.CommitID]bool{}
		fixed := []kernel.CommitID{}
		for _, basis := range bases {
			if basis == "" {
				return kernel.Fail(kernel.ErrVersionUnresolved, "index serving demand requires a fixed commit")
			}
			if !seen[basis] {
				seen[basis] = true
				fixed = append(fixed, basis)
			}
		}
		sort.Slice(fixed, func(i, j int) bool { return fixed[i] < fixed[j] })
		next[id] = fixed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.demands = next
	return nil
}

func (c *Controller) demandRepositories() []kernel.RepositoryID {
	ids := make([]kernel.RepositoryID, 0, len(c.demands))
	for id := range c.demands {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
