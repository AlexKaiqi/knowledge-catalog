package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/knowledge"
)

type noopSnapshotConsumer string

func (c noopSnapshotConsumer) ID() string { return string(c) }
func (c noopSnapshotConsumer) Reconcile(context.Context, knowledge.Repository, kernel.CommitID) error {
	return nil
}

func TestConsumerDesirePersistenceFailureStillWakesEveryLane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	c, err := NewController(nil, NewTargetStore(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cache/v1", "vector/v1"} {
		if err := c.RegisterConsumer(noopSnapshotConsumer(id)); err != nil {
			t.Fatal(err)
		}
	}
	// A broken queue must not suppress wakeups: workers recover through HEAD.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := c.Desire("kr://acme/public/core", "commit"); err == nil {
		t.Fatal("failed persistence was hidden")
	}
	for _, lane := range c.snapshotConsumers() {
		select {
		case <-lane.wake:
		default:
			t.Fatalf("consumer %s was not signaled", lane.id)
		}
	}
	select {
	case <-c.wake:
	default:
		t.Fatal("projection lane was not signaled")
	}
}

func TestConsumerOnlyControllerRejectsStateNoticeWithRuntime(t *testing.T) {
	c, err := NewController(nil, NewTargetStore(filepath.Join(t.TempDir(), "controller.db")), nil)
	if err != nil {
		t.Fatal(err)
	}
	c.SetStateLookup(&stateTestLookup{})
	if err := c.Notify(ChangeNotice{Repository: "kr://acme/public/core"}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("notice err=%v", err)
	}
	notices, err := c.store.pendingNotices()
	if err != nil || len(notices) != 0 {
		t.Fatalf("unsupported State notice was enqueued: %#v err=%v", notices, err)
	}
}
