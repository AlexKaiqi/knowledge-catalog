package home

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	common "kc/discovery/adoption"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	accessplugin "kc/liveaccess/plugin"
	"kc/snapshot"
)

func TestLiveAccessCurrentScopeOverridesInitialMembershipAndIsConcurrent(t *testing.T) {
	registry := snapshot.NewRegistry()
	initial := testkit.MakeTreeStore(t, "kr://scope/initial")
	late := testkit.MakeTreeStore(t, "kr://scope/late")
	if err := registry.Add(initial); err != nil {
		t.Fatal(err)
	}
	adapter := &admittedAccessAdapter{}
	var opened atomic.Int64
	entry := accessplugin.Entry(accessplugin.Options{OpenLookup: func(scope common.PluginScope) (liveaccess.StateLookup, error) {
		opened.Add(1)
		if scope.ContainsRepository == nil {
			t.Fatal("factory did not receive the current membership port")
		}
		return adapter, nil
	}})
	ports, err := entry.Open(common.PluginScope{Repositories: []kernel.RepositoryID{initial.ID()}, ContainsRepository: func(id kernel.RepositoryID) bool {
		_, ok := registry.Get(id)
		return ok
	}})
	if err != nil {
		t.Fatal(err)
	}
	check := func(id kernel.RepositoryID, changing bool, want kernel.ErrorCode) {
		t.Helper()
		observed, lookupErr := ports.Lookup.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: id}})
		value, invokeErr := ports.Accessor.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: id}})
		for _, result := range []struct {
			name  string
			value any
			err   error
		}{{"lookup", observed.Value, lookupErr}, {"invoke", value, invokeErr}} {
			code := kernel.CodeOf(result.err)
			if changing && code != "" && code != kernel.ErrCapabilityUnsatisfied || !changing && code != want || code == "" && result.value != id {
				t.Errorf("%s current scope: value=%#v err=%v", result.name, result.value, result.err)
			}
		}
	}
	check(initial.ID(), false, "")
	registry.Delete(initial.ID())
	check(initial.ID(), false, kernel.ErrCapabilityUnsatisfied)
	check(late.ID(), false, kernel.ErrCapabilityUnsatisfied)
	if err := registry.Add(late); err != nil {
		t.Fatal(err)
	}
	check(late.ID(), false, "")
	var readers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for i := 0; i < 150; i++ {
				check(late.ID(), true, "")
			}
		}()
	}
	close(start)
	for i := 0; i < 100; i++ {
		registry.Delete(late.ID())
		if err := registry.Add(late); err != nil {
			t.Fatal(err)
		}
	}
	readers.Wait()
	registry.Delete(late.ID())
	lookups, invokes := adapter.lookups.Load(), adapter.invokes.Load()
	check(late.ID(), false, kernel.ErrCapabilityUnsatisfied)
	check("kr://scope/outside", false, kernel.ErrCapabilityUnsatisfied)
	if opened.Load() != 1 || adapter.lookups.Load() != lookups || adapter.invokes.Load() != invokes {
		t.Fatal("scope refresh reopened the adapter or forwarded a rejected request")
	}
}
