package datasets

import (
	"context"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// Validation may not rewrite the frozen candidate, not even through the slices,
// pointers and maps it shares with the registry's frozen value.
func TestDatasetValidationCannotMutateFrozenCandidateThroughSharedValues(t *testing.T) {
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
			publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
				Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
					mutation.change(&def)
					return def, nil
				}}
			_, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources, Metadata: map[string]string{"purpose": "original"}})
			if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("mutated frozen candidate was not rejected before publication: %v", err)
			}
			if fixture.authority.attempts != 0 {
				t.Fatalf("mutated validation reached authority CAS: %d", fixture.authority.attempts)
			}
			if !reflect.DeepEqual(dataset.NormalizeState(service.DumpState()), fixture.acceptedState) {
				t.Fatal("validation mutation changed accepted authority state")
			}
		})
	}
}

// Frozen reference dependencies are the one validation output allowed to reach
// the accepted release.
func TestDatasetValidationCanReturnFrozenDependenciesWithoutChangingCandidate(t *testing.T) {
	fixture := newPublicationFailureFixture(t)
	service := guardedPublicationService(t, fixture)
	publisher := DatasetPublisher{Registry: service, Authorize: func(context.Context, DatasetPublication) error { return nil },
		Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
			if def.Dependencies[fixture.setup.RepositoryID] == fixture.nextCommit {
				t.Fatal("validation received dependencies it was supposed to freeze")
			}
			def.Dependencies = map[kernel.RepositoryID]kernel.CommitID{fixture.setup.RepositoryID: fixture.nextCommit}
			return def, nil
		}}
	result, err := publisher.Execute(context.Background(), DatasetPublication{Dataset: "reading", Revision: 2, Sources: fixture.sources})
	if err != nil || result.Dependencies[fixture.setup.RepositoryID] != fixture.nextCommit || result.Revision != 2 {
		t.Fatalf("valid dependency validation rejected: %#v %v", result, err)
	}
}
