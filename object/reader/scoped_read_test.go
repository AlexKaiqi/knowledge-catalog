package reader_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
)

// Reading one unit never widens to its siblings, and reading the whole object
// returns every unit. This replaces the retired aspect selector: narrowing is
// expressed by reading a coordinate, not by reading everything and trimming.
func TestReaderReadsEitherOneCoordinateOrTheWholeObject(t *testing.T) {
	s := testkit.NewSetup(t, "")
	root := testkit.MustHead(t, s.Repo, "")
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/t", AspectName: "structure"}, Value: map[string]any{"pk": []any{"id"}}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/t", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := s.Reader.ReadAddress(s.RepositoryID, objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "fixture/t", AspectName: "structure"}, commit)
	if err != nil || unit.Value.(map[string]any)["pk"].([]any)[0] != "id" {
		t.Fatalf("%#v %v", unit, err)
	}
	assembled, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: "fixture/t"}, commit)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"structure": map[string]any{"pk": []any{"id"}},
		"ownership": map[string]any{"owner": "alice"},
	}
	if !reflect.DeepEqual(assembled.Value, want) {
		t.Fatalf("whole object read dropped or reshaped a unit: %#v", assembled.Value)
	}
	if assembled.Digest != kernel.CanonicalDigest(want) {
		t.Fatalf("whole object read lost its complete digest: %#v", assembled)
	}
}
