package service_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

type auditedCatalogProvider struct {
	base     datasetHTTPCatalogs
	opened   []string
	empty    bool
	redirect *dataset.Catalog
}

func (p *auditedCatalogProvider) CatalogIDs() []string { return p.base.CatalogIDs() }
func (p *auditedCatalogProvider) OpenCatalog(id string) (*dataset.Catalog, error) {
	p.opened = append(p.opened, id)
	if p.empty {
		return nil, nil
	}
	if p.redirect != nil {
		return p.redirect, nil
	}
	return p.base.OpenCatalog(id)
}

func otherFileCatalog(t *testing.T) *dataset.Catalog {
	t.Helper()
	registry, err := dataset.NewRegistry(t.TempDir(), "B")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := dataset.NewCatalog(snapshot.NewRegistry(), registry)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestSnapshotFileAuthorizationPrecedesHiddenCatalogLookup(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	fixture.rules.rules = nil
	provider := &auditedCatalogProvider{base: datasetHTTPCatalogs{fixture.catalog}}
	fixture.resources.Catalogs = provider
	for _, catalogID := range []string{"A", "B", "unknown"} {
		response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"catalog": catalogID, "dataset": "files"}, http.StatusForbidden)
		if response["error"].(map[string]any)["code"] != string(kernel.ErrForbidden) {
			t.Fatalf("Catalog existence affected unauthorized response: %#v", response)
		}
	}
	if len(provider.opened) != 0 || source.reads != 0 {
		t.Fatalf("unauthorized request reached catalog/source lookup: catalogs=%v reads=%d", provider.opened, source.reads)
	}
}

func TestSnapshotFileMissingCatalogPortResultFailsClosed(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	fixture.resources.Catalogs = &auditedCatalogProvider{base: datasetHTTPCatalogs{fixture.catalog}, empty: true}
	response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"catalog": "A", "dataset": "files"}, http.StatusConflict)
	if response["error"].(map[string]any)["code"] != string(kernel.ErrPreconditionFailed) || source.reads != 0 {
		t.Fatalf("missing Catalog did not fail before delivery: %#v reads=%d", response, source.reads)
	}
}

func TestSnapshotFileRejectsDifferentCatalogAuthority(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	provider := &auditedCatalogProvider{base: datasetHTTPCatalogs{fixture.catalog}, redirect: otherFileCatalog(t)}
	fixture.resources.Catalogs = provider
	response := datasetHTTPCall(t, fixture.handler(), http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"catalog": "A", "dataset": "files"}, http.StatusConflict)
	if response["error"].(map[string]any)["code"] != string(kernel.ErrPreconditionFailed) {
		t.Fatalf("wrong Catalog authority did not fail its binding contract: %#v", response)
	}
	if len(provider.opened) != 1 || provider.opened[0] != "A" || source.reads != 0 {
		t.Fatalf("actual Catalog scope was not rechecked: lookups=%v reads=%d", provider.opened, source.reads)
	}
}

func TestSnapshotFileRechecksActualDefaultCatalogScope(t *testing.T) {
	fixture, source := newFileHTTPFixture(t)
	provider := &auditedCatalogProvider{base: datasetHTTPCatalogs{fixture.catalog}, redirect: otherFileCatalog(t)}
	fixture.resources.Catalogs = provider
	checked := []string{}
	fixture.resources.Authorize = func(_ context.Context, request service.Authorization) error {
		checked = append(checked, request.Query.Catalog)
		query := request.Query
		query.Principal = request.Caller.Principal
		// The embedding application's existing default policy admits A. A
		// provider returning B cannot borrow that admission for its authority.
		if query.Catalog == "" {
			query.Catalog = "A"
		}
		if _, ok := access.Match(fixture.rules.rules, query); ok {
			return nil
		}
		return kernel.Fail(kernel.ErrForbidden, "%s is not allowed to %s", request.Caller.Principal, query.Action)
	}
	handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Local: true}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
		return fixture.resources, func() {}, nil
	}})
	datasetHTTPCall(t, handler, http.MethodPost, "/dataset-files/v1/mounts:list", map[string]any{"dataset": "files"}, http.StatusForbidden)
	if !reflect.DeepEqual(checked, []string{"", "B"}) || !reflect.DeepEqual(provider.opened, []string{""}) || source.reads != 0 {
		t.Fatalf("default admission was reused for another Catalog: checked=%v lookups=%v reads=%d", checked, provider.opened, source.reads)
	}
}
