package reader_test

import (
	"testing"

	"kc/internal/testkit"
	objectmodel "kc/object"
)

func TestReaderAspectSelectorOnlyShapesCanonicalRead(t *testing.T) {
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
	assembled, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: "fixture/t"}, commit, &objectmodel.AspectSelector{Exclude: []string{"ownership"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := assembled.Value.(map[string]any)["ownership"]; ok {
		t.Fatalf("%#v", assembled.Value)
	}
}
