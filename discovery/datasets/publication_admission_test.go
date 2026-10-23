package datasets

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type originPublicationAuthority struct {
	*publicationFaultAuthority
	origin string
}

func (s originPublicationAuthority) Origin() string { return s.origin }

func guardedPublicationService(t *testing.T, fixture publicationFailureFixture) dataset.Service {
	t.Helper()
	registry, err := dataset.OpenSnapshotRegistry(originPublicationAuthority{publicationFaultAuthority: fixture.authority, origin: "fixture:" + t.TempDir()}, snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	service, err := dataset.NewService(fixture.setup.Store, registry)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// Acceptance is a metadata decision. It records the release and admits no
// preparation, so the accepted release has no receipt until its own build
// adopts it (doc/design/dataset.md#发布).
func TestDatasetPublicationLeavesNoPreparationReceipt(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	var events []string
	publisher := DatasetPublisher{
		Registry:  service,
		Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
		Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
			events = append(events, "validate")
			return def, nil
		},
	}
	accepted, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources})
	if err != nil || accepted.Revision != 2 {
		t.Fatalf("publication did not accept: %#v %v", accepted, err)
	}
	if want := []string{"authorize", "validate"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("publication order: %v want %v", events, want)
	}
	current, err := service.Set("reading")
	if err != nil || current.Revision != 2 {
		t.Fatalf("accepted release is not serving: %#v %v", current, err)
	}
	receipt, err := service.(dataset.PublicationCoordinator).PublicationReceipt(context.Background(), dataset.PublicationLookup{SetID: "reading", Revision: 2})
	if err != nil || receipt.Admission != "" {
		t.Fatalf("acceptance admitted a preparation: %#v %v", receipt, err)
	}
	if fixture.authority.attempts != 1 {
		t.Fatalf("acceptance attempts: %d", fixture.authority.attempts)
	}
}

func TestDatasetPublisherRejectsIncompleteServices(t *testing.T) {
	var events []string
	registry := &publicationRegistry{events: &events, accepted: 1}
	allow := func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil }
	pass := func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
		events = append(events, "validate")
		return def, nil
	}
	for name, executor := range map[string]DatasetPublisher{
		"registry":  {Authorize: allow, Prepare: pass},
		"authorize": {Registry: registry, Prepare: pass},
		"prepare":   {Registry: registry, Authorize: allow},
	} {
		if _, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(events) != 0 || registry.accepted != 1 {
			t.Fatalf("%s-less publisher changed publication order or outcome: %v %v", name, events, err)
		}
	}
}

type admissionResource struct {
	admit func(context.Context, dataset.Dataset, dataset.PublicationAuthority) error
}

func (r *admissionResource) Admit(ctx context.Context, raw []byte) error {
	var proof publicationResourceEvidence
	if err := json.Unmarshal(raw, &proof); err != nil {
		return err
	}
	return r.admit(ctx, proof.Definition, proof.Authority)
}
func (*admissionResource) Confirm(context.Context) error { return nil }
func (*admissionResource) Cancel(context.Context) error  { return nil }
func (*admissionResource) Close() error                  { return nil }
