package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/snapshot"
)

type adoptionConsumerRepository struct{ *testkit.ObjectRepository }

func (*adoptionConsumerRepository) SchemaReferrerAddresses(objectmodel.ObjectID, kernel.CommitID) ([]objectmodel.Address, error) {
	panic("consumer enumerated adoption dependencies")
}
func (*adoptionConsumerRepository) ScanSnapshotPage(kernel.CommitID, objectmaintenance.ScanRequest) (objectmaintenance.ScanPage, error) {
	panic("consumer scanned object bodies")
}
func (r *adoptionConsumerRepository) SchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	return r.Repository.(objectmodel.SchemaStore).SchemaObjectIDs(at)
}

func TestPinnedSchemaDiscoveryRetainsVerifiedAccessAfterProjectionReopen(t *testing.T) {
	repo := testfixture.MakeIndexRepository(t, "kr://test/pinned-schema-discovery")
	commit := func(operations ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		base := testkit.MustHead(t, repo, snapshot.DefaultRef)
		at, err := repo.ApplyObjectCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	schema := func(fields ...string) objectmodel.Operation {
		shape := map[string]any{}
		for _, field := range fields {
			shape[field] = map[string]any{"type": "string"}
		}
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/asset"},
			Value: map[string]any{"entity": "Asset", "fields": shape}}
	}
	old := commit(schema("title"))
	current := commit(schema("title", "note"))
	item := func(id, ref string, value any) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, SchemaRef: ref, Value: value}
	}
	oldRef, currentRef := "schema/asset@"+string(old), "schema/asset@"+string(current)
	head := commit(
		testkit.IndexMember("asset-old-title", "text", []string{objectmodel.IndexUseText}, oldRef+"#/fields/title"),
		testkit.IndexMember("asset-current-title", "keyword", []string{objectmodel.IndexUseFilter, objectmodel.IndexUseSort}, currentRef+"#/fields/title"),
		item("asset/old", oldRef, map[string]any{"title": "oldtoken"}),
		item("asset/old-missing", oldRef, map[string]any{}),
		item("asset/current", currentRef, map[string]any{"title": "current"}),
		item("asset/current-missing", currentRef, map[string]any{}))
	idx := liveIndex(t)
	if _, err := idx.EnsureAt(repo, head); err != nil {
		t.Fatal(err)
	}
	meta, err := idx.CheckSearchProjectionAt(repo, head)
	if err != nil {
		t.Fatal(err)
	}
	retained := map[string]bool{}
	for _, adoption := range meta.SchemaAdoptions {
		retained[adoption.SchemaRef] = len(adoption.Witnesses) > 0
	}
	if len(retained) != 3 || !retained[oldRef] || !retained[currentRef] || !retained[string(objectmodel.IndexDeclarationSchemaV1)] {
		t.Fatalf("maintenance did not retain the actual fixed Schema adoptions and declaration members: %#v", meta.SchemaAdoptions)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := liveIndex(t)
	consumer := &adoptionConsumerRepository{repo}
	field := &retrieval.FieldRef{Schema: "schema/asset", Path: "title"}
	for _, query := range []struct {
		clause retrieval.SearchClause
		want   []objectmodel.ObjectID
	}{
		{retrieval.SearchMATCH("oldtoken"), []objectmodel.ObjectID{"asset/old"}},
		{retrieval.SearchClause{Op: retrieval.OpEQ, Field: field, Value: "oldtoken"}, nil},
		{retrieval.SearchClause{Op: retrieval.OpExists, Field: field}, []objectmodel.ObjectID{"asset/current"}},
		{retrieval.SearchClause{Op: retrieval.OpMissing, Field: field}, []objectmodel.ObjectID{"asset/current-missing"}},
	} {
		result, err := reopened.SearchAt(consumer, head, retrieval.SearchOf(query.clause))
		if err != nil || len(result.Hits) != len(query.want) {
			t.Fatalf("%s lost actual access after reopen: %#v %v", query.clause.Op, result, err)
		}
		for i, expected := range query.want {
			hit := result.Hits[i]
			if hit.Object.Address.ObjectID != expected || hit.Object.Commit != head || hit.Version.DeclarationCommit != head {
				t.Fatalf("%s lost object or version: %#v", query.clause.Op, hit)
			}
		}
	}
}
