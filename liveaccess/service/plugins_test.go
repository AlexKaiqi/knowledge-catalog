package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	common "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
)

const testProtocol = "resource-access/v1"

type pluginLookupFunc func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error)

func (f pluginLookupFunc) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	return f(ctx, request)
}

// pluginConfigs deploys LiveAccess instances keyed by instance name.
func pluginConfigs(instances map[string]common.PluginInstanceConfig) common.PluginsConfig {
	return common.PluginsConfig{"liveaccess": {Instances: instances}}
}

// mainPlugin is one protocol-wide instance.
func mainPlugin() common.PluginsConfig {
	return pluginConfigs(map[string]common.PluginInstanceConfig{"main": {Protocol: testProtocol}})
}

func lookupRequest(runtime string) liveaccess.StateLookupRequest {
	return liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo", Runtime: runtime, Protocol: testProtocol}}
}

func TestAccessPluginsRouteByRuntimeAndProtocolAndPreserveRequest(t *testing.T) {
	configs := pluginConfigs(map[string]common.PluginInstanceConfig{
		"warehouse": {Protocol: testProtocol, Runtimes: []string{"mysql-shop"}},
		"default":   {Protocol: testProtocol, Runtimes: []string{"http"}},
	})
	opened := map[string][]kernel.RepositoryID{}
	var got liveaccess.StateLookupRequest
	entry := PluginEntry{Open: func(scope common.PluginScope) (PluginContribution, error) {
		opened[scope.Instance] = scope.Repositories
		return PluginContribution{Lookup: pluginLookupFunc(func(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
			got = request
			return liveaccess.StateObservation{Value: scope.Instance}, nil
		})}, nil
	}}
	plugins, err := OpenPlugins(configs, []kernel.RepositoryID{"repo-b", "repo-a"}, map[string]PluginEntry{"liveaccess": entry})
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	all := []kernel.RepositoryID{"repo-a", "repo-b"}
	if !reflect.DeepEqual(opened, map[string][]kernel.RepositoryID{"default": all, "warehouse": all}) {
		t.Fatalf("instances were scoped by Repository: %#v", opened)
	}
	request := liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo-b", DeclarationCommit: "pinned", DeclarationDigest: "declaration", Runtime: "mysql-shop", Protocol: testProtocol}, SchemaRef: "schema/status", Origin: "https://source.example", Identity: kernel.IdentityContext{Principal: "reader", OnBehalfOf: "alice"}, Trace: kernel.TraceContext{TraceID: "trace", SpanID: "span"}, RequestID: "request"}
	result, err := plugins.LookupState(context.Background(), request)
	if err != nil || result.Value != "warehouse" || !reflect.DeepEqual(got, request) {
		t.Fatalf("request or routed result changed: %#v %#v %v", got, result, err)
	}
	if result, err := plugins.LookupState(context.Background(), lookupRequest("http")); err != nil || result.Value != "default" {
		t.Fatalf("runtime was not routed to its own instance: %#v %v", result, err)
	}
	if _, err := plugins.LookupState(context.Background(), lookupRequest("unlisted")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("an unlisted runtime borrowed a listed instance: %v", err)
	}
	unmapped := liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo-a", Runtime: "http", Protocol: "resource-access/v2"}}
	if _, err := plugins.LookupState(context.Background(), unmapped); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !strings.Contains(err.Error(), "resource-access/v2") {
		t.Fatalf("unmapped protocol fell through to another instance: %v", err)
	}
	if _, err := plugins.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Runtime: "http", Protocol: testProtocol}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("lookup-only plugin acquired an undeclared operation port: %v", err)
	}
}

