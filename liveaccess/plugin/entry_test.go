package plugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	common "kc/discovery/plugin"
	"kc/kernel"
	"kc/liveaccess"
	accessplugin "kc/liveaccess/plugin"
	"kc/liveaccess/service"
	objectmodel "kc/object"
)

func TestEntryDefaultCallsActualHTTPAdapterAndEnforcesRepoScope(t *testing.T) {
	var body map[string]any
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/access" || r.Header.Get("X-Resource-Principal") != "reader" || r.Header.Get("X-Resource-Request-Id") != "request-7" {
			t.Errorf("actual adapter target/identity changed: %s %s %#v", r.Method, r.URL, r.Header)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": map[string]any{"status": "healthy"}, "basis": objectmodel.ObservationBasis{BindingGeneration: "source-1", Consistency: objectmodel.ObservationRepeatable, SourceRevision: "rev-7", ObservedAt: "2026-10-14T10:00:00Z"}})
	}))
	defer runtime.Close()
	var resources []func() error
	contribution, err := accessplugin.Entry(accessplugin.Options{}).Open(common.PluginScope{Name: "liveaccess", Instance: "selected", Repositories: []kernel.RepositoryID{"repo"}, Own: func(close func() error) { resources = append(resources, close) }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, close := range resources {
			if err := close(); err != nil {
				t.Error(err)
			}
		}
	}()
	request := liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo", DeclarationCommit: "pinned-1", DeclarationDigest: "decl", Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "status"}, Runtime: "source", Protocol: objectmodel.ProtocolResourceAccessV1, Operations: map[string]objectmodel.BindingOperation{"read": {Call: "status.read"}}, Origin: runtime.URL}, Identity: kernel.IdentityContext{Principal: "reader"}, RequestID: "request-7"}
	observation, err := contribution.Lookup.LookupState(context.Background(), request)
	if err != nil || !reflect.DeepEqual(observation.Value, map[string]any{"status": "healthy"}) || observation.Basis.SourceRevision != "rev-7" {
		t.Fatalf("plugin did not return actual source observation: %#v %v", observation, err)
	}
	if body["call"] != "status.read" || body["binding"].(map[string]any)["declarationCommit"] != "pinned-1" {
		t.Fatalf("fixed binding changed: %#v", body)
	}
	request.Binding.Repository = "outside"
	if _, err := contribution.Lookup.LookupState(context.Background(), request); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("entry escaped configured Repo scope: %v", err)
	}
	if _, err := contribution.Accessor.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: "outside"}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("operation escaped configured Repo scope: %v", err)
	}
}

type injectedLookup struct{ instance string }

func (l *injectedLookup) LookupState(context.Context, liveaccess.StateLookupRequest) (liveaccess.StateObservation, error) {
	return liveaccess.StateObservation{Value: l.instance}, nil
}

func TestEntryStaticExplanationDoesNotOpenAndInstanceFactoryIsUsed(t *testing.T) {
	opened, released := 0, 0
	entry := accessplugin.Entry(accessplugin.Options{OpenLookup: func(scope common.PluginScope) (liveaccess.StateLookup, error) {
		opened++
		scope.Own(func() error { released++; return nil })
		return &injectedLookup{instance: scope.Instance}, nil
	}})
	if entry.Affordance.Name != "liveaccess" || entry.Affordance.Usage == "" || opened != 0 {
		t.Fatalf("static affordance opened runtime: %#v opens=%d", entry.Affordance, opened)
	}
	configs := common.RepositoryPluginConfigs{Repositories: map[kernel.RepositoryID]common.RepositoryPluginConfig{"repo": {Version: 1, Plugins: []common.PluginSelection{{Name: "liveaccess", Instance: "replacement"}}}}}
	plugins, err := service.OpenPlugins(configs, map[string]service.PluginEntry{"liveaccess": entry})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plugins.LookupState(context.Background(), liveaccess.StateLookupRequest{Binding: liveaccess.ResolvedBinding{Repository: "repo"}})
	if err != nil || result.Value != "replacement" || opened != 1 {
		t.Fatalf("configured factory not adopted: %#v %v opens=%d", result, err, opened)
	}
	if _, err := plugins.AccessResource(context.Background(), liveaccess.ResourceOperationRequest{Descriptor: liveaccess.ResourceDescriptorCoordinate{Repository: "repo"}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("lookup-only adapter gained source operations: %v", err)
	}
	_ = plugins.Close()
	_ = plugins.Close()
	if released != 1 {
		t.Fatalf("factory resource closed %d times", released)
	}
}

func TestEntryNilInjectedAdapterIsUnavailable(t *testing.T) {
	entry := accessplugin.Entry(accessplugin.Options{OpenLookup: func(common.PluginScope) (liveaccess.StateLookup, error) {
		var lookup *injectedLookup
		return lookup, nil
	}})
	if _, err := entry.Open(common.PluginScope{Name: "liveaccess", Instance: "main", Repositories: []kernel.RepositoryID{"repo"}}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("typed nil adapter admitted: %v", err)
	}
}
