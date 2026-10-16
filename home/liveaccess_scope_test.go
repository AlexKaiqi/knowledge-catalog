package home

import (
	"context"
	"sync/atomic"
	"testing"

	common "kc/discovery/adoption"
	"kc/kernel"
	"kc/liveaccess"
	accessplugin "kc/liveaccess/plugin"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
)

type admittedAccessAdapter struct {
	lookups atomic.Int64
	invokes atomic.Int64
}

func (a *admittedAccessAdapter) LookupState(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	a.lookups.Add(1)
	return liveaccess.StateObservation{Value: request.Binding.Repository}, nil
}

func (a *admittedAccessAdapter) AccessResource(_ context.Context, request liveaccess.ResourceOperationRequest) (any, error) {
	a.invokes.Add(1)
	return request.Descriptor.Repository, nil
}

func TestLiveAccessUsesCurrentManagedRepositoryScopeWithoutReopening(t *testing.T) {
	_, ws, _ := managedFixture(t)
	adapter := &admittedAccessAdapter{}
	opened := 0
	entry := accessplugin.Entry(accessplugin.Options{OpenLookup: func(scope common.PluginScope) (liveaccess.StateLookup, error) {
		if scope.ContainsRepository == nil {
			t.Fatal("adapter factory did not receive the host's current scope")
		}
		return adapter, nil
	}})
	custom := liveservice.PluginEntry{Affordance: entry.Affordance, Open: func(scope common.PluginScope) (liveservice.PluginContribution, error) {
		opened++
		if scope.ContainsRepository == nil {
			t.Fatal("compiled Entry did not receive the host's current scope")
		}
		return entry.Open(scope)
	}}
	if err := ws.SetLiveAccessEntries(map[string]liveservice.PluginEntry{"liveaccess": custom}); err != nil {
		t.Fatal(err)
	}
	if err := ws.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	plugins := ws.LiveAccess
	if err := plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := kernel.RepositoryID("kr://managed/late-liveaccess")
	check := func(want kernel.ErrorCode) {
		t.Helper()
		observed, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: id, Runtime: "http", Protocol: objectmodel.ProtocolResourceAccessV1}})
		if kernel.CodeOf(err) != want || want == "" && observed.Value != id {
			t.Errorf("lookup for current Repository: value=%#v err=%v, want %s", observed.Value, err, want)
		}
		value, err := plugins.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Runtime: "http", Protocol: objectmodel.ProtocolResourceAccessV1, Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: id}})
		if kernel.CodeOf(err) != want || want == "" && value != id {
			t.Errorf("invoke for current Repository: value=%#v err=%v, want %s", value, err, want)
		}
	}
	check(kernel.ErrCapabilityUnsatisfied)
	if _, err := ws.CreateManagedRepository(ManagedRepositoryRequest{RepositoryID: string(id), CommandID: "late-liveaccess", Principal: "alice"}, func(ManagedRepositoryGrant) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := ws.AttachRepository(id); err != nil {
		t.Fatal(err)
	}
	check("")
	source, _ := ws.Store.Get(id)
	defer closeManagedSource(source)
	ws.Store.Delete(id)
	check(kernel.ErrCapabilityUnsatisfied)
	if ws.LiveAccess != plugins || opened != 1 || adapter.lookups.Load() != 1 || adapter.invokes.Load() != 1 {
		t.Fatalf("scope change reopened runtime or reached rejected sources: opens=%d lookup=%d invoke=%d", opened, adapter.lookups.Load(), adapter.invokes.Load())
	}
}
