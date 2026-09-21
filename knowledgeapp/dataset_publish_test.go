package knowledgeapp

import (
	"context"
	"errors"
	"kc/catalog"
	"reflect"
	"testing"
)

type publicationRegistry struct {
	events   *[]string
	accepted int
}

func (r *publicationRegistry) PrepareKnowledgeSet(id string, rev int, sources []catalog.KnowledgeSetSource) (catalog.KnowledgeSet, error) {
	*r.events = append(*r.events, "freeze")
	return catalog.KnowledgeSet{SetID: id, Revision: rev, Sources: sources}, nil
}
func (r *publicationRegistry) PublishKnowledgeSet(def catalog.KnowledgeSet) (catalog.KnowledgeSet, error) {
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
				Prepare: func(context.Context, catalog.KnowledgeSet) error {
					events = append(events, "prepare")
					if registry.accepted != 1 {
						t.Fatal("switched before ready")
					}
					if fail {
						return problem
					}
					return nil
				},
			}
			_, err := executor.Execute(context.Background(), DatasetPublication{Dataset: "notes", Revision: 2})
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
	executor := DatasetPublisher{Registry: &publicationRegistry{events: &events}, Authorize: func(context.Context, DatasetPublication) error { return denied }, Prepare: func(context.Context, catalog.KnowledgeSet) error { t.Fatal("prepared denied publication"); return nil }}
	if _, err := executor.Execute(context.Background(), DatasetPublication{}); !errors.Is(err, denied) || len(events) != 0 {
		t.Fatalf("authorization order: %v %v", events, err)
	}
}
