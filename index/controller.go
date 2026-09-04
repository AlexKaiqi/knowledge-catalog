package index

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/snapshot"
)

var projectionTargetBucket = []byte("targets")
var stateNoticeBucket = []byte("state-notices")

type ProjectionTarget struct {
	Repository    kernel.RepositoryID `json:"repository"`
	DesiredCommit kernel.CommitID     `json:"desiredCommit"`
	AppliedCommit kernel.CommitID     `json:"appliedCommit,omitempty"`
	Status        string              `json:"status"`
	LastError     string              `json:"lastError,omitempty"`
	UpdatedAt     string              `json:"updatedAt"`
}

const (
	TargetPending = "PENDING"
	TargetReady   = "READY"
	TargetFailed  = "FAILED"
)

type TargetStore struct{ file string }

func NewTargetStore(file string) *TargetStore { return &TargetStore{file: file} }

func (s *TargetStore) ready() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return s.update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(projectionTargetBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(stateNoticeBucket)
		return err
	})
}

func (s *TargetStore) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(projectionTargetBucket)
		if err != nil {
			return err
		}
		target := ProjectionTarget{Repository: repository}
		if raw := bucket.Get([]byte(repository)); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
		}
		target.DesiredCommit = commit
		target.Status = TargetPending
		target.LastError = ""
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), raw)
	})
}

func (s *TargetStore) List() ([]ProjectionTarget, error) {
	targets := []ProjectionTarget{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var target ProjectionTarget
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			targets = append(targets, target)
			return nil
		})
	})
	return targets, err
}

func (s *TargetStore) finish(repository kernel.RepositoryID, attempted kernel.CommitID, syncResult IndexSync, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(projectionTargetBucket)
		if bucket == nil {
			return nil
		}
		raw := bucket.Get([]byte(repository))
		if raw == nil {
			return nil
		}
		var target ProjectionTarget
		if err := json.Unmarshal(raw, &target); err != nil {
			return err
		}
		if target.DesiredCommit != attempted {
			return nil
		}
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if applyErr != nil {
			target.Status = TargetFailed
			target.LastError = applyErr.Error()
		} else {
			target.Status = TargetReady
			target.LastError = ""
			target.AppliedCommit = syncResult.BasisCommit
		}
		next, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(repository), next)
	})
}

func (s *TargetStore) view(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.View(fn)
}

