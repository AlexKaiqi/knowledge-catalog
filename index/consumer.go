package index

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// SnapshotConsumer maintains one rebuildable derivative of a published Snapshot.
// ID identifies a consumer instance and its revision, and must remain stable for
// its lifetime. Reconcile is called at startup, on notices and on periodic HEAD
// reconciliation, including when the durable target already reports READY.
// Consumers must therefore be idempotent and check their actual serving state;
// a durable READY record cannot prove a process-local cache is still resident.
// The repository and commit are fixed inputs, never an instruction to read latest.
// Implementations must honor ctx cancellation so Controller.Close can stop them.
// They neither implement retrieval semantics nor write Canonical knowledge.
type SnapshotConsumer interface {
	ID() string
	Reconcile(context.Context, knowledge.Repository, kernel.CommitID) error
}

// ConsumerTarget is an independent progress record for a registered Snapshot
// consumer. It is maintenance metadata, not proof of query readiness or content
// availability. ProjectionTarget remains the existing search projection view.
type ConsumerTarget struct {
	ConsumerID string `json:"consumerID"`
	ProjectionTarget
}

var consumerTargetBucket = []byte("snapshot-consumer-targets")

type consumerLane struct {
	id       string
	consumer SnapshotConsumer
	wake     chan struct{}
	mu       sync.Mutex
}

func (lane *consumerLane) signal() {
	select {
	case lane.wake <- struct{}{}:
	default:
	}
}

// RegisterConsumer adds a separately scheduled Snapshot derivative before the
// first Start. Duplicate IDs are rejected, including different implementations
// or revisions that accidentally reuse an ID. Construction never starts work.
func (c *Controller) RegisterConsumer(consumer SnapshotConsumer) error {
	if consumer == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "Snapshot consumer is required")
	}
	id := consumer.ID()
	if strings.TrimSpace(id) == "" || strings.ContainsRune(id, '\x00') {
		return kernel.Fail(kernel.ErrUsageInvalid, "Snapshot consumer ID must be nonempty and contain no NUL")
	}
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.started {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot consumers must be registered before Controller.Start")
	}
	c.consumersMu.Lock()
	defer c.consumersMu.Unlock()
	for _, lane := range c.consumers {
		if lane.id == id {
			return kernel.Fail(kernel.ErrUsageInvalid, "Snapshot consumer ID is already registered")
		}
	}
	c.consumers = append(c.consumers, &consumerLane{id: id, consumer: consumer, wake: make(chan struct{}, 1)})
	return nil
}

func (c *Controller) snapshotConsumers() []*consumerLane {
	c.consumersMu.RLock()
	defer c.consumersMu.RUnlock()
	return append([]*consumerLane(nil), c.consumers...)
}

// ConsumerTargets returns all durable consumer records, including inactive
// revisions. One consumer's READY or failure never changes another's record.
func (c *Controller) ConsumerTargets() ([]ConsumerTarget, error) {
	return c.store.listConsumers("")
}

func (c *Controller) startWorker(ctx context.Context, interval time.Duration, wake <-chan struct{}, run func(context.Context) error) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
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
	}()
}

func (c *Controller) reconcileConsumer(ctx context.Context, lane *consumerLane) error {
	ids, err := c.inventoryIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, head, headErr := c.consumerHead(id)
		if headErr != nil {
			if err := c.store.failConsumerHead(lane.id, id, headErr); err != nil {
				return err
			}
			continue
		}
		if err := c.store.desireConsumer(lane.id, id, head); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) catchUpConsumer(ctx context.Context, lane *consumerLane) error {
	lane.mu.Lock()
	defer lane.mu.Unlock()
	if err := c.reconcileConsumer(ctx, lane); err != nil {
		return err
	}
	targets, err := c.store.listConsumers(lane.id)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Published HEAD is the authority. A stale queued notice is not permission
		// to warm a historical or unpublished commit or an unattached repository.
		repo, head, headErr := c.consumerHead(target.Repository)
		if headErr != nil {
			if err := c.store.failConsumerHead(lane.id, target.Repository, headErr); err != nil {
				return err
			}
			continue
		}
		if target.DesiredCommit != head {
			if err := c.store.desireConsumer(lane.id, target.Repository, head); err != nil {
				return err
			}
		}
		applyErr := lane.consumer.Reconcile(ctx, repo, head)
		if err := c.store.finishConsumer(lane.id, target.Repository, head, applyErr); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) consumerHead(id kernel.RepositoryID) (knowledge.Repository, kernel.CommitID, error) {
	if c.lookup == nil {
		return nil, "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Snapshot consumer has no repository lookup")
	}
	repo, err := c.lookup(id)
	if err != nil {
		return nil, "", err
	}
	if repo == nil {
		return nil, "", kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Snapshot consumer requires a knowledge repository")
	}
	head, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		return nil, "", err
	}
	if head == "" {
		return nil, "", kernel.Fail(kernel.ErrVersionUnresolved, "Snapshot consumer requires a published HEAD")
	}
	return repo, head, nil
}

