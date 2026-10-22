package reader_test

import (
	"reflect"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func TestReaderKeepsSameObjectIDInDifferentRepositoriesIndependent(t *testing.T) {
	repositoryA := testkit.MakeTreeStore(t, "kr://reader/identity/a")
	repositoryB := testkit.MakeTreeStore(t, "kr://reader/identity/b")
	registry := snapshot.NewRegistry()
	for _, repository := range []snapshot.Store{repositoryA, repositoryB} {
		if err := registry.Add(repository); err != nil {
			t.Fatal(err)
		}
	}
	w, err := writer.NewWriter(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	const objectID objectmodel.ObjectID = "table/orders"
	put := func(commandID string, repository snapshot.Store, base kernel.CommitID, value any) kernel.CommitID {
		t.Helper()
		receipt, err := w.Commit(commandID, objectmodel.ChangeSet{
			TargetRepository: repository.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base,
			Operations: []objectmodel.Operation{{
				Op:      objectmodel.OpPut,
				Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectID},
				Value:   value,
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.Result.NewCommit
	}
	commitA := put("seed-a", repositoryA, testkit.MustHead(t, repositoryA, snapshot.DefaultRef), map[string]any{"name": "A orders", "owner": "alice"})
	commitB := put("seed-b", repositoryB, testkit.MustHead(t, repositoryB, snapshot.DefaultRef), map[string]any{"name": "B orders", "owner": "bob"})
	rd := reader.NewReader(registry)
	assertRead := func(repository kernel.RepositoryID, commit kernel.CommitID, want map[string]any) {
		t.Helper()
		ref := objectmodel.ObjectRef{Repository: repository, Object: objectID}
		got, err := rd.Read(ref, commit)
		if err != nil {
			t.Fatal(err)
		}
		if got.Repository != repository || got.ObjectRef != ref || got.Commit != commit || got.Address.ObjectID != objectID {
			t.Fatalf("same-name object lost its source identity or fixed basis: %#v", got)
		}
		if !reflect.DeepEqual(got.Value, want) {
			t.Fatalf("repository %s at %s: got %#v, want %#v", repository, commit, got.Value, want)
		}
	}
	assertRead(repositoryA.ID(), commitA, map[string]any{"name": "A orders", "owner": "alice"})
	assertRead(repositoryB.ID(), commitB, map[string]any{"name": "B orders", "owner": "bob"})
	assertRead(repositoryA.ID(), commitA, map[string]any{"name": "A orders", "owner": "alice"})

	updatedA := put("update-a", repositoryA, commitA, map[string]any{"name": "A orders revised", "owner": "carol"})
	if updatedA == commitA || testkit.MustHead(t, repositoryA, snapshot.DefaultRef) != updatedA {
		t.Fatal("updating A did not publish a new A basis")
	}
	if got := testkit.MustHead(t, repositoryB, snapshot.DefaultRef); got != commitB {
		t.Fatalf("updating A moved B Ref: got %s, want %s", got, commitB)
	}
	assertRead(repositoryA.ID(), updatedA, map[string]any{"name": "A orders revised", "owner": "carol"})
	assertRead(repositoryB.ID(), commitB, map[string]any{"name": "B orders", "owner": "bob"})
	assertRead(repositoryA.ID(), commitA, map[string]any{"name": "A orders", "owner": "alice"})
}
