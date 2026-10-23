package datasets

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"kc/kernel"
	"kc/snapshot/dataset"
)

type publicationRegistry struct {
	events   *[]string
	accepted int
}

func (r *publicationRegistry) PrepareDataset(id string, rev int, sources []dataset.DatasetSource) (dataset.Dataset, error) {
	*r.events = append(*r.events, "freeze")
	return dataset.Dataset{SetID: id, Revision: rev, Sources: sources}, nil
}

func (r *publicationRegistry) PublishDataset(def dataset.Dataset) (dataset.Dataset, error) {
	*r.events = append(*r.events, "publish")
	r.accepted = def.Revision
	return def, nil
}

// Publication performs no corpus build. Acceptance is a metadata decision, and
// the accepted release's corpus is built afterwards as its own background task,
// so the ordering a publisher can observe is authorize → freeze → validate →
// accept with no build in between.
func TestDatasetPublicationBuildsNoCorpusBeforeAcceptance(t *testing.T) {
	var events []string
	registry := &publicationRegistry{events: &events, accepted: 1}
	executor := DatasetPublisher{Registry: registry,
		Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
		Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
			events = append(events, "validate")
			if registry.accepted != 1 {
				t.Fatal("accepted before validating the frozen candidate")
			}
			return def, nil
		},
	}
	metadata := map[string]string{"custom": "value"}
	published, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2, Metadata: metadata})
	if err != nil || registry.accepted != 2 {
		t.Fatal(err)
	}
	if want := []string{"authorize", "freeze", "validate", "publish"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("publication order: %v want %v", events, want)
	}
	metadata["custom"] = "caller changed"
	if published.Metadata["custom"] != "value" {
		t.Fatalf("accepted metadata lost or aliases caller: %#v", published.Metadata)
	}
}

// A rejected validation never advances the serving version, and it happens
// before anything durable about the corpus exists.
func TestDatasetPublicationRejectsInvalidCandidateBeforeAcceptance(t *testing.T) {
	var events []string
	registry := &publicationRegistry{events: &events, accepted: 1}
	problem := kernel.Fail(kernel.ErrObjectIDConflict, "table/orders takes structure from two sources")
	executor := DatasetPublisher{Registry: registry,
		Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
		Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
			events = append(events, "validate")
			return dataset.Dataset{}, problem
		},
	}
	_, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2})
	if kernel.CodeOf(err) != kernel.ErrObjectIDConflict || registry.accepted != 1 {
		t.Fatalf("invalid candidate changed latest: %v", err)
	}
	if want := []string{"authorize", "freeze", "validate"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("rejected publication order: %v want %v", events, want)
	}
}

func TestDatasetPublicationDeniedBeforeFreeze(t *testing.T) {
	var events []string
	denied := errors.New("denied")
	executor := DatasetPublisher{Registry: &publicationRegistry{events: &events}, Authorize: func(context.Context, DatasetPublication) error { return denied }, Prepare: func(context.Context, dataset.Dataset) (dataset.Dataset, error) {
		t.Fatal("prepared denied publication")
		return dataset.Dataset{}, nil
	}}
	if _, err := executor.Execute(context.Background(), DatasetPublication{}); !errors.Is(err, denied) || len(events) != 0 {
		t.Fatalf("authorization order: %v %v", events, err)
	}
}
