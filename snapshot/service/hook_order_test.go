package service_test

import (
	"context"
	"net/http"
	"testing"

	"kc/identity"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func TestSnapshotDatasetHTTPAuthorizesBeforePublicationHooks(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	f.rules.rules[0].Actions = []string{"inventory.read"}
	before := dataset.NormalizeState(f.catalog.DumpState())
	f.source.inspections = 0
	hookCalls, prepared := 0, 0
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
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
			if metadata.Dataset != "files" || caller.Principal != "alice" || operation != "dataset-define" {
				t.Fatalf("hook lost diagnostic scope: %#v %#v %s", metadata, caller, operation)
			}
			return run()
		},
	}
	handler := service.NewHandler(config)
	base := "/snapshot/v1/datasets"
	body := datasetHTTPDefinition(1, f.source.ID())
	datasetHTTPCall(t, handler, http.MethodPost, base, body, http.StatusForbidden)
	if hookCalls != 0 || prepared != 0 || f.source.inspections != 0 {
		t.Fatalf("unauthorized publication reached external preparation: hooks=%d prepared=%d inspections=%d", hookCalls, prepared, f.source.inspections)
	}
	assertDatasetHTTPState(t, f, before)
	f.rules.rules[0].Actions = append(f.rules.rules[0].Actions, "dataset.manage")
	result := datasetHTTPCall(t, handler, http.MethodPost, base, body, http.StatusOK)
	accepted, err := f.catalog.Set("files")
	if err != nil || accepted.Revision != 1 || result["setId"] != "files" || result["revision"] != float64(1) || hookCalls != 1 || prepared != 1 {
		t.Fatalf("authorized route lost publication hook wiring: %#v accepted=%#v err=%v hooks=%d prepared=%d", result, accepted, err, hookCalls, prepared)
	}
	if _, err := f.catalog.DatasetVersion("files", 1); err != nil || f.source.inspections == 0 {
		t.Fatalf("authorized route did not accept its inspected candidate: err=%v inspections=%d", err, f.source.inspections)
	}
}