func TestAccessPluginsRefuseOverlappingCatalogs(t *testing.T) {
	entries := map[string]PluginEntry{"liveaccess": {}}
	for name, instances := range map[string]map[string]common.PluginInstanceConfig{
		"same runtime":      {"a": {Protocol: testProtocol, Runtimes: []string{"http"}}, "b": {Protocol: testProtocol, Runtimes: []string{"http"}}},
		"two protocol-wide": {"a": {Protocol: testProtocol}, "b": {Protocol: testProtocol}},
		"wide and listed":   {"a": {Protocol: testProtocol}, "b": {Protocol: testProtocol, Runtimes: []string{"http"}}},
	} {
		_, err := OpenPlugins(pluginConfigs(instances), nil, entries)
		if kernel.CodeOf(err) != kernel.ErrPreconditionFailed || !strings.Contains(err.Error(), "liveaccess/a") || !strings.Contains(err.Error(), "liveaccess/b") {
			t.Fatalf("%s was served by assembly order: %v", name, err)
		}
	}
	for name, instance := range map[string]common.PluginInstanceConfig{"missing protocol": {}, "index shape": {Protocol: testProtocol, Catalog: "declarative-index/opensearch@2026-10"}} {
		if _, err := OpenPlugins(pluginConfigs(map[string]common.PluginInstanceConfig{"main": instance}), nil, entries); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("%s opened an access instance: %v", name, err)
		}
	}
}

func TestAccessPluginConstructionFailureIsLocalAndReleasesPartialResources(t *testing.T) {
	failure := errors.New("adapter cannot open selected instance")
	var closed []string
	entry := PluginEntry{Open: func(scope common.PluginScope) (PluginContribution, error) {
		scope.Own(func() error { closed = append(closed, scope.Instance+"/first"); return nil })
		scope.Own(func() error { closed = append(closed, scope.Instance+"/second"); return nil })
		if scope.Instance == "bad" {
			return PluginContribution{}, failure
		}
		return PluginContribution{Lookup: pluginLookupFunc(func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
			return liveaccess.StateObservation{Value: "independent"}, nil
		})}, nil
	}}
	configs := pluginConfigs(map[string]common.PluginInstanceConfig{
		"bad":  {Protocol: testProtocol, Runtimes: []string{"broken"}},
		"good": {Protocol: testProtocol, Runtimes: []string{"http"}},
	})
	plugins, err := OpenPlugins(configs, []kernel.RepositoryID{"repo"}, map[string]PluginEntry{"liveaccess": entry})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closed, []string{"bad/second", "bad/first"}) {
		t.Fatalf("partial resources not released immediately in reverse order: %v", closed)
	}
	if _, err := plugins.LookupState(context.Background(), lookupRequest("broken")); !errors.Is(err, failure) {
		t.Fatalf("instance construction failure lost: %v", err)
	}
	value, err := plugins.LookupState(context.Background(), lookupRequest("http"))
	if err != nil || value.Value != "independent" {
		t.Fatalf("independent instance blocked: %#v %v", value, err)
	}
	if err := plugins.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugins.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closed, []string{"bad/second", "bad/first", "good/second", "good/first"}) {
		t.Fatalf("resources closed more than once: %v", closed)
	}
}

func TestAccessPluginsIncompleteEntryAndAbsenceFailExplicitly(t *testing.T) {
	for _, entry := range []PluginEntry{{}, {Open: func(common.PluginScope) (PluginContribution, error) {
		var lookup *nilPluginLookup
		return PluginContribution{Lookup: lookup}, nil
	}}} {
		plugins, err := OpenPlugins(mainPlugin(), []kernel.RepositoryID{"repo"}, map[string]PluginEntry{"liveaccess": entry})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = plugins.LookupState(context.Background(), lookupRequest("http")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("incomplete contribution accepted: %v", err)
		}
		_ = plugins.Close()
	}
	plugins, err := OpenPlugins(common.PluginsConfig{}, []kernel.RepositoryID{"repo"}, map[string]PluginEntry{"liveaccess": {}})
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	if plugins.Configured() {
		t.Fatal("an absent plugin was reported as installed")
	}
	if _, err := plugins.LookupState(context.Background(), lookupRequest("http")); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("absent LiveAccess did not report a missing capability: %v", err)
	}
}

type nilPluginLookup struct{}

func (*nilPluginLookup) LookupState(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	panic("nil adapter must never be invoked")
}
