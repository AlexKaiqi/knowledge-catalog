package dataset_test

import (
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestDatasetRegistryMetadataRequiresStringKeysAndValues(t *testing.T) {
	for _, metadata := range []string{
		"description: 42", "description: true", "description: null", "description: [text]",
		"description: {nested: value}", "42: value", "null", "[]", "description: first\n  description: second",
	} {
		store := seedRegistryFiles(t, map[string]string{
			"registry.yaml":       "authorityId: kr://format/catalog\nrepositories: [table-meta]\n",
			"dataset-orders.yaml": storedDataset + "metadata:\n  " + metadata + "\n",
		})
		before, _ := store.Head(snapshot.DefaultRef)
		if _, err := catalog.OpenSnapshotRegistry(store, ""); kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
			t.Fatalf("registry accepted invalid metadata %q: %v", metadata, err)
		}
		if after, _ := store.Head(snapshot.DefaultRef); after != before {
			t.Fatal("metadata validation mutated the authority")
		}
	}
}

func TestDatasetMetadataLegacyRecoveryPreservesAcceptedRevision(t *testing.T) {
	store := seedRegistryFiles(t, map[string]string{
		"catalog.yaml":        "id: kr://format/catalog\nrepositories: [table-meta]\n",
		"dataset-orders.yaml": storedDataset,
	})
	before, _ := store.Head(snapshot.DefaultRef)
	registry, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	service := catalogFromRegistry(t, registry, "table-meta")
	state := service.DumpState()
	state.KnowledgeSets[0].Metadata = map[string]string{"description": "attempted rewrite"}
	state.DatasetVersions[0].Metadata = map[string]string{"description": "attempted rewrite"}
	if err := registry.Save(state, "legacy rewrite", "", "", ""); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("metadata rewrote a legacy accepted revision: %v", err)
	}
	if head, _ := store.Head(snapshot.DefaultRef); head != before {
		t.Fatal("rejected legacy rewrite migrated the authority")
	}
	def, err := service.PrepareKnowledgeSet("orders", 2, []catalog.KnowledgeSetSource{{
		Repository: "table-meta", Selector: snapshot.DefaultRef, Path: catalog.MountPath("tables"), SubPath: "tables",
	}})
	if err != nil {
		t.Fatal(err)
	}
	def.Metadata = map[string]string{"description": "new revision"}
	if _, err := service.PublishKnowledgeSet(def); err != nil {
		t.Fatalf("metadata prevented first legitimate old-ledger migration: %v", err)
	}
	reopened, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err = reopened.Load()
	if err != nil || state.KnowledgeSets[0].Metadata["description"] != "new revision" {
		t.Fatalf("new metadata lost on legacy recovery: %#v %v", state, err)
	}
	old, err := service.DatasetVersion("orders", 1)
	if err != nil || old.Metadata != nil || old.Sources[0].Commit != "T1" {
		t.Fatalf("legacy release changed during metadata publication: %#v %v", old, err)
	}
	if len(state.DatasetVersions) != 2 || state.DatasetVersions[0].Metadata != nil {
		t.Fatalf("legacy accepted history was synthesized or lost: %#v", state.DatasetVersions)
	}
}
