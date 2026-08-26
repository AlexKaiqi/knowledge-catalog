package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func TestReaderAspectSelectorOnlyShapesCanonicalRead(t *testing.T) {
	s := testkit.NewSetup(t, "")
	root := testkit.MustHead(t, s.Repo, "")
	commit, err := s.Repo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: "HEAD",
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"}, Value: map[string]any{"pk": []any{"id"}}},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := s.Reader.ReadAddress(s.RepositoryID, knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "t", AspectName: "structure"}, commit)
	if err != nil || unit.Value.(map[string]any)["pk"].([]any)[0] != "id" {
		t.Fatalf("%#v %v", unit, err)
	}
	assembled, err := s.Reader.Read(knowledge.KnowledgeRef{Repository: s.RepositoryID, Object: "t"}, commit, &knowledge.AspectSelector{Exclude: []string{"ownership"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := assembled.Value.(map[string]any)["ownership"]; ok {
		t.Fatalf("%#v", assembled.Value)
	}
}
