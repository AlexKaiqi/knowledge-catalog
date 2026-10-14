package plugin_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

func TestIndexControlHeadFailureDoesNotAcknowledgeUnfinishedStateNotice(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		name := "head-only"
		if fixed {
			name = "with-fixed-basis"
		}
		t.Run(name, func(t *testing.T) {
			f := newControlRecoveryFixture(t)
			source := f.base.setups[0]
			at, address := controlBoundJob(t, f)
			headFailure := kernel.Fail(kernel.ErrTemporaryUnavailable, "notice HEAD authority unavailable")
			blocked := false
			f.lookup = func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				repo, err := f.base.lookup(id)
				if err == nil && blocked && id == source.RepositoryID {
					repo = failingControlHead(repo, headFailure)
				}
				return repo, err
			}
			runtime := f.open(t, source.RepositoryID)
			head := at
			if fixed {
				f.publish(t, runtime, "notice-fixed-a", source.RepositoryID, at)
				// HEAD and the accepted Dataset must use different State media;
				// recovering only fixed a cannot accidentally satisfy HEAD b.
				head = f.advance(t, 0, "notice-head-b")
			}
			lookup := &controlStateLookup{status: "old-observation", revision: "r1"}
			runtime.plugins.SetObservationSource(controlObservationSource(lookup))
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertControlState(t, f, runtime, lookup, head, "old-observation", "a-owner", "r1")
			evidence := f.accepted(t)
			lookup.mu.Lock()
			lookup.status, lookup.revision = "new-observation", "r2"
			lookup.mu.Unlock()
			blocked = true
			faulted, err := f.lookup(source.RepositoryID)
			if err != nil {
				t.Fatal(err)
			}
			adoptions, ok := faulted.(objectmodel.SchemaAdoptionLocator)
			if !ok {
				t.Fatal("HEAD-only fault removed fixed Schema adoption metadata")
			}
			refs, err := adoptions.AdoptedSchemaRefs(address.ObjectID, at)
			if err != nil || !reflect.DeepEqual(refs, []string{"schema/job.definition"}) {
				t.Fatalf("HEAD-only fault changed actual fixed adoption references: %#v %v", refs, err)
			}
			beforeCalls := len(lookup.requests)
			if err := runtime.plugins.Notify(index.ChangeNotice{Repository: source.RepositoryID, Address: &address}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.plugins.CatchUp(context.Background()); !errors.Is(err, headFailure) {
				t.Errorf("State notice maintenance swallowed its HEAD authority failure: %v", err)
			}
			assertControlState(t, f, runtime, lookup, head, "old-observation", "a-owner", "r1")
			if fixed {
				assertControlState(t, f, runtime, lookup, at, "new-observation", "a-owner", "r2")
				if len(lookup.requests) <= beforeCalls {
					t.Error("HEAD failure blocked the independent fixed State observation")
				}
			} else if len(lookup.requests) != beforeCalls {
				t.Error("unreadable HEAD was replaced by an unrelated declaration basis")
			}
			for _, request := range lookup.requests[beforeCalls:] {
				if request.Binding.DeclarationCommit != at {
					t.Errorf("failed HEAD lane changed its declaration basis: %#v", request)
				}
			}
			f.assertAccepted(t, evidence)
			blocked = false
			// No second notification: the incomplete durable work must survive
			// until this actual authority recovery permits its HEAD observation.
			if err := runtime.plugins.CatchUp(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertControlState(t, f, runtime, lookup, head, "new-observation", "a-owner", "r2")
			f.assertAccepted(t, evidence)
		})
	}
}
