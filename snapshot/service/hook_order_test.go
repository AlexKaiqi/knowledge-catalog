package service_test

import (
	"context"
	"net/http"
	"testing"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func TestSnapshotDatasetHTTPAuthorizesBeforeHooksAndPublishesOnlyOnce(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	f.rules.rules[0].Actions = []string{"catalog.read"}
	hookCalls, prepared := 0, 0
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ *dataset.Catalog, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
		prepared++
		return definition, nil, nil
	}
	config := service.Config{
		Identity: service.IdentityOptions{Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
		Around: func(ctx context.Context, caller identity.HTTPIdentity, operation string, run func() (any, error)) (any, error) {
			hookCalls++
			metadata := service.ContextMetadata(ctx)
			if metadata.Catalog != "A" || metadata.Dataset != "files" || caller.Principal != "alice" || operation != "dataset-define" {
				t.Fatalf("hook lost diagnostic scope: %#v %#v %s", metadata, caller, operation)
			}
			_, _ = run()
			_, _ = run()
			return map[string]any{"setId": "invented", "revision": 99}, nil
		},
	}
	handler := service.NewHandler(config)
	base := "/catalog/v1/catalogs/A/datasets"
	body := datasetHTTPDefinition(1, f.source.ID())
	datasetHTTPCall(t, handler, http.MethodPost, base, body, http.StatusForbidden)
	if hookCalls != 0 || prepared != 0 {
		t.Fatalf("unauthorized publication triggered an external hook: hooks=%d prepared=%d", hookCalls, prepared)
	}
	f.rules.rules[0].Actions = append(f.rules.rules[0].Actions, "dataset.manage")
	result := datasetHTTPCall(t, handler, http.MethodPost, base, body, http.StatusOK)
	accepted, err := f.catalog.Set("files")
	if err != nil || accepted.Revision != 1 || result["setId"] != "files" || result["revision"] != float64(1) || hookCalls != 1 || prepared != 1 {
		t.Fatalf("hook repeated or replaced accepted publication: %#v accepted=%#v err=%v hooks=%d prepared=%d", result, accepted, err, hookCalls, prepared)
	}
	f.resources.PreparePublication = func(context.Context, identity.HTTPIdentity, dataset.KnowledgeSet) error {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "external preparation unavailable")
	}
	result = datasetHTTPCall(t, handler, http.MethodPost, base, datasetHTTPDefinition(2, f.source.ID()), http.StatusBadRequest)
	if result["error"].(map[string]any)["code"] != string(kernel.ErrCapabilityUnsatisfied) {
		t.Fatalf("hook fabricated success after a domain failure: %#v", result)
	}
	accepted, err = f.catalog.Set("files")
	if err != nil || accepted.Revision != 1 || hookCalls != 2 || prepared != 2 {
		t.Fatalf("failed publication changed accepted authority: %#v %v hooks=%d prepared=%d", accepted, err, hookCalls, prepared)
	}
	f.resources.PreparePublication = nil
	config.Around = func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error) {
		return nil, kernel.Fail(kernel.ErrHookDenied, "pre-hook refused")
	}
	datasetHTTPCall(t, service.NewHandler(config), http.MethodPost, base, datasetHTTPDefinition(2, f.source.ID()), http.StatusBadRequest)
	accepted, err = f.catalog.Set("files")
	if err != nil || accepted.Revision != 1 || prepared != 2 {
		t.Fatalf("pre-hook denial reached publication: %#v %v prepared=%d", accepted, err, prepared)
	}
}
