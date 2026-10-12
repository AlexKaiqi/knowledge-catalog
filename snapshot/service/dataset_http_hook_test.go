package service_test

import (
	"context"
	"reflect"
	"testing"

	"kc/identity"
	"kc/snapshot/dataset"
	"kc/snapshot/service"
)

func TestDatasetHTTPPublicationHooksObserveOneAuthoritativeSwitch(t *testing.T) {
	f := newDatasetHTTPFixture(t)
	events := []string{}
	f.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
		events = append(events, "portable")
		f.source.head = "c2"
		return definition, nil, nil
	}
	f.resources.PreparePublication = func(_ context.Context, _ identity.HTTPIdentity, definition dataset.KnowledgeSet) error {
		events = append(events, "ready")
		if _, err := f.catalog.Set("files"); err == nil {
			t.Fatal("current pointer advanced before capability preparation")
		}
		if definition.Sources[0].Commit != "c2" {
			t.Fatal("preparation did not inherit the portable-recipe source version")
		}
		return nil
	}
	h := service.NewHandler(service.Config{
		Identity: service.IdentityOptions{Mode: "local", Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
			return f.resources, func() {}, nil
		},
		Around: func(_ context.Context, _ identity.HTTPIdentity, operation string, run func() (any, error)) (any, error) {
			if operation != "dataset-define" {
				t.Fatalf("wrong mutation aspect: %s", operation)
			}
			events = append(events, "before")
			out, err := run()
			events = append(events, "after")
			accepted, acceptedErr := f.catalog.Set("files")
			if err != nil || acceptedErr != nil || accepted.Revision != 1 {
				t.Fatalf("after hook did not observe the accepted release: %#v, %v, %v", accepted, acceptedErr, err)
			}
			return out, err
		},
	})
	datasetHTTPCall(t, h, "POST", "/snapshot/v1/datasets", datasetHTTPDefinition(1, f.source.ID()), 200)
	if !reflect.DeepEqual(events, []string{"before", "portable", "ready", "after"}) {
		t.Fatalf("publication order changed: %v", events)
	}
}

func TestDatasetHTTPPreHookRevocationPrecedesPreparationAndRetirement(t *testing.T) {
	for _, operation := range []string{"dataset-define", "dataset-retire"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newDatasetHTTPFixture(t)
			if _, err := fixture.catalog.DefineKnowledgeSet("files", 1, []dataset.KnowledgeSetSource{{Repository: fixture.source.id, Selector: "refs/heads/main"}}); err != nil {
				t.Fatal(err)
			}
			prepared, hookCalls := 0, 0
			fixture.resources.BeforePublish = func(_ context.Context, _ identity.HTTPIdentity, _ dataset.Service, definition dataset.KnowledgeSet) (dataset.KnowledgeSet, map[string]any, error) {
				prepared++
				return definition, nil, nil
			}
			handler := service.NewHandler(service.Config{
				Identity: service.IdentityOptions{Local: true},
				Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
					return fixture.resources, func() {}, nil
				},
				Around: func(_ context.Context, _ identity.HTTPIdentity, actual string, run func() (any, error)) (any, error) {
					hookCalls++
					if actual != operation {
						t.Fatalf("wrong hook operation: %s", actual)
					}
					fixture.rules.rules = nil
					return run()
				},
			})
			target, body := "/snapshot/v1/datasets", any(datasetHTTPDefinition(2, fixture.source.id))
			if operation == "dataset-retire" {
				target, body = target+"/files/retire", map[string]any{}
			}
			datasetHTTPCall(t, handler, "POST", target, body, 403)
			current, err := fixture.catalog.Set("files")
			if err != nil || current.Revision != 1 || current.Retired || prepared != 0 || hookCalls != 1 {
				t.Fatalf("pre-hook revocation allowed mutation: current=%#v err=%v prepared=%d hooks=%d", current, err, prepared, hookCalls)
			}
			if _, err := fixture.catalog.DatasetVersion("files", 2); err == nil {
				t.Fatal("revoked candidate became accepted history")
			}
		})
	}
}
