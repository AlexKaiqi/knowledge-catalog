package liveaccess_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

type bindingScopeGuard struct {
	snapshot.Store
	snapshot.TreeStore
	privateReads int
}

func (g *bindingScopeGuard) ReadFile(path string, at kernel.CommitID) ([]byte, error) {
	if strings.HasPrefix(path, "private/") {
		g.privateReads++
		return nil, fmt.Errorf("binding resolver read an unpublished Schema body: %s", path)
	}
	return g.TreeStore.ReadFile(path, at)
}

func TestLiveAccessBindingUsesScopedSchemaCoordinatesWithoutPrivateReads(t *testing.T) {
	var requests atomic.Int64
	runtime := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer runtime.Close()
	raw := testkit.MakeTreeStore(t, "kr://binding/scoped")
	registry := snapshot.NewRegistry()
	if err := registry.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := testkit.MustHead(t, raw, snapshot.DefaultRef)
	bound := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.health"}
	properties := objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/service.properties"}
	entity := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/orders", AspectName: "properties"}
	published, err := w.Commit("publish-bindings", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: before, ExpectedTargetCommit: before,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: bound, PathHint: "public/health-schema.yaml", Value: map[string]any{
				"entity": "Service", "aspect": "health",
				"fields": map[string]any{"status": map[string]any{"type": "string"}},
			}},
			{Op: objectmodel.OpPut, Address: properties, PathHint: "public/properties-schema.yaml", Value: map[string]any{
				"entity": "Service", "aspect": "properties", "fields": map[string]any{"name": map[string]any{"type": "string"}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/private.health"}, PathHint: "private/health-schema.yaml", Value: map[string]any{
				"entity": "Service", "aspect": "health",
				"fields": map[string]any{"secret": map[string]any{"type": "string"}},
			}},
			bindingfixture.WithPath(bindingfixture.PutDescriptor("resource/health", bindingfixture.Descriptor(runtime.URL)), "public/health-descriptor.yaml"),
			bindingfixture.WithPath(bindingfixture.PutDescriptor("resource/private", bindingfixture.Descriptor("https://private.example")), "private/private-descriptor.yaml"),
			{Op: objectmodel.OpPut, Address: entity, SchemaRef: string(properties.ObjectID), PathHint: "public/orders.yaml", Value: map[string]any{"name": "orders"}},
			bindingfixture.WithPath(bindingfixture.PutAspect(entity.ObjectID, "health", string(bound.ObjectID), "resource/health"), "public/orders-health.yaml"),
			bindingfixture.WithPath(bindingfixture.PutAspect("service/secret", "health", "schema/private.health", "resource/private"), "private/secret-health.yaml"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	at := published.Result.NewCommit
	guard := &bindingScopeGuard{Store: raw, TreeStore: raw.(snapshot.TreeStore)}
	rd := reader.NewReader(nil)
	scope := reader.Open(rd.Lookup(func(kernel.RepositoryID) (snapshot.Store, error) { return guard, nil }), reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): at},
		Items:        []reader.DatasetItem{{Repository: raw.ID(), Commit: at, Kind: "prefix", Prefix: "public"}},
	})
	member, err := scope.Member(raw.ID())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := member.(objectmodel.BindingLocator).BindingSchemaObjectIDs(at)
	if err != nil || !reflect.DeepEqual(ids, []objectmodel.ObjectID{bound.ObjectID}) {
		t.Fatalf("fixed binding coordinates escaped the accepted Schema namespace: %v %v", ids, err)
	}
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: entity.ObjectID, AspectName: "health"}
	resolved, err := liveaccess.ResolveBindingAt(scope, raw.ID(), address)
	if err != nil || resolved.Origin != runtime.URL || resolved.SchemaRef != string(bound.ObjectID) || resolved.DeclarationCommit != at || resolved.Mode != objectmodel.BindingState || resolved.DescriptorRef != "resource/health" {
		t.Fatalf("LiveAccess did not interpret the allowed fixed declaration: %#v %v", resolved, err)
	}
	all, err := liveaccess.ResolveBinding(scope, address)
	if err != nil || len(all) != 1 || all[0].DeclarationDigest != resolved.DeclarationDigest {
		t.Fatalf("binding browse differed from exact scoped interpretation: %#v %v", all, err)
	}
	updated, err := w.Commit("advance-descriptor-origin", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at,
		Operations: []objectmodel.Operation{bindingfixture.PutDescriptor("resource/health", bindingfixture.Descriptor("https://new.example"))},
	})
	if err != nil || updated.Result.NewCommit == at {
		t.Fatalf("origin advance failed to establish a different authority basis: %#v %v", updated, err)
	}
	again, err := liveaccess.ResolveBindingAt(scope, raw.ID(), address)
	if err != nil || again.DeclarationCommit != at || again.Origin != runtime.URL || again.DeclarationDigest != resolved.DeclarationDigest {
		t.Fatalf("scoped binding interpretation followed newer HEAD: %#v %v", again, err)
	}
	if guard.privateReads != 0 || requests.Load() != 0 {
		t.Fatalf("binding interpretation crossed its read boundary: private=%d external=%d", guard.privateReads, requests.Load())
	}
}
