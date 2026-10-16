package service

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	common "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
	interfaces "kc/liveaccess/handler"
)

type pluginAccessorFunc func(context.Context, liveaccess.ResourceOperationRequest) (any, error)

func (f pluginAccessorFunc) AccessResource(ctx context.Context, request liveaccess.ResourceOperationRequest) (any, error) {
	return f(ctx, request)
}

func TestAccessPluginsCloseDrainsLookupOperationAndPrivateInterface(t *testing.T) {
	entered := make(chan string, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var closed atomic.Int32
	closeFailure := errors.New("owned close failure")
	plugins, err := OpenPlugins(mainPlugin(), []kernel.RepositoryID{"repo"}, map[string]PluginEntry{"liveaccess": {Open: func(scope common.PluginScope) (PluginContribution, error) {
		scope.Own(func() error { closed.Add(1); return closeFailure })
		return PluginContribution{
			Lookup: pluginLookupFunc(func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
				entered <- "lookup"
				<-release
				return liveaccess.StateObservation{Value: "lookup-result"}, nil
			}),
			Accessor: pluginAccessorFunc(func(context.Context, liveaccess.ResourceOperationRequest) (any, error) {
				entered <- "operation"
				<-release
				return "operation-result", nil
			}),
			Interfaces: pluginInterfaceRegistry(t, pluginInterfaceFunc(func(context.Context, interfaces.InterfaceCall) (interfaces.InterfaceResult, error) {
				entered <- "interface"
				<-release
				return interfaces.InterfaceResult{Value: "interface-result"}, nil
			})),
		}, nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	implementation := pluginImplementation("repo")
	registry, err := plugins.InterfaceRegistryFor(implementation)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := registry.Resolve(implementation)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 3)
	go func() {
		value, err := plugins.LookupState(context.Background(), lookupRequest("http"))
		if err == nil && value.Value != "lookup-result" {
			err = errors.New("lookup result changed")
		}
		results <- err
	}()
	go func() {
		value, err := plugins.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: "repo"}, Runtime: "http", Protocol: testProtocol})
		if err == nil && value != "operation-result" {
			err = errors.New("operation result changed")
		}
		results <- err
	}()
	go func() {
		value, err := registration.Handler.Execute(context.Background(), interfaces.InterfaceCall{Implementation: implementation})
		if err == nil && value.Value != "interface-result" {
			err = errors.New("interface result changed")
		}
		results <- err
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("accepted contribution did not execute")
		}
	}
	closing := make(chan error, 1)
	go func() { closing <- plugins.Close() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		plugins.mu.Lock()
		closingStarted := plugins.closed
		plugins.mu.Unlock()
		if closingStarted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not reject new calls")
		}
		runtime.Gosched()
	}
	if closed.Load() != 0 {
		t.Fatal("owned resource released while contribution was running")
	}
	select {
	case <-closing:
		t.Fatal("Close returned before contributions drained")
	default:
	}
	if _, err := plugins.LookupState(context.Background(), lookupRequest("http")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("new call admitted during Close: %v", err)
	}
	if _, err := registration.Handler.Execute(context.Background(), interfaces.InterfaceCall{Implementation: implementation}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("escaped private registry admitted after Close: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := <-closing; !errors.Is(err, closeFailure) {
		t.Fatalf("Close error lost: %v", err)
	}
	if err := plugins.Close(); !errors.Is(err, closeFailure) || closed.Load() != 1 {
		t.Fatalf("Close did not retain one release: %v count=%d", err, closed.Load())
	}
}
