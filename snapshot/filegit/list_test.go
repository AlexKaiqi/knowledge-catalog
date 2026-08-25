package filegit_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/knowledge"
)

func TestFileGitListAssemblesAllObjectsFromPinnedTree(t *testing.T) {
	repo := testkit.MakeRepository(t, "")
	root := testkit.MustHead(t, repo, "")
	commit := apply(t, repo, root, []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "a", AspectName: "properties"}, Value: map[string]any{"name": "A"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "a", AspectName: "ownership"}, Value: map[string]any{"owner": "alice"}},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "b"}, Value: map[string]any{"name": "B"}},
	}, "", nil)

	values, err := repo.List(commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("List returned %d objects, want 2: %#v", len(values), values)
	}
	got := map[knowledge.ObjectID]any{}
	for _, value := range values {
		if value.Commit != commit {
			t.Fatalf("object %s moved basis to %s, want %s", value.Address.ObjectID, value.Commit, commit)
		}
		got[value.Address.ObjectID] = value.Value
	}
	a, ok := got["a"].(map[string]any)
	if !ok || a["properties"] == nil || a["ownership"] == nil {
		t.Fatalf("assembled aspects = %#v", got["a"])
	}
	if _, ok := got["b"]; !ok {
		t.Fatalf("entity b missing: %#v", got)
	}
}
