package snapshot

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"kc/kernel"
)

func TestRegistrySupportsProvisioningDuringInventoryReads(t *testing.T) {
	r := NewRegistry()
	const count = 64
	var notified atomic.Int64
	// Event consumers may query the registry: callbacks cannot run under its
	// mutation lock. Projection inventory reads run independently of HTTP.
	r.OnAdvanced(func(event Advanced) {
		if _, ok := r.Get(event.Store.ID()); !ok {
			t.Error("event delivered without its source")
		}
		notified.Add(1)
	})
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < count; i++ {
			store := &registryStore{id: kernel.RepositoryID(fmt.Sprintf("kr://managed/%d", i))}
			if err := r.Add(store); err != nil {
				t.Error(err)
			}
			r.NotifyAdvanced(Advanced{Store: store, To: "root"})
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < count; i++ {
			for _, id := range r.IDs() {
				if _, err := r.Require(id, kernel.ErrPreconditionFailed); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < count; i++ {
			r.OnAdvanced(func(Advanced) {})
		}
	}()
	close(start)
	workers.Wait()
	if len(r.IDs()) != count || notified.Load() != count {
		t.Fatalf("allocations or notifications lost: %d / %d", len(r.IDs()), notified.Load())
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
