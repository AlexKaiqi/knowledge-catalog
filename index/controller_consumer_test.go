package index_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kc/index"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
)

type testSnapshotConsumer struct {
	id        string
	reconcile func(context.Context, knowledge.Repository, kernel.CommitID) error
}

func (c *testSnapshotConsumer) ID() string { return c.id }
func (c *testSnapshotConsumer) Reconcile(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID) error {
	return c.reconcile(ctx, repo, commit)
}

func newConsumerController(t *testing.T, path string, repo knowledge.Repository) *index.Controller {
	t.Helper()
	c, err := index.NewController(nil, index.NewTargetStore(path), func(id kernel.RepositoryID) (knowledge.Repository, error) { return repo, nil })
	if err != nil {
		t.Fatal(err)
	}
	c.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
	t.Cleanup(c.Close)
	return c
}

func TestSnapshotConsumersHaveIndependentDurableProgressAndRetry(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	path := filepath.Join(t.TempDir(), "controller.db")
	c := newConsumerController(t, path, repo)
	goodCalls, badCalls := 0, 0
	for _, consumer := range []index.SnapshotConsumer{
		&testSnapshotConsumer{id: "cache/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { goodCalls++; return nil }},
		&testSnapshotConsumer{id: "vector/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error {
			badCalls++
			return errors.New("provider unavailable")
		}},
	} {
		if err := c.RegisterConsumer(consumer); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Desire(repo.ID(), head); err != nil {
		t.Fatal(err)
	}
	if goodCalls != 0 || badCalls != 0 {
		t.Fatal("Writer receipt performed consumer work")
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err := c.ConsumerTargets()
	if err != nil || len(targets) != 2 {
		t.Fatalf("targets=%#v err=%v", targets, err)
	}
	for _, target := range targets {
		if target.DesiredCommit != head {
			t.Fatalf("wrong desired basis: %#v", target)
		}
		if target.ConsumerID == "cache/v1" && (target.Status != index.TargetReady || target.AppliedCommit != head || target.LastError != "") {
			t.Fatalf("cache target=%#v", target)
		}
		if target.ConsumerID == "vector/v1" && (target.Status != index.TargetFailed || target.AppliedCommit != "" || target.LastError != "provider unavailable") {
			t.Fatalf("vector target=%#v", target)
		}
	}
	// A replacement process must recheck its own volatile storage despite durable READY.
	reopened := newConsumerController(t, path, repo)
	restoredCalls := 0
	if err := reopened.RegisterConsumer(&testSnapshotConsumer{id: "cache/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { restoredCalls++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RegisterConsumer(&testSnapshotConsumer{id: "vector/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err = reopened.ConsumerTargets()
	if err != nil || restoredCalls != 1 || len(targets) != 2 {
		t.Fatalf("restart calls=%d targets=%#v err=%v", restoredCalls, targets, err)
	}
	for _, target := range targets {
		if target.Status != index.TargetReady || target.AppliedCommit != head || target.LastError != "" {
			t.Fatalf("retry target=%#v", target)
		}
	}
}

func TestSnapshotConsumerWorkersRecoverLostNotificationsWithoutBlockingEachOther(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	c := newConsumerController(t, filepath.Join(t.TempDir(), "controller.db"), repo)
	c.SetReconcileInterval(10 * time.Millisecond)
	slowStarted := make(chan struct{})
	slowStopped := make(chan struct{})
	var once sync.Once
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "blocked/v1", reconcile: func(ctx context.Context, _ knowledge.Repository, _ kernel.CommitID) error {
		once.Do(func() { close(slowStarted) })
		<-ctx.Done()
		close(slowStopped)
		return ctx.Err()
	}}); err != nil {
		t.Fatal(err)
	}
	received := make(chan kernel.CommitID, 20)
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "cache/v1", reconcile: func(ctx context.Context, _ knowledge.Repository, commit kernel.CommitID) error {
		select {
		case received <- commit:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	c.Start(context.Background())
	select {
	case <-slowStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("slow worker not started")
	}
	awaitCommit := func(want kernel.CommitID) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case got := <-received:
				if got == want {
					return
				}
			case <-deadline:
				t.Fatalf("consumer never reconciled %s", want)
			}
		}
	}
	awaitCommit(head)
	next := putAt(t, repo, head, testkit.PutEntity("policy/P-2", map[string]any{"body": "new runbook"}, ""))
	// Deliberately omit Desire: periodic reconciliation discovers the published HEAD.
	awaitCommit(next)
	c.Close()
	select {
	case <-slowStopped:
	default:
		t.Fatal("Close returned before worker cancellation completed")
	}
}

func TestSnapshotConsumerRegistrationIsUniqueAndFrozenAtStart(t *testing.T) {
	repo, _, _ := committedSearchablePolicy(t)
	c := newConsumerController(t, filepath.Join(t.TempDir(), "controller.db"), repo)
	noop := func(context.Context, knowledge.Repository, kernel.CommitID) error { return nil }
	if err := c.RegisterConsumer(nil); err == nil {
		t.Fatal("nil consumer accepted")
	}
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "", reconcile: noop}); err == nil {
		t.Fatal("empty consumer ID accepted")
	}
	consumer := &testSnapshotConsumer{id: "cache/v1", reconcile: noop}
	if err := c.RegisterConsumer(consumer); err != nil {
		t.Fatal(err)
	}
	if err := c.RegisterConsumer(consumer); err == nil {
		t.Fatal("duplicate consumer accepted")
	}
	c.Start(context.Background())
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "vector/v1", reconcile: noop}); err == nil {
		t.Fatal("registration after Start accepted")
	}
	c.Close()
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "vector/v1", reconcile: noop}); err == nil {
		t.Fatal("registration after Close accepted")
	}
}

