package cache

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
)

// Warm refreshes a bounded set of retained hot object/address reads at the exact
// commit supplied by the maintenance controller. Successful completion means
// only that this small work set was attempted; it never asserts whole-repository
// readiness. Cancellation is checked between synchronous authority calls.
func (c *Cache) Warm(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRequest(repo, commit); err != nil {
		return err
	}
	c.warmMu.Lock()
	defer c.warmMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.stats.Closed {
		c.mu.Unlock()
		return nil
	}
	c.stats.WarmRuns++
	epoch := c.epoch
	k := warmKey{repository: repo.ID(), commit: commit}
	ids := make([]knowledge.ObjectID, 0, c.config.WarmLimit)
	addresses := make([]knowledge.Address, 0)
	seen := map[key]bool{}
	for e := c.lru.Front(); e != nil && len(ids)+len(addresses) < c.config.WarmLimit; e = e.Next() {
		item := e.Value.(entry)
		identity := item.key
		identity.commit = commit
		if item.key.repository == k.repository && !seen[identity] {
			seen[identity] = true
			if item.key.object {
				ids = append(ids, item.key.address.ObjectID)
			} else {
				addresses = append(addresses, item.key.address)
			}
		}
	}
	_, warmed := c.warmed[k]
	c.mu.Unlock()
	defer func() {
		if err != nil {
			c.mu.Lock()
			c.stats.WarmFailures++
			c.mu.Unlock()
		}
	}()
	cold := len(ids)+len(addresses) == 0 && c.config.ColdStart && !warmed
	if cold {
		pager, ok := repo.(knowledge.SnapshotObjectPager)
		if !ok {
			return nil
		}
		limit := min(c.config.WarmLimit, 1000)
		page, pageErr := pager.ObjectIDsPage(commit, limit, "")
		if pageErr != nil {
			return pageErr
		}
		if len(page.ObjectIDs) > limit {
			return kernel.Fail(kernel.ErrPreconditionFailed, "cache warm identity page exceeded its requested bound")
		}
		for _, id := range page.ObjectIDs {
			identity := objectKey(k.repository, commit, id)
			if id == "" || seen[identity] {
				continue
			}
			seen[identity] = true
			ids = append(ids, id)
		}
	}
	for start := 0; start < len(ids); start += c.config.WarmBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		interrupted := c.stats.Closed || c.epoch != epoch
		c.mu.Unlock()
		if interrupted {
			return nil
		}
		batch := ids[start:min(start+c.config.WarmBatchSize, len(ids))]
		values, readErr := c.readMany(repo, commit, batch, epoch)
		if readErr != nil {
			return readErr
		}
		c.mu.Lock()
		c.stats.WarmObjects += uint64(len(values))
		c.mu.Unlock()
	}
	for _, address := range addresses {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		interrupted := c.stats.Closed || c.epoch != epoch
		c.mu.Unlock()
		if interrupted {
			return nil
		}
		_, readErr := c.readAddress(repo, commit, address, epoch)
		if kernel.CodeOf(readErr) == kernel.ErrKnowledgeRefUnresolved {
			continue
		}
		if readErr != nil {
			return readErr
		}
		c.mu.Lock()
		c.stats.WarmObjects++
		c.mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if cold {
		c.markWarm(k, epoch)
	}
	return nil
}

func (c *Cache) markWarm(k warmKey, epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stats.Closed || c.epoch != epoch {
		return
	}
	// Resident bodies already suppress cold scans. Do not leave a marker that
	// would prevent repopulating this basis after ordinary capacity eviction.
	for bodyKey := range c.entries {
		if bodyKey.repository == k.repository {
			return
		}
	}
	if e, ok := c.warmed[k]; ok {
		c.warmLRU.MoveToFront(e)
		return
	}
	weight := int64(len(k.repository) + len(k.commit) + 128)
	if weight > c.config.MaxBytes {
		return
	}
	for (c.warmLRU.Len() >= min(c.config.MaxEntries, 1024) || c.stats.Bytes+weight > c.config.MaxBytes) && c.warmLRU.Len() > 0 {
		c.evictMarker()
	}
	// Warm bookkeeping must not evict useful bodies just to remember completion.
	if c.stats.Bytes+weight > c.config.MaxBytes {
		return
	}
	c.warmed[k] = c.warmLRU.PushFront(warmMarker{key: k, bytes: weight})
	c.stats.Bytes += weight
	c.stats.WarmMarkers = len(c.warmed)
}

func (c *Cache) evictMarker() {
	e := c.warmLRU.Back()
	marker := e.Value.(warmMarker)
	delete(c.warmed, marker.key)
	c.warmLRU.Remove(e)
	c.stats.Bytes -= marker.bytes
	c.stats.WarmMarkers = len(c.warmed)
}
