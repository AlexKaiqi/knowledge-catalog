package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	common "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
)

type pluginLookupFunc func(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error)

func (f pluginLookupFunc) LookupState(ctx context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	return f(ctx, request)
}

func pluginConfigs(selections map[kernel.RepositoryID]string) common.RepositoryPluginConfigs {
	configs := common.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]common.RepositoryPluginConfig{}}
	for repo, instance := range selections {
		configs.Repositories[repo] = common.RepositoryPluginConfig{Version: 1, Plugins: []common.PluginSelection{{Name: "liveaccess", Instance: instance}}}
	}
	return configs
}

func TestAccessPluginsSelectRepoInstanceAndPreserveRequest(t *testing.T) {
	configs := pluginConfigs(map[kernel.RepositoryID]string{"repo-a": "one", "repo-b": "two"})
	configs.Repositories["unknown"] = common.RepositoryPluginConfig{Version: 1, Plugins: []common.PluginSelection{{Name: "absent", Instance: "one"}}}
	opened := map[string][]kernel.RepositoryID{}
	var got liveaccess.StateLookupRequest
	entry := PluginEntry{Open: func(scope common.PluginScope) (PluginContribution, error) {
		opened[scope.Instance] = scope.Repositories
		return PluginContribution{Lookup: pluginLookupFunc(func(_ context.Context, request liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
			got = request
			return liveaccess.StateObservation{Value: scope.Instance}, nil
		})}, nil
	}}
	plugins, err := OpenPlugins(configs, map[string]PluginEntry{"liveaccess": entry})
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	if !reflect.DeepEqual(opened, map[string][]kernel.RepositoryID{"one": {"repo-a"}, "two": {"repo-b"}}) {
		t.Fatalf("instance scopes: %#v", opened)
	}
	request := liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo-b", DeclarationCommit: "pinned", DeclarationDigest: "declaration"}, SchemaRef: "schema/status", Origin: "https://source.example", Identity: kernel.IdentityContext{Principal: "reader", OnBehalfOf: "alice"}, Trace: kernel.TraceContext{TraceID: "trace", SpanID: "span"}, RequestID: "request"}
	result, err := plugins.LookupState(context.Background(), request)
	if err != nil || result.Value != "two" || !reflect.DeepEqual(got, request) {
		t.Fatalf("request or selected result changed: %#v %#v %v", got, result, err)
	}
	for _, repo := range []kernel.RepositoryID{"unknown", "not-configured"} {
		if _, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: repo}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("%s fell through selection: %v", repo, err)
		}
	}
	if _, err := plugins.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: "repo-b"}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("lookup-only plugin acquired an undeclared operation port: %v", err)
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
	plugins, err := OpenPlugins(pluginConfigs(map[kernel.RepositoryID]string{"repo-bad": "bad", "repo-good": "good"}), map[string]PluginEntry{"liveaccess": entry})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closed, []string{"bad/second", "bad/first"}) {
		t.Fatalf("partial resources not released immediately in reverse order: %v", closed)
	}
	if _, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo-bad"}}); !errors.Is(err, failure) {
		t.Fatalf("selected construction failure lost: %v", err)
	}
	value, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo-good"}})
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

func TestAccessPluginsIncompleteEntryAndUnavailableConfigFailExplicitly(t *testing.T) {
	for _, entry := range []PluginEntry{{}, {Open: func(common.PluginScope) (PluginContribution, error) {
		var lookup *nilPluginLookup
		return PluginContribution{Lookup: lookup}, nil
	}}} {
		plugins, err := OpenPlugins(pluginConfigs(map[kernel.RepositoryID]string{"repo": "main"}), map[string]PluginEntry{"liveaccess": entry})
		if err != nil {
			t.Fatal(err)
		}
		_, err = plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo"}})
		if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
			t.Fatalf("incomplete contribution accepted: %v", err)
		}
		_ = plugins.Close()
	}
	failure := errors.New("Repo metadata unavailable")
	plugins, err := OpenPlugins(common.RepositoryPluginConfigs{Unavailable: map[kernel.RepositoryID]error{"repo": failure}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer plugins.Close()
	if _, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo"}}); !errors.Is(err, failure) {
		t.Fatalf("metadata failure became no configuration: %v", err)
	}
	if _, err := OpenPlugins(common.RepositoryPluginConfigs{Unavailable: map[kernel.RepositoryID]error{"repo": nil}}, nil); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("invalid global setup accepted: %v", err)
	}
}

type nilPluginLookup struct{}

func (*nilPluginLookup) LookupState(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	panic("nil adapter must never be invoked")
}
