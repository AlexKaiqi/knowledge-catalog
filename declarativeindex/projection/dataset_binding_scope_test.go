package projection_test

import (
	"context"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestDatasetStaticSearchIgnoresUnselectedBindingUnits(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/binding-scope")
	base, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	const schema = "schema/shared-properties"
	bound := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/external", AspectName: "properties"}
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema}, PathHint: "schemas/shared.yaml",
				Value: map[string]any{"entity": "Service", "aspect": "properties", "fields": map[string]any{"name": map[string]any{"type": "string"}}}},
			testkit.IndexUnder("indexes", testkit.IndexText(schema, "name")),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "service/selected", AspectName: "properties"},
				SchemaRef: schema, PathHint: "public/selected.yaml", Value: map[string]any{"name": "allowed-token"}},
			bindingfixture.WithPath(bindingfixture.PutDescriptor("resource/external", bindingfixture.Descriptor("https://outside.example")), "private/descriptor.yaml"),
			bindingfixture.WithPath(bindingfixture.Bind(bound, schema, "resource/external"), "private/bound.yaml"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{SetID: "stored-only", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
	for _, prefix := range []string{"schemas", "indexes", "public"} {
		pin.Items = append(pin.Items, reader.DatasetItem{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: prefix})
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	engine := &datasetTestEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	// No observation runtime or state projection is prepared. A bound unit
	// outside this Dataset must not change how its stored name is searched.
	result, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("allowed-token")))
	if err != nil || len(result.Hits) != 1 || engine.queries != 1 {
		t.Fatalf("unselected binding blocked the stored corpus: result=%#v queries=%d error=%v", result, engine.queries, err)
	}
	if hit := result.Hits[0]; hit.Object.Address.ObjectID != "service/selected" || hit.Object.Repository != repo.ID() || hit.Object.Commit != at {
		t.Fatalf("stored hit lost its selected fixed source: %#v", hit)
	}
	if result.Dataset == nil || result.Dataset.ID != pin.SetID || result.Dataset.Revision != pin.Revision || result.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("stored search lost its Dataset basis or completeness: %#v", result)
	}
}
