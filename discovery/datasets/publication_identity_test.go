package datasets

import (
	"context"
	"kc/kernel"
	"kc/snapshot/dataset"
	"reflect"
	"testing"
)

func TestDatasetPreparationCannotMutateFrozenCandidateThroughSharedValues(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		change func(*dataset.Dataset)
	}{
		{"source-commit", func(def *dataset.Dataset) { def.Sources[0].Commit = "rewritten" }},
		{"source-path-pointer", func(def *dataset.Dataset) { *def.Sources[0].Path = "redirected" }},
		{"selected-file-item", func(def *dataset.Dataset) { def.Items[0].Target = "rewritten" }},
		{"revision-metadata-map", func(def *dataset.Dataset) { def.Metadata["purpose"] = "rewritten" }},
		{"retirement", func(def *dataset.Dataset) { def.Retired = true }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newPublicationFailureFixture(t)
			service := guardedPublicationService(t, fixture)
			ticket := &trackedPublicationProtection{admissionResource: admissionResource{admit: func(context.Context, dataset.Dataset, dataset.PublicationAuthority) error { return nil }}}
			publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
				PrepareResources: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
					mutation.change(&def)
					return def, ticket, nil
				}}
			_, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources, Metadata: map[string]string{"purpose": "original"}})
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("mutated frozen candidate was not rejected before publication: %v", err)
			}
			if ticket.admissions != 0 || ticket.confirmations != 0 || ticket.cancellations != 1 || ticket.closes != 1 || fixture.authority.attempts != 0 {
				t.Fatalf("mutated preparation crossed admission: %+v attempts=%d", ticket, fixture.authority.attempts)
			}
			if !reflect.DeepEqual(dataset.NormalizeState(service.DumpState()), fixture.acceptedState) {
				t.Fatal("preparation mutation changed accepted authority state")
			}
		})
	}
}

func TestDatasetPreparationCanReturnFrozenDependenciesWithoutChangingCandidate(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	ticket := &trackedPublicationProtection{admissionResource: admissionResource{admit: func(_ context.Context, def dataset.Dataset, _ dataset.PublicationAuthority) error {
		if def.Dependencies[fixture.setup.RepositoryID] != fixture.nextCommit {
			t.Fatal("frozen dependency was not admitted")
		}
		return nil
	}}}
	publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
		PrepareResources: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, PublicationProtection, error) {
			def.Dependencies = map[kernel.RepositoryID]kernel.CommitID{fixture.setup.RepositoryID: fixture.nextCommit}
			return def, ticket, nil
		}}
	result, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources})
	if err != nil || result.Dependencies[fixture.setup.RepositoryID] != fixture.nextCommit || ticket.admissions != 1 || ticket.confirmations != 1 || ticket.cancellations != 0 || ticket.closes != 1 {
		t.Fatalf("valid dependency preparation rejected: %#v %+v %v", result, ticket, err)
	}
}
