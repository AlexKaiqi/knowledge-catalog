package maintenance

import (
	"context"
	"kc/kernel"
	objectmodel "kc/object"
	"os"
	"path/filepath"
	"testing"
)

type noopSnapshotConsumer string

func (c noopSnapshotConsumer) ID() string { return string(c) }
func (c noopSnapshotConsumer) Reconcile(context.Context, objectmodel.Repository, kernel.CommitID) error {
	return nil
}

func TestConsumerDesirePersistenceFailureStillWakesEveryLane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	c, err := NewCoordinator(path, nil)
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
}
