package projection_test

import (
	"context"
	"strings"
	"testing"

	index "kc/declarativeindex/projection"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestDatasetBuildAndReadExcludeUnselectedSiblingAspect(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/partial")
	root, _ := repo.Head(snapshot.DefaultRef)
	schema := func(id, aspect, field string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, PathHint: "schemas/" + field + ".yaml", Value: map[string]any{"entity": "Article", "aspect": aspect, "fields": map[string]any{field: map[string]any{"type": "string"}}}}
	}
	declaration := testkit.IndexUnder("indexes", testkit.IndexMember("both-text", "text", []string{"text"}, "schema/public#/fields/name", "schema/private#/fields/secret"))
	declaration.Value.(map[string]any)["inputs"].([]any)[1].(map[string]any)["missing"] = "skip"
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Operations: []objectmodel.Operation{
		schema("schema/public", "properties", "name"), schema("schema/private", "private", "secret"), declaration,
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: "properties"}, SchemaRef: "schema/public", PathHint: "public/name.yaml", Value: map[string]any{"name": "allowed"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "article/one", AspectName: "private"}, SchemaRef: "schema/private", PathHint: "private/secret.yaml", Value: map[string]any{"secret": "forbidden"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	pin := reader.DatasetPin{SetID: "selected", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
	for _, prefix := range []string{"schemas", "indexes", "public"} {
		pin.Items = append(pin.Items, reader.DatasetItem{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: prefix})
	}
	engines := map[kernel.RepositoryID]*datasetTestEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		engine := engines[id]
		if engine == nil {
			engine = &datasetTestEngine{}
			engines[id] = engine
		}
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	private, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("forbidden")))
	if err != nil || len(private.Hits) != 0 || private.Completeness != retrieval.CompletenessComplete {
		t.Fatalf("unselected sibling entered the declared corpus: %#v %v", private, err)
	}
	public, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("allowed")))
	if err != nil || len(public.Hits) != 1 {
		t.Fatalf("selected aspect was dropped with its unselected sibling: %#v %v", public, err)
	}
	body := public.Hits[0].Object.Value.(map[string]any)
	if _, exists := body["private"]; exists {
		t.Fatalf("same-version candidate read leaked private aspect: %#v", body)
	}
	for _, engine := range engines {
		for _, doc := range engine.docs {
			if doc.ObjectID == "article/one" && strings.Join(strings.Fields(doc.Text), " ") != "allowed" {
				t.Fatalf("physical corpus contained an unselected sibling value: %#v", doc)
			}
		}
	}
}

func TestDatasetStatePlanningNormalizesUnambiguousBareFields(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/bare-fields")
	base, _ := repo.Head(snapshot.DefaultRef)
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"name": map[string]any{"type": "string"}}}},
		testkit.IndexField("schema/article", "name", "keyword", "filter", "sort"),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/one"}, SchemaRef: "schema/article", Value: map[string]any{"name": "one"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	pin := reader.DatasetPin{SetID: "bare-fields", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	engines := map[kernel.RepositoryID]*datasetTestEngine{}
	idx := index.NewIndexEngine(t.TempDir(), func(_ string, id kernel.RepositoryID) (ports.Engine, error) {
		engine := engines[id]
		if engine == nil {
			engine = &datasetTestEngine{}
			engines[id] = engine
		}
		return engine, nil
	})
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
		t.Fatal(err)
	}
	result, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchEXISTS("name"), retrieval.SearchSORT("name", "desc")))
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Commit != at {
		t.Fatalf("bare field eligibility failed before its normal planning step: %#v %v", result, err)
	}
}

func TestDatasetPreparationDoesNotRequireContractsForUnindexedUnits(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://dataset/unindexed-units")
	base, _ := repo.Head(snapshot.DefaultRef)
	schema := func(id, path string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, PathHint: path, Value: map[string]any{"entity": "Note", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}
	}
	at, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: []objectmodel.Operation{
		schema("schema/indexed", "selected-schemas/indexed.yaml"), schema("schema/unindexed", "private-schemas/unindexed.yaml"),
		testkit.IndexUnder("indexes", testkit.IndexText("schema/indexed", "body")),
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/indexed"}, PathHint: "objects/indexed.yaml", SchemaRef: "schema/indexed", Value: map[string]any{"body": "selected-token"}},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "note/unindexed"}, PathHint: "objects/unindexed.yaml", SchemaRef: "schema/unindexed", Value: map[string]any{"body": "unindexed-token"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo.Repository, nil }
	t.Run("selected-declaration-missing-schema", func(t *testing.T) {
		pin := reader.DatasetPin{SetID: "missing-schema", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}}
		for _, prefix := range []string{"objects", "indexes"} {
			pin.Items = append(pin.Items, reader.DatasetItem{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: prefix})
		}
		idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return &datasetTestEngine{}, nil })
		t.Cleanup(func() { _ = idx.Close() })
		if _, err := idx.PrepareDataset(context.Background(), lookup, pin); kernel.CodeOf(err) != kernel.ErrSchemaRevisionUnresolved {
			t.Fatalf("selected declaration's missing Schema was silently skipped: %v", err)
		}
	})
	for _, withDeclaration := range []bool{false, true} {
		name := "no-declaration"
		if withDeclaration {
			name = "selected-declaration"
		}
		t.Run(name, func(t *testing.T) {
			pin := reader.DatasetPin{SetID: name, Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): at}, Items: []reader.DatasetItem{{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: "objects"}}}
			if withDeclaration {
				for _, prefix := range []string{"selected-schemas", "indexes"} {
					pin.Items = append(pin.Items, reader.DatasetItem{Repository: repo.ID(), Commit: at, Kind: "prefix", Prefix: prefix})
				}
			}
			engine := &datasetTestEngine{}
			idx := index.NewIndexEngine(t.TempDir(), func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
			t.Cleanup(func() { _ = idx.Close() })
			if _, err := idx.PrepareDataset(context.Background(), lookup, pin); err != nil {
				t.Fatalf("unindexed Schema dependency prevented Dataset preparation: %v", err)
			}
			selected, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("selected-token")))
			if !withDeclaration {
				if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || engine.queries != 0 {
					t.Fatalf("Dataset without selected declarations acquired search access: %#v %v", selected, err)
				}
				return
			}
			if err != nil || len(selected.Hits) != 1 || selected.Hits[0].Object.Address.ObjectID != "note/indexed" {
				t.Fatalf("unindexed unit blocked a selected member: %#v %v", selected, err)
			}
			unindexed, err := idx.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("unindexed-token")))
			if err != nil || len(unindexed.Hits) != 0 || unindexed.Completeness != retrieval.CompletenessComplete {
				t.Fatalf("Schema outside the declaration namespace entered the index: %#v %v", unindexed, err)
			}
		})
	}
}
