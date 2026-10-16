package service_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

var inventoryDatasetRoutes = []string{"/snapshot/v1/inventory", "/snapshot/v1/datasets"}

func TestInventoryDatasetsRequireCompleteConsumption(t *testing.T) {
	for _, route := range inventoryDatasetRoutes {
		t.Run(route, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				rules   []access.Rule
				retired bool
				visible bool
			}{
				{name: "no consumption"},
				{name: "management only", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.manage"}}}},
				{name: "resolve only", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve"}}}},
				{name: "file only", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"file.read"}}}},
				{name: "repository read only", rules: []access.Rule{{Repo: "kr://files/source", Actions: []string{"file.read"}}, {Repo: "kr://files/other", Actions: []string{"file.read"}}}},
				{name: "partial member reads", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve"}}, {Repo: "kr://files/source", Actions: []string{"file.read"}}}},
				{name: "whole Dataset", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve", "file.read"}}}, visible: true},
				{name: "all fixed members", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve"}}, {Repo: "kr://files/source", Ref: "c1", Actions: []string{"file.read"}}, {Repo: "kr://files/other", Ref: "c1", Actions: []string{"file.read"}}}, visible: true},
				{name: "wrong member version", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve"}}, {Repo: "kr://files/source", Ref: "c2", Actions: []string{"file.read"}}, {Repo: "kr://files/other", Ref: "c1", Actions: []string{"file.read"}}}},
				{name: "retired", rules: []access.Rule{{Dataset: "delivery", Actions: []string{"dataset.resolve", "file.read"}}}, retired: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					f := newInventoryConsumptionFixture(t)
					f.rules.rules = []access.Rule{{ID: "inventory", Principal: "alice", Actions: []string{"inventory.read"}}}
					for _, rule := range tc.rules {
						rule.Principal = "alice"
						f.rules.rules = append(f.rules.rules, rule)
					}
					if tc.retired {
						if err := f.catalog.RetireDataset("delivery"); err != nil {
							t.Fatal(err)
						}
					}
					assertInventoryConsumption(t, f, f.handler(), route, tc.visible)
				})
			}
		})
	}
}

func TestInventoryDatasetsRecheckRevocationWithoutProbingSources(t *testing.T) {
	for _, route := range inventoryDatasetRoutes {
		t.Run(route, func(t *testing.T) {
			f := newInventoryConsumptionFixture(t)
			f.rules.rules = []access.Rule{
				{ID: "inventory", Principal: "alice", Actions: []string{"inventory.read"}},
				{ID: "resolve", Principal: "alice", Dataset: "delivery", Actions: []string{"dataset.resolve"}},
				{ID: "read", Principal: "alice", Dataset: "delivery", Actions: []string{"file.read"}},
			}
			handler := f.handler()
			assertInventoryConsumption(t, f, handler, route, true)
			f.rules.rules = f.rules.rules[:2]
			assertInventoryConsumption(t, f, handler, route, false)
			f.rules.rules = f.rules.rules[1:]
			failure := datasetHTTPCall(t, handler, http.MethodGet, route, nil, http.StatusForbidden)
			if failure["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
				t.Fatalf("consumption replaced inventory.read: %#v", failure)
			}
		})
	}
}

func TestInventoryDatasetsPropagateAuthorizationFailure(t *testing.T) {
	for _, route := range inventoryDatasetRoutes {
		t.Run(route, func(t *testing.T) {
			for _, action := range []string{"dataset.resolve", "file.read"} {
				t.Run(action, func(t *testing.T) {
					f := newInventoryConsumptionFixture(t)
					f.resources.Authorize = func(_ context.Context, request service.Authorization) error {
						if request.Caller.Principal != "alice" || request.InventoryDiscovery || request.Definition != nil {
							t.Fatalf("inventory changed the consumption authority: %#v", request)
						}
						if request.Query.Action == action {
							return kernel.Fail(kernel.ErrTemporaryUnavailable, "consumption policy unavailable")
						}
						return nil
					}
					failure := datasetHTTPCall(t, f.handler(), http.MethodGet, route, nil, http.StatusServiceUnavailable)
					if failure["error"].(map[string]any)["code"] != string(kernel.ErrTemporaryUnavailable) {
						t.Fatalf("policy failure became an empty inventory: %#v", failure)
					}
				})
			}
		})
	}
}

func TestInventoryDatasetsRejectUnauthenticatedCaller(t *testing.T) {
	for _, route := range inventoryDatasetRoutes {
		t.Run(route, func(t *testing.T) {
			f := newInventoryConsumptionFixture(t)
			handler := f.handler()
			withoutIdentity := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Del("X-Kc-As")
				handler.ServeHTTP(w, r)
			})
			failure := datasetHTTPCall(t, withoutIdentity, http.MethodGet, route, nil, http.StatusUnauthorized)
			if failure["error"].(map[string]any)["code"] != string(kernel.ErrUnauthenticated) {
				t.Fatalf("unauthenticated inventory changed error: %#v", failure)
			}
		})
	}
}

func newInventoryConsumptionFixture(t *testing.T) *datasetHTTPFixture {
	t.Helper()
	f := newDatasetHTTPFixture(t)
	other := &datasetHTTPSource{id: "kr://files/other", head: "c1"}
	if err := f.resources.Sources.(snapshot.Registry).Add(other); err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.RegisterRepository(other.id); err != nil {
		t.Fatal(err)
	}
	definition, err := f.catalog.PrepareDataset("delivery", 1, []dataset.DatasetSource{
		{Repository: f.source.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("docs")},
		{Repository: other.id, Selector: snapshot.DefaultRef, Path: dataset.MountPath("other")},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition.Metadata = map[string]string{"description": "selected delivery"}
	if _, err := f.catalog.PublishDataset(definition); err != nil {
		t.Fatal(err)
	}
	return f
}

func assertInventoryConsumption(t *testing.T, f *datasetHTTPFixture, handler http.Handler, route string, visible bool) {
	t.Helper()
	before := dataset.NormalizeState(f.catalog.DumpState())
	reads := inventorySourceInspections(f)
	result := datasetHTTPCall(t, handler, http.MethodGet, route, nil, http.StatusOK)
	rows := result["datasets"].([]any)
	if len(rows) != 0 && !visible || len(rows) != 1 && visible {
		t.Fatalf("inventory consumption selection differs: visible=%v rows=%#v", visible, rows)
	}
	if visible {
		row := rows[0].(map[string]any)
		if row["id"] != "delivery" || row["revision"] != float64(1) || row["metadata"].(map[string]any)["description"] != "selected delivery" {
			t.Fatalf("filtered inventory lost its accepted summary: %#v", row)
		}
	}
	if inventorySourceInspections(f) != reads || !reflect.DeepEqual(before, dataset.NormalizeState(f.catalog.DumpState())) {
		t.Fatal("inventory consumption inspected a source or changed the accepted authority")
	}
}

func inventorySourceInspections(f *datasetHTTPFixture) int {
	other, _ := f.resources.Sources.Get("kr://files/other")
	return f.source.inspections + other.(*datasetHTTPSource).inspections
}
