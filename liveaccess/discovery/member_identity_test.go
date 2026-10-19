package discovery_test

import (
	"context"
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
)

var privateMemberAddress = objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "sensor/one", AspectName: "private", MemberKey: "primary"}

func privateMemberQuery(value string) retrieval.SearchRequest {
	return retrieval.SearchOf(retrieval.SearchClause{Op: retrieval.OpEQ, Field: &retrieval.FieldRef{
		Schema: "schema/sensor.private", Aspect: "private", Path: "value",
	}, Value: value})
}

// accept-cross-shared-only-indexed: a per-caller binding unit makes only the
// index members it feeds unsupported. The revision's shared members still
// build and answer completely, and the per-caller unit is never observed under
// the index worker's identity.
func TestDatasetIndexIsolatesPerCallerMemberAndKeepsSharedMembers(t *testing.T) {
	f := newMemberProjectionFixture(t, "mixed")
	unit := string(f.repo.ID()) + ":" + objectmodel.AddressKey(privateMemberAddress)
	assertIsolated := func(label string, spec retrieval.AccessSpec) {
		t.Helper()
		var shared, private bool
		for _, declaration := range spec.Declarations {
			for _, input := range declaration.Inputs {
				switch input.Schema {
				case "schema/sensor.readings":
					shared = declaration.Status == "ready"
				case "schema/sensor.private":
					private = declaration.Status == "unsupported" && strings.Contains(declaration.Reason, unit) &&
						strings.Contains(declaration.Reason, "resource/sensor-private") && strings.Contains(declaration.Reason, "per-caller")
				}
			}
		}
		if !shared || !private {
			t.Fatalf("%s did not isolate the per-caller member alone: %#v", label, spec.Declarations)
		}
	}
	declared, err := f.index.DeclaredAccessSpecAt(f.repo, f.commit)
	if err != nil {
		t.Fatal(err)
	}
	assertIsolated("explanation", declared)
	published, err := f.index.RefreshState(context.Background(), f.repo, f.commit, f.source)
	if err != nil || published.Revision == "" {
		t.Fatalf("shared members did not publish beside a per-caller member: %#v %v", published, err)
	}
	for _, call := range f.origin.calls() {
		if call.Binding.Address != f.address {
			t.Fatalf("index observed a unit other than the shared one: %#v", call)
		}
	}
	if len(f.origin.calls()) != 1 {
		t.Fatalf("shared unit observation count = %d, want 1", len(f.origin.calls()))
	}
	prepared, err := f.index.AccessSpecAt(f.repo, f.commit)
	if err != nil {
		t.Fatal(err)
	}
	assertIsolated("prepared overview", prepared)
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return f.reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	result, err := f.index.SearchDataset(context.Background(), lookup, f.pin, memberQuery("31"))
	if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 ||
		result.Hits[0].Object.Address.ObjectID != f.address.ObjectID {
		t.Fatalf("shared member query is not a complete match: %#v %v", result, err)
	}
	private, err := f.index.SearchDataset(context.Background(), lookup, f.pin, privateMemberQuery("31"))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(private.Hits) != 0 || private.Completeness == retrieval.CompletenessComplete {
		t.Fatalf("per-caller member answered from the shared index: %#v %v", private, err)
	}
	if len(f.origin.calls()) != 1 {
		t.Fatalf("queries reobserved a source: calls=%d", len(f.origin.calls()))
	}
}