func consumerKey(id string, repository kernel.RepositoryID) []byte {
	return []byte(id + "\x00" + string(repository))
}

func (s *TargetStore) desireConsumer(id string, repository kernel.RepositoryID, commit kernel.CommitID) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(consumerTargetBucket)
		if err != nil {
			return err
		}
		key := consumerKey(id, repository)
		target := ConsumerTarget{ConsumerID: id, ProjectionTarget: ProjectionTarget{Repository: repository}}
		if raw := bucket.Get(key); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			if target.DesiredCommit == commit {
				return nil
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
		return bucket.Put(key, raw)
	})
}

func (s *TargetStore) listConsumers(id string) ([]ConsumerTarget, error) {
	targets := []ConsumerTarget{}
	err := s.view(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(consumerTargetBucket)
		if bucket == nil {
			return nil
		}
		read := func(_, raw []byte) error {
			var target ConsumerTarget
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			if id == "" || target.ConsumerID == id {
				targets = append(targets, target)
			}
			return nil
		}
		if id == "" {
			return bucket.ForEach(read)
		}
		prefix := []byte(id + "\x00")
		cursor := bucket.Cursor()
		for key, raw := cursor.Seek(prefix); bytes.HasPrefix(key, prefix); key, raw = cursor.Next() {
			if err := read(key, raw); err != nil {
				return err
			}
		}
		return nil
	})
	return targets, err
}

func (s *TargetStore) finishConsumer(id string, repository kernel.RepositoryID, attempted kernel.CommitID, applyErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(consumerTargetBucket)
		if bucket == nil {
			return nil
		}
		key := consumerKey(id, repository)
		raw := bucket.Get(key)
		if raw == nil {
			return nil
		}
		var target ConsumerTarget
		if err := json.Unmarshal(raw, &target); err != nil {
			return err
		}
		if target.DesiredCommit != attempted {
			return nil
		}
		if applyErr == nil && target.Status == TargetReady && target.AppliedCommit == attempted && target.LastError == "" {
			return nil
		}
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if applyErr != nil {
			target.Status = TargetFailed
			target.LastError = applyErr.Error()
		} else {
			target.Status = TargetReady
			target.AppliedCommit = attempted
			target.LastError = ""
		}
		next, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put(key, next)
	})
}

func (s *TargetStore) failConsumerHead(id string, repository kernel.RepositoryID, headErr error) error {
	return s.update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(consumerTargetBucket)
		if err != nil {
			return err
		}
		key := consumerKey(id, repository)
		target := ConsumerTarget{ConsumerID: id, ProjectionTarget: ProjectionTarget{Repository: repository}}
		if raw := bucket.Get(key); raw != nil {
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
		} else if kernel.CodeOf(headErr) == kernel.ErrCapabilityUnsatisfied {
			// Inventory may include plain Snapshot repositories. They are not
			// consumers of Knowledge derivatives; previously known targets are
			// still marked failed when their capability disappears.
			return nil
		}
		if target.Status == TargetFailed && target.LastError == headErr.Error() {
			return nil
		}
		target.Status = TargetFailed
		target.LastError = headErr.Error()
		target.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		raw, err := json.Marshal(target)
		if err != nil {
			return err
		}
		return bucket.Put(key, raw)
	})
}
