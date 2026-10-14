package maintenance

import (
	"context"
	"errors"
	"kc/kernel"
	objectmodel "kc/object"
	"sync"
	"time"
)

// RepositoryLookup resolves a maintenance repository; request access remains separate.
type RepositoryLookup func(kernel.RepositoryID) (objectmodel.Repository, error)
type RepositoryInventory func() ([]kernel.RepositoryID, error)

const DefaultReconcileInterval = 15 * time.Second

// Coordinator schedules independent Snapshot derivatives. Durable progress is
// not query readiness and one lane cannot block another lane's recovery.
type Coordinator struct {
	store       *ProgressStore
	lookup      RepositoryLookup
	inventory   RepositoryInventory
	interval    time.Duration
	lookupMu    sync.RWMutex
	startMu     sync.Mutex
	started     bool
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	consumersMu sync.RWMutex
	consumers   []*consumerLane
}

func NewCoordinator(file string, lookup RepositoryLookup) (*Coordinator, error) {
	store := &ProgressStore{file: file}
	if err := store.ready(); err != nil {
		return nil, err
	}
	return &Coordinator{store: store, lookup: lookup, interval: DefaultReconcileInterval}, nil
}
func (c *Coordinator) SetInventory(inventory RepositoryInventory) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	c.inventory = inventory
}
func (c *Coordinator) SetReconcileInterval(interval time.Duration) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	if interval > 0 {
		c.interval = interval
	}
}
func (c *Coordinator) inventoryIDs() ([]kernel.RepositoryID, error) {
	c.lookupMu.RLock()
	inventory := c.inventory
	c.lookupMu.RUnlock()
	if inventory != nil {
		return inventory()
	}
	targets, err := c.ConsumerTargets()
	if err != nil {
		return nil, err
	}
	ids := []kernel.RepositoryID{}
	seen := map[kernel.RepositoryID]bool{}
	for _, target := range targets {
		if !seen[target.Repository] {
			seen[target.Repository] = true
			ids = append(ids, target.Repository)
		}
	}
	return ids, nil
}
func (c *Coordinator) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	var errs []error
	for _, lane := range c.snapshotConsumers() {
		errs = append(errs, c.store.desireConsumer(lane.id, repository, commit))
		lane.signal()
	}
	return errors.Join(errs...)
}
func (c *Coordinator) Start(parent context.Context) {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.started = true
	c.lookupMu.RLock()
	interval := c.interval
	c.lookupMu.RUnlock()
	for _, lane := range c.snapshotConsumers() {
		c.startWorker(ctx, interval, lane.wake, func(ctx context.Context) error { return c.catchUpConsumer(ctx, lane) })
	}
}
func (c *Coordinator) Close() {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.wg.Wait()
		c.cancel = nil
	}
}
func (c *Coordinator) CatchUp(ctx context.Context) error {
	lanes := c.snapshotConsumers()
	errs := make([]error, len(lanes))
	var wg sync.WaitGroup
	for n, lane := range lanes {
		wg.Add(1)
		go func() { defer wg.Done(); errs[n] = c.catchUpConsumer(ctx, lane) }()
	}
	wg.Wait()
	return errors.Join(errs...)
}
func (c *Coordinator) Reconcile(ctx context.Context) error {
	var errs []error
	for _, lane := range c.snapshotConsumers() {
		errs = append(errs, c.reconcileConsumer(ctx, lane))
	}
	return errors.Join(errs...)
}
func (c *Coordinator) startWorker(ctx context.Context, interval time.Duration, wake <-chan struct{}, run func(context.Context) error) {
	c.wg.Add(1)
	go func() { defer c.wg.Done(); RunWorker(ctx, interval, wake, run) }()
}

// RunWorker is the single startup/wakeup/periodic reconciliation loop used by
// specialized maintenance controllers and the independent consumer coordinator.
func RunWorker(ctx context.Context, interval time.Duration, wake <-chan struct{}, run func(context.Context) error) {
	_ = run(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			_ = run(ctx)
		case <-ticker.C:
			_ = run(ctx)
		}
	}
}
