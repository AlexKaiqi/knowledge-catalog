package datasets

import (
	"context"
	"errors"
	"kc/snapshot/dataset"
	"reflect"
	"testing"
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
func TestDatasetPublicationSwitchesOnlyAfterCapabilitiesReady(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "failed"}[fail], func(t *testing.T) {
			var events []string
			registry := &publicationRegistry{events: &events, accepted: 1}
			problem := errors.New("projection unavailable")
			executor := DatasetPublisher{Registry: registry,
				Authorize: func(context.Context, DatasetPublication) error { events = append(events, "authorize"); return nil },
				Prepare: func(_ context.Context, def dataset.Dataset) (dataset.Dataset, error) {
					events = append(events, "prepare")
					if registry.accepted != 1 {
						t.Fatal("switched before ready")
					}
					if fail {
						return dataset.Dataset{}, problem
					}
					return def, nil
				},
			}
			metadata := map[string]string{"custom": "value"}
			published, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2, Metadata: metadata})
			want := []string{"authorize", "freeze", "prepare"}
			if fail {
				if !errors.Is(err, problem) || registry.accepted != 1 {
					t.Fatalf("failed publication changed latest: %v", err)
				}
			} else {
				want = append(want, "publish")
				if err != nil || registry.accepted != 2 {
					t.Fatal(err)
				}
				metadata["custom"] = "caller changed"
				if published.Metadata["custom"] != "value" {
					t.Fatalf("accepted metadata lost or aliases caller: %#v", published.Metadata)
				}
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("order: %v", events)
			}
		})
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
