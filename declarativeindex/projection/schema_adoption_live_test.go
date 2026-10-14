package projection_test

import (
	"kc/declarativeindex/internal/testfixture"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	knowledgemaintenance "kc/object/maintenance"
	"kc/snapshot"
)

type adoptionConsumerRepository struct{ *testkit.KnowledgeRepository }

func (*adoptionConsumerRepository) SchemaReferrerAddresses(objectmodel.ObjectID, kernel.CommitID) ([]objectmodel.Address, error) {
	panic("consumer enumerated adoption dependencies")
}
func (*adoptionConsumerRepository) ScanSnapshotPage(kernel.CommitID, knowledgemaintenance.ScanRequest) (knowledgemaintenance.ScanPage, error) {
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
		at, err := repo.ApplyKnowledgeCommit(objectmodel.ChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef,
			BaseCommit: base, ExpectedTargetCommit: base, Operations: operations})
		if err != nil {
			t.Fatal(err)
		}
		return at
	}
	schema := func(access ...string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/asset"},
			Value: map[string]any{"entity": "Asset", "fields": map[string]any{"title": map[string]any{"type": "string", "access": access}}}}
	}
	old := commit(schema("text"))
	item := func(id, ref string, value any) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, SchemaRef: ref, Value: value}
	}
	head := commit(schema("filter", "sort"),
		item("asset/old", "schema/asset@"+string(old), map[string]any{"title": "oldtoken"}),
		item("asset/old-missing", "schema/asset@"+string(old), map[string]any{}),
		item("asset/current", "schema/asset", map[string]any{"title": "current"}),
		item("asset/current-missing", "schema/asset", map[string]any{}))
	idx := liveIndex(t)
	if _, err := idx.EnsureAt(repo, head); err != nil {
		t.Fatal(err)
	}
	meta, err := idx.CheckSearchProjectionAt(repo, head)
	if err != nil || len(meta.SchemaAdoptions) != 1 || meta.SchemaAdoptions[0].SchemaRef != "schema/asset@"+string(old) {
		t.Fatalf("maintenance did not retain the actual fixed Schema adoption: %#v %v", meta, err)
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
			if hit.Knowledge.Address.ObjectID != expected || hit.Knowledge.Commit != head || hit.Version.DeclarationCommit != head {
				t.Fatalf("%s lost object or version: %#v", query.clause.Op, hit)
			}
		}
	}
}
