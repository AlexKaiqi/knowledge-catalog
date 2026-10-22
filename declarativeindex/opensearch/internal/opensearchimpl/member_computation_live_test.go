package opensearchimpl

import (
	"context"
	"sync/atomic"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

type countedMemberEngine struct {
	*openSearchEngine
	misses *atomic.Int64
}

func (e *countedMemberEngine) ComputeMember(ctx context.Context, key kernel.Digest, compute func() ([]byte, error)) ([]byte, error) {
	return e.openSearchEngine.ComputeMember(ctx, key, func() ([]byte, error) { e.misses.Add(1); return compute() })
}

func TestLiveOpenSearchMemberValuesSurviveColdReopenAndKeepNewSourceBasis(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	repo := testkit.MakeRepository(t, "kr://test/persistent-member-values")
	at := testkit.MustHead(t, repo, snapshot.DefaultRef)
	text := testkit.IndexMember("body", "text", []string{"text"}, "schema/item#/fields/body")
	text.Value.(map[string]any)["build"].(map[string]any)["template"] = "Old {body}"
	apply := func(ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		next, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		at = next
		return next
	}
	apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"body": map[string]any{"type": "string"}, "tag": map[string]any{"type": "string"}}}}, text,
		testkit.IndexMember("tag", "keyword", []string{"filter"}, "schema/item#/fields/tag"),
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"body": "stabletoken", "tag": "A"}})
	directory := t.TempDir()
	var misses atomic.Int64
	open := func() *index.Index {
		opener := Open(Config{URL: e.base, Namespace: e.namespace, PrimaryShards: 1})
		idx := index.NewIndexEngine(directory, func(dir string, id kernel.RepositoryID) (ports.Engine, error) {
			raw, err := opener(dir, id)
			if err != nil {
				return nil, err
			}
			native := raw.(*openSearchEngine)
			native.http.Transport = resources
			return &countedMemberEngine{openSearchEngine: native, misses: &misses}, nil
		})
		t.Cleanup(func() { _ = idx.Close() })
		return idx
	}
	first := open()
	if _, err := first.Ensure(context.Background(), repo, at); err != nil {
		t.Fatal(err)
	}
	if misses.Load() != 4 {
		t.Fatalf("initial field/member computations=%d", misses.Load())
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	newer := apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"body": "stabletoken", "tag": "A", "private": "changed unindexed source value"}})
	cold := open()
	result, err := cold.Ensure(context.Background(), repo, newer)
	if err != nil || result.Updated != 0 || misses.Load() != 4 {
		t.Fatalf("source rewrite recomputed compatible values: %#v misses=%d %v", result, misses.Load(), err)
	}
	if _, err := cold.EnsureAt(context.Background(), repo, newer); err != nil {
		t.Fatal(err)
	}
	if misses.Load() != 4 {
		t.Fatalf("fresh fixed query corpus recomputed members: %d", misses.Load())
	}
	old, err := cold.SearchAt(repo, newer, retrieval.SearchOf(retrieval.SearchMATCH("Old")))
	if err != nil || len(old.Hits) != 1 || old.Hits[0].Object.Commit != newer || old.Hits[0].Object.Repository != repo.ID() {
		t.Fatalf("reused values retained old source: %#v %v", old, err)
	}
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: testkit.IndexDeclarationObject, AspectName: "index", MemberKey: "body"}
	value, err := repo.ReadAddress(address, at)
	if err != nil {
		t.Fatal(err)
	}
	definition := value.Value.(map[string]any)
	definition["build"].(map[string]any)["template"] = "New {body}"
	changed := apply(objectmodel.Operation{Op: objectmodel.OpPut, Address: address, SchemaRef: string(objectmodel.IndexDeclarationSchemaV1), Value: definition})
	if _, err := cold.Ensure(context.Background(), repo, changed); err != nil {
		t.Fatal(err)
	}
	if misses.Load() != 5 {
		t.Fatalf("same-key template change recomputed unrelated members: %d", misses.Load())
	}
	if _, err := cold.EnsureAt(context.Background(), repo, changed); err != nil {
		t.Fatal(err)
	}
	current, err := cold.SearchAt(repo, changed, retrieval.SearchOf(retrieval.SearchMATCH("New")))
	if err != nil || len(current.Hits) != 1 || current.Hits[0].Object.Commit != changed {
		t.Fatalf("new construction/source was not used: %#v %v", current, err)
	}
	retained, err := cold.SearchAt(repo, newer, retrieval.SearchOf(retrieval.SearchMATCH("Old")))
	if err != nil || len(retained.Hits) != 1 || retained.Hits[0].Object.Commit != newer {
		t.Fatalf("new construction changed retained corpus: %#v %v", retained, err)
	}
}