func (s *TargetStore) update(fn func(*bolt.Tx) error) error {
	db, err := bolt.Open(s.file, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Update(fn)
}

type RepositoryLookup func(kernel.RepositoryID) (knowledge.Repository, error)

// RepositoryInventory lists attached repositories that may need a live
// projection. Controller looks up each id and skips members that are not
// knowledge-capable. Without an inventory, Reconcile can only see ids already
// present in controller.db and cannot recover a never-desired HEAD.
type RepositoryInventory func() ([]kernel.RepositoryID, error)

const DefaultReconcileInterval = 15 * time.Second

type stateNoticeRecord struct {
	ChangeNotice
	Status    string `json:"status"`
	LastError string `json:"lastError,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

// Controller durably coalesces desired projection commits and Bound State
// change notices on independent keys. Desire is the only work performed on
// the Writer receipt path; compilation and provider I/O run in the background
// or through explicit CatchUp. controller.db is a work queue, not Snapshot
// truth: CatchUp first reconciles published HEAD.
type Controller struct {
	index       *Index
	store       *TargetStore
	lookup      RepositoryLookup
	inventory   RepositoryInventory
	interval    time.Duration
	wake        chan struct{}
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	startMu     sync.Mutex
	catchupMu   sync.Mutex
	lookupMu    sync.RWMutex
	stateLookup knowledgeserving.StateLookup
	request     knowledgeserving.RequestContext
}

func NewController(index *Index, store *TargetStore, lookup RepositoryLookup) (*Controller, error) {
	if err := store.ready(); err != nil {
		return nil, err
	}
	return &Controller{
		index:    index,
		store:    store,
		lookup:   lookup,
		interval: DefaultReconcileInterval,
		wake:     make(chan struct{}, 1),
	}, nil
}

func (c *Controller) SetInventory(inventory RepositoryInventory) {
	c.inventory = inventory
}

func (c *Controller) SetStateLookup(lookup knowledgeserving.StateLookup) {
	c.lookupMu.Lock()
	c.stateLookup = lookup
	c.lookupMu.Unlock()
}

func (c *Controller) SetRequestContext(request knowledgeserving.RequestContext) {
	c.lookupMu.Lock()
	c.request = request
	c.lookupMu.Unlock()
}

func (c *Controller) stateRuntime() (knowledgeserving.StateLookup, knowledgeserving.RequestContext) {
	c.lookupMu.RLock()
	defer c.lookupMu.RUnlock()
	return c.stateLookup, c.request
}

func (c *Controller) SetReconcileInterval(interval time.Duration) {
	if interval > 0 {
		c.interval = interval
	}
}

func (c *Controller) Desire(repository kernel.RepositoryID, commit kernel.CommitID) error {
	if err := c.store.Desire(repository, commit); err != nil {
		return err
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return nil
}

// Notify enqueues a Bound State invalidation. It never writes an observation
// value and does not share Snapshot Desire keys.
func (c *Controller) Notify(notice ChangeNotice) error {
	if err := ValidateChangeNotice(notice); err != nil {
		return err
	}
	lookup, _ := c.stateRuntime()
	if lookup == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic State projection requires a Materialization Runtime")
	}
	if err := c.store.enqueueNotice(notice); err != nil {
		return err
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	return nil
}

func (c *Controller) Start(parent context.Context) {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	interval := c.interval
	if interval <= 0 {
		interval = DefaultReconcileInterval
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		_ = c.CatchUp(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.wake:
				_ = c.CatchUp(ctx)
			case <-ticker.C:
				_ = c.CatchUp(ctx)
			}
		}
	}()
}

func (c *Controller) Close() {
	c.startMu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.startMu.Unlock()
	if cancel != nil {
		cancel()
		c.wg.Wait()
	}
}

func (c *Controller) CatchUp(ctx context.Context) error {
	c.catchupMu.Lock()
	defer c.catchupMu.Unlock()
	if err := c.reconcile(ctx); err != nil {
		return err
	}
	if err := c.applyPending(ctx); err != nil {
		return err
	}
	return c.catchUpState(ctx)
}

func (c *Controller) Reconcile(ctx context.Context) error {
	c.catchupMu.Lock()
	defer c.catchupMu.Unlock()
	return c.reconcile(ctx)
}

func (c *Controller) reconcile(ctx context.Context) error {
	ids, err := c.inventoryIDs()
	if err != nil {
		return err
	}
	known, err := c.store.List()
	if err != nil {
		return err
	}
	byRepo := map[kernel.RepositoryID]ProjectionTarget{}
	for _, target := range known {
		byRepo[target.Repository] = target
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		repo, head, ok := c.publishedHead(id)
		if !ok {
			continue
		}
		if !c.needsHead(repo, head, byRepo[id]) {
			continue
		}
		if err := c.Desire(id, head); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) applyPending(ctx context.Context) error {
	targets, err := c.store.List()
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.alignedWithHead(target) {
			continue
		}
		repo, lookupErr := c.lookup(target.Repository)
		result := IndexSync{}
		applyErr := lookupErr
		if applyErr == nil && repo != nil && c.index != nil {
			result, applyErr = c.index.Ensure(repo, target.DesiredCommit)
		} else if applyErr == nil && (repo == nil || c.index == nil) {
			applyErr = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller has no knowledge repository")
		}
		if err := c.store.finish(target.Repository, target.DesiredCommit, result, applyErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) inventoryIDs() ([]kernel.RepositoryID, error) {
	if c.inventory != nil {
		return c.inventory()
	}
	targets, err := c.store.List()
	if err != nil {
		return nil, err
	}
	ids := make([]kernel.RepositoryID, 0, len(targets))
	seen := map[kernel.RepositoryID]bool{}
	for _, target := range targets {
		if seen[target.Repository] {
			continue
		}
		seen[target.Repository] = true
		ids = append(ids, target.Repository)
	}
	return ids, nil
}

func (c *Controller) publishedHead(id kernel.RepositoryID) (knowledge.Repository, kernel.CommitID, bool) {
	if c.lookup == nil {
		return nil, "", false
	}
	repo, err := c.lookup(id)
	if err != nil {
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return nil, "", false
		}
		return nil, "", false
	}
	if repo == nil {
		return nil, "", false
	}
	head, err := repo.Head("")
	if err != nil || head == "" {
		return nil, "", false
	}
	return repo, head, true
}

func (c *Controller) needsHead(repo knowledge.Repository, head kernel.CommitID, target ProjectionTarget) bool {
	if target.DesiredCommit != head || target.AppliedCommit != head || target.Status != TargetReady {
		return true
	}
	if c.index == nil {
		return false
	}
	desc, err := c.index.Describe(repo)
	if err != nil {
		return true
	}
	return desc.BasisCommit != head || desc.State != ProjectionStateReady
}

func (c *Controller) alignedWithHead(target ProjectionTarget) bool {
	if target.Status != TargetReady || target.AppliedCommit != target.DesiredCommit {
		return false
	}
	_, head, ok := c.publishedHead(target.Repository)
	if !ok {
		return true
	}
	return target.DesiredCommit == head
}

func (c *Controller) Targets() ([]ProjectionTarget, error) { return c.store.List() }

func (c *Controller) catchUpState(ctx context.Context) error {
	lookup, request := c.stateRuntime()
	if lookup == nil || c.index == nil || c.lookup == nil {
		return nil
	}
	ids, err := c.inventoryIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		repo, head, ok := c.publishedHead(id)
		if !ok {
			continue
		}
		has, bindErr := c.index.HasStateBindings(repo, head)
		if bindErr != nil || !has {
			continue
		}
		if _, ready := c.index.StateView(id, head); ready {
			continue
		}
		_, _ = c.index.RefreshState(ctx, repo, head, lookup, request)
	}
	notices, err := c.store.pendingNotices()
	if err != nil {
		return err
	}
	for _, notice := range notices {
		if err := ctx.Err(); err != nil {
			return err
		}
		applyErr := c.refreshNotice(ctx, notice, lookup, request)
		if err := c.store.finishNotice(notice, applyErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) refreshNotice(ctx context.Context, notice ChangeNotice, lookup knowledgeserving.StateLookup, request knowledgeserving.RequestContext) error {
	repo, err := c.lookup(notice.Repository)
	if err != nil {
		return err
	}
	if repo == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "projection controller has no knowledge repository")
	}
	head, err := repo.Head(snapshot.RefOrDefault(notice.Ref))
	if err != nil {
		return err
	}
	if notice.Address == nil || strings.TrimSpace(string(notice.Address.ObjectID)) == "" {
		_, err = c.index.RefreshState(ctx, repo, head, lookup, request)
		return err
	}
	_, err = c.index.RefreshStateObjects(ctx, repo, head, lookup, request, []knowledge.ObjectID{notice.Address.ObjectID})
	return err
}

func noticeKey(notice ChangeNotice) []byte {
	addr := "*"
	if notice.Address != nil {
		addr = knowledge.AddressKey(*notice.Address)
	}
	return []byte(string(notice.Repository) + "\x00" + snapshot.RefOrDefault(notice.Ref) + "\x00" + addr)
}

func (s *TargetStore) enqueueNotice(notice ChangeNotice) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(stateNoticeBucket)
		if err != nil {
			return err
		}
		record := stateNoticeRecord{ChangeNotice: notice, Status: TargetPending}
		if raw := bucket.Get(noticeKey(notice)); raw != nil {
			_ = json.Unmarshal(raw, &record)
			record.ChangeNotice = notice
		}
		record.Status = TargetPending
		record.LastError = ""
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put(noticeKey(notice), raw)
	})
}

func (s *TargetStore) pendingNotices() ([]ChangeNotice, error) {
	notices := []ChangeNotice{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateNoticeBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, raw []byte) error {
			var record stateNoticeRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			if record.Status == TargetReady {
				return nil
			}
			notices = append(notices, record.ChangeNotice)
			return nil
		})
	})
	return notices, err
}

func (s *TargetStore) finishNotice(notice ChangeNotice, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateNoticeBucket)
		if bucket == nil {
			return nil
		}
		key := noticeKey(notice)
		if applyErr == nil {
			return bucket.Delete(key)
		}
		record := stateNoticeRecord{ChangeNotice: notice, Status: TargetFailed, LastError: applyErr.Error(), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put(key, raw)
	})
}