func TestSnapshotConsumerLateCompletionCannotOverwriteNewDesiredCommit(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	c := newConsumerController(t, filepath.Join(t.TempDir(), "controller.db"), repo)
	started := make(chan struct{})
	release := make(chan struct{})
	var first sync.Once
	consumer := &testSnapshotConsumer{id: "cache/v1", reconcile: func(ctx context.Context, _ knowledge.Repository, commit kernel.CommitID) error {
		first.Do(func() {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
		return ctx.Err()
	}}
	if err := c.RegisterConsumer(consumer); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.CatchUp(context.Background()) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not start")
	}
	next := putAt(t, repo, head, testkit.PutEntity("policy/P-2", map[string]any{"body": "new version"}, ""))
	if err := c.Desire(repo.ID(), next); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	targets, err := c.ConsumerTargets()
	if err != nil || len(targets) != 1 || targets[0].DesiredCommit != next || targets[0].AppliedCommit != "" || targets[0].Status != index.TargetPending {
		t.Fatalf("late completion replaced newer target: %#v err=%v", targets, err)
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err = c.ConsumerTargets()
	if err != nil || targets[0].Status != index.TargetReady || targets[0].AppliedCommit != next {
		t.Fatalf("target did not recover: %#v err=%v", targets, err)
	}
}

func TestSnapshotConsumerConcurrentRegistrationKeepsOneInstance(t *testing.T) {
	repo, _, _ := committedSearchablePolicy(t)
	c := newConsumerController(t, filepath.Join(t.TempDir(), "controller.db"), repo)
	results := make(chan error, 12)
	for n := 0; n < cap(results); n++ {
		go func() {
			results <- c.RegisterConsumer(&testSnapshotConsumer{id: "cache/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { return nil }})
		}()
	}
	accepted := 0
	for n := 0; n < cap(results); n++ {
		if <-results == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d duplicate consumer registrations", accepted)
	}
}

func TestSnapshotConsumerRevisionsDoNotShareProgress(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	path := filepath.Join(t.TempDir(), "controller.db")
	c := newConsumerController(t, path, repo)
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "cache/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	reopened := newConsumerController(t, path, repo)
	reopened.SetInventory(nil) // The persisted consumer records still identify the repository.
	calls := 0
	if err := reopened.RegisterConsumer(&testSnapshotConsumer{id: "cache/v2", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error {
		calls++
		return errors.New("new layout is not ready")
	}}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err := reopened.ConsumerTargets()
	if err != nil || len(targets) != 2 || calls != 1 {
		t.Fatalf("targets=%#v calls=%d err=%v", targets, calls, err)
	}
	for _, target := range targets {
		if target.ConsumerID == "cache/v1" && (target.Status != index.TargetReady || target.AppliedCommit != head) {
			t.Fatalf("old revision changed: %#v", target)
		}
		if target.ConsumerID == "cache/v2" && (target.Status != index.TargetFailed || target.AppliedCommit != "") {
			t.Fatalf("new revision inherited READY: %#v", target)
		}
	}
}

func TestSnapshotConsumerHeadFailureDoesNotLeaveReady(t *testing.T) {
	repo, _, head := committedSearchablePolicy(t)
	var lookupErr error
	c, err := index.NewController(nil, index.NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), func(kernel.RepositoryID) (knowledge.Repository, error) { return repo, lookupErr })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	c.SetInventory(func() ([]kernel.RepositoryID, error) { return []kernel.RepositoryID{repo.ID()}, nil })
	calls := 0
	if err := c.RegisterConsumer(&testSnapshotConsumer{id: "cache/v1", reconcile: func(context.Context, knowledge.Repository, kernel.CommitID) error { calls++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	lookupErr = kernel.Fail(kernel.ErrTemporaryUnavailable, "repository transport unavailable")
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err := c.ConsumerTargets()
	if err != nil || len(targets) != 1 || calls != 1 || targets[0].Status != index.TargetFailed || targets[0].AppliedCommit != head || targets[0].LastError != lookupErr.Error() {
		t.Fatalf("HEAD failure was hidden: targets=%#v calls=%d err=%v", targets, calls, err)
	}
	lookupErr = nil
	if err := c.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	targets, err = c.ConsumerTargets()
	if err != nil || targets[0].Status != index.TargetReady || targets[0].LastError != "" {
		t.Fatalf("HEAD recovery=%#v err=%v", targets, err)
	}
}
