package reader_test

import (
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
	"kc/object/writer"
	"kc/snapshot"
)

func TestSchemaAccessCompilationReadsOnlyScopedSchemaUnits(t *testing.T) {
	raw := testkit.MakeTreeStore(t, "kr://reader/schema-access-scope")
	stores := snapshot.NewRegistry()
	if err := stores.Add(raw); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	published, err := w.Commit("publish-scoped-access", objectmodel.ChangeSet{
		TargetRepository: raw.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, PathHint: "public/article-schema.yaml", Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}}}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/private"}, PathHint: "private/schema.yaml", Value: map[string]any{"entity": "Private", "fields": map[string]any{"secret": map[string]any{"type": "string", "access": []any{"filter"}}}}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/a"}, PathHint: "public/article.yaml", Value: map[string]any{"body": "ordinary content"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "index/ordinary"}, PathHint: "public/ordinary-index.yaml", Value: map[string]any{"targets": "ordinary content"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	counting := &locatorCountingRepository{Store: raw, TreeStore: raw.(snapshot.TreeStore), reads: map[string]int{}}
	repo, err := reader.NewReader(nil).Wrap(counting, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{raw.ID(): published.Result.CommitID},
		Items:        []reader.DatasetItem{{Repository: raw.ID(), Commit: published.Result.CommitID, Kind: "prefix", Prefix: "public"}},
	}
	plan, err := retrieval.PlanAccess(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, pin)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Specs) != 1 || len(plan.Specs[0].Schemas) != 1 || len(plan.Specs[0].Fields) != 1 || plan.Specs[0].Fields[0].Schema != "schema/article" {
		t.Fatalf("compiled access escaped the published Schema scope: %#v", plan)
	}
	if counting.listCalls != 0 || counting.reads[repofile.LocatorManifestPath] != 0 {
		t.Fatalf("access compilation scanned the repository: lists=%d reads=%#v", counting.listCalls, counting.reads)
	}
	for _, path := range []string{"private/schema.yaml", "public/article.yaml", "public/ordinary-index.yaml"} {
		if counting.reads[path] != 0 {
			t.Fatalf("access compilation read an unneeded or unpublished body %s", path)
		}
	}
	if counting.reads["public/article-schema.yaml"] != 1 {
		t.Fatalf("published Schema authority not read once: %#v", counting.reads)
	}
	for _, commit := range counting.commits {
		if commit != published.Result.CommitID {
			t.Fatalf("access compilation mixed fixed versions: %s", commit)
		}
	}
}
