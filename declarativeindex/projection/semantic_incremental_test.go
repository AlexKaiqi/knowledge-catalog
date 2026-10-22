package projection

import (
	"context"
	"testing"

	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestSemanticOnlyInputChangeIsAnIncrementalDocumentChange(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://test/semantic-only-delta")
	head := testkit.MustHead(t, repo, snapshot.DefaultRef)
	declaration := testkit.IndexMember("semantic", "vector", []string{"semantic"}, "schema/item#/fields/body")
	declaration.Value.(map[string]any)["build"].(map[string]any)["model"] = "model@1"
	at, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/item"}, Value: map[string]any{"entity": "Item", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		declaration, {Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"body": "old semantic input"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	engine := &projectionDeltaEngine{}
	idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	if _, err := idx.Ensure(context.Background(), repo, at); err != nil {
		t.Fatal(err)
	}
	next, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "item/a"}, SchemaRef: "schema/item", Value: map[string]any{"body": "new semantic input"}}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := idx.Ensure(context.Background(), repo, next)
	if err != nil || result.Mode != ports.IndexModeIncremental || result.Updated != 1 || engine.upserts != 1 {
		t.Fatalf("semantic-only change was skipped: %#v upserts=%d %v", result, engine.upserts, err)
	}
}
