package datasets

import (
	"context"
	"encoding/json"
	"errors"
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

func TestDatasetPublisherAdmitsFrozenPreparationBeforeAuthorityAccepts(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "accept", true: "reject"}[reject], func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			var events []string
			denied := errors.New("resource admission denied")
			admit := func(_ context.Context, def dataset.Dataset, authority dataset.PublicationAuthority) error {
				events = append(events, "admit")
				current, err := service.Set("reading")
				if err != nil || current.Revision != 1 || authority.Head != kernel.CommitID(fixture.authorityHead) || authority.Binding == "" || authority.Ref != snapshot.DefaultRef ||
					def.SetID != "reading" || def.Revision != 2 || len(def.Sources) != 1 || def.Sources[0].Commit != fixture.nextCommit {
					t.Fatalf("admission did not bind frozen preparation to the actual authority: candidate=%#v authority=%#v current=%#v err=%v", def, authority, current, err)
				}
				if reject {
					return denied
				}
				return nil
			}
			executor := DatasetPublisher{
				Registry:  service,
				Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
				PrepareResources: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
					events = append(events, "prepare")
					return def, &admissionResource{admit: admit}, nil
				},
			}
			result, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources})
			if !reflect.DeepEqual(events, []string{"authorize", "prepare", "admit"}) {
				t.Fatalf("publication order: %v", events)
			}
			current, _ := service.Set("reading")
			if reject {
				if !errors.Is(err, denied) || current.Revision != 1 || fixture.authority.attempts != 0 {
					t.Fatalf("rejected admission changed authority: %#v %v", current, err)
				}
			} else if err != nil || result.Revision != 2 || current.Revision != 2 || fixture.authority.attempts != 1 {
				t.Fatalf("admitted publication was not accepted once: %#v %v attempts=%d", result, err, fixture.authority.attempts)
			}
		})
	}
}

func TestDatasetPublisherRejectsMissingCoordinationBeforePreparingResources(t *testing.T) {
	var events []string
	registry := &publicationRegistry{events: &events, accepted: 1}
	executor := DatasetPublisher{
		Registry:  registry,
		Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
		PrepareResources: func(context.Context, dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
			t.Fatal("prepared resources without an authority coordinator")
			return dataset.Dataset{}, nil, nil
		},
	}
	if _, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2}); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || !reflect.DeepEqual(events, []string{"authorize"}) {
		t.Fatalf("missing coordination changed publication order or outcome: %v %v", events, err)
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
