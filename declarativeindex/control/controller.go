package control

import (
	"context"
	"errors"
	"sync"
	"time"

	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/maintenance"
)

// MaintenanceIndex is the complete index port consumed by the Controller.
// Query execution and physical provider ownership remain outside control.
type MaintenanceIndex interface {
	Ensure(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error)
	EnsureAt(context.Context, objectmodel.Repository, kernel.CommitID) (index.IndexSync, error)
	Describe(objectmodel.Repository) (index.IndexDescriptor, error)
	HasStateBindings(objectmodel.Repository, kernel.CommitID) (bool, error)
	StateProjectionReady(context.Context, objectmodel.Repository, kernel.CommitID) (bool, error)
	RefreshState(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource) (index.StateSync, error)
	RefreshStateObjects(context.Context, objectmodel.Repository, kernel.CommitID, index.ObservationSource, []objectmodel.ObjectID) (index.StateSync, error)
}

type RepositoryLookup func(kernel.RepositoryID) (objectmodel.Repository, error)
type RepositoryInventory func() ([]kernel.RepositoryID, error)

const DefaultReconcileInterval = 15 * time.Second

// Controller reconciles the instance's current Repository inventory and
// accepted fixed serving bases. Its durable queue records work, never demand.
// Object's independent Snapshot consumers are not instantiated here.
type Controller struct {
	index             MaintenanceIndex
	store             *TargetStore
	lookup            RepositoryLookup
	inventory         RepositoryInventory
	servingBases      func(kernel.RepositoryID) ([]kernel.CommitID, error)
	demands           map[kernel.RepositoryID][]kernel.CommitID
	interval          time.Duration
	wake              chan struct{}
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	startMu           sync.Mutex
	catchupMu         sync.Mutex
	lookupMu          sync.RWMutex
	observationSource index.ObservationSource
}

func NewController(idx MaintenanceIndex, store *TargetStore, lookup RepositoryLookup) (*Controller, error) {
	if err := store.ready(); err != nil {
		return nil, err
	}
	return &Controller{index: idx, store: store, lookup: lookup, interval: DefaultReconcileInterval, wake: make(chan struct{}, 1)}, nil
}

func (c *Controller) SetInventory(inventory RepositoryInventory) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	c.inventory = inventory
}

// SetServingBases supplies only accepted active Dataset dependencies for one
// configured Repository. A failed refresh retains the previous complete view.
func (c *Controller) SetServingBases(bases func(kernel.RepositoryID) ([]kernel.CommitID, error)) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	c.servingBases = bases
}

func (c *Controller) SetObservationSource(source index.ObservationSource) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	c.observationSource = source
}

func (c *Controller) stateRuntime() index.ObservationSource {
	c.lookupMu.RLock()
	defer c.lookupMu.RUnlock()
	return c.observationSource
}

func (c *Controller) SetReconcileInterval(interval time.Duration) {
	c.lookupMu.Lock()
	defer c.lookupMu.Unlock()
	if interval > 0 {
		c.interval = interval
	}
}

// Desire is a durable hint only. It does no authority or provider I/O on the
// Writer receipt path, and cannot authorize a later historical build.
func (c *Controller) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	err := c.store.Desire(repository, commit)
	c.signal()
	return err
}

func (c *Controller) Notify(notice index.ChangeNotice) error {
	if err := index.ValidateChangeNotice(notice); err != nil {
		return err
	}
	if c.stateRuntime() == nil || c.index == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic State projection requires a configured index and Materialization Runtime")
	}
	if err := c.store.enqueueNotice(notice); err != nil {
		return err
	}
	c.signal()
	return nil
}

func (c *Controller) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Start validates the complete initial demand view before activating a worker.
// Physical readiness remains asynchronous and failures remain observable.
func (c *Controller) Start(parent context.Context) error {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.cancel != nil {
		return nil
	}
	c.catchupMu.Lock()
	err := c.refreshDemands(parent)
	c.catchupMu.Unlock()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.lookupMu.RLock()
	interval := c.interval
	c.lookupMu.RUnlock()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		maintenance.RunWorker(ctx, interval, c.wake, c.CatchUp)
	}()
	return nil
}

func (c *Controller) Close() {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.wg.Wait()
		c.cancel = nil
	}
}

func (c *Controller) CatchUp(ctx context.Context) error { return c.catchUp(ctx, "") }

// CatchUpRepository brings one source of the instance current. Other sources
// the same instance interprets keep their own lanes and are not touched.
func (c *Controller) CatchUpRepository(ctx context.Context, repository kernel.RepositoryID) error {
	if repository == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "scoped catch-up requires a Repository")
	}
	return c.catchUp(ctx, repository)
}

func (c *Controller) catchUp(ctx context.Context, only kernel.RepositoryID) error {
	c.catchupMu.Lock()
	defer c.catchupMu.Unlock()
	refreshErr := c.refreshDemands(ctx)
	if c.demands == nil || c.index == nil {
		return refreshErr
	}
	heads, reconcileErr := c.reconcile(ctx, only)
	return errors.Join(refreshErr, reconcileErr, c.applyPending(ctx, heads), c.catchUpFixed(ctx, only), c.catchUpState(ctx, heads, only))
}

// Reconcile discovers current HEAD demand without building physical state.
func (c *Controller) Reconcile(ctx context.Context) error {
	c.catchupMu.Lock()
	defer c.catchupMu.Unlock()
	refreshErr := c.refreshDemands(ctx)
	if c.demands == nil || c.index == nil {
		return refreshErr
	}
	_, reconcileErr := c.reconcile(ctx, "")
	return errors.Join(refreshErr, reconcileErr)
}

func (c *Controller) Targets() ([]index.ProjectionTarget, error) { return c.store.List() }
