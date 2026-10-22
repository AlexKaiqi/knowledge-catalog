package home

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestLegacyPublishedDatasetGetsPreparationReceiptFromFixedSourcesAfterColdReopen(t *testing.T) {
	ws, plain, _ := newDatasetPreparationHome(t, nil)
	def, err := ws.Datasets.PrepareDataset("legacy-fixed-release", 1, []dataset.DatasetSource{plain.mapping()})
	if err != nil {
		t.Fatal(err)
	}
	def, err = ws.Datasets.PublishDataset(def)
	if err != nil {
		t.Fatal(err)
	}
	key := dataset.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	receipt, err := ws.Datasets.(dataset.PublicationCoordinator).PublicationReceipt(context.Background(), key)
	if err != nil || receipt.Admission != "" {
		t.Fatalf("fixture is not a legacy pure-file publication: %#v %v", receipt, err)
	}
	// Advance the actual source ref through its Writer; migration must retain the
	// original frozen commit, not prepare the new head as this old release.
	if _, err := ws.TreeWriter.Commit("advance-after-legacy-publication", snapshot.TreeChangeSet{
		TargetRepository: plain.id, TargetRef: snapshot.DefaultRef, BaseCommit: plain.at, ExpectedTargetCommit: plain.at,
		Changes: []snapshot.TreeChange{{Path: "notes/readme.txt", Content: []byte("new source head outside old release")}},
	}); err != nil {
		t.Fatal(err)
	}
	config := *ws.Deployment
	config.CacheDir = t.TempDir()
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	before := kernel.CanonicalDigest(dataset.NormalizeState(reopened.Datasets.DumpState()))
	if err := reopened.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if before != kernel.CanonicalDigest(dataset.NormalizeState(reopened.Datasets.DumpState())) {
		t.Fatal("migration changed original registry definitions or revisions")
	}
	actual, err := reopened.Datasets.DatasetVersion(def.SetID, def.Revision)
	if err != nil || kernel.CanonicalDigest(actual) != kernel.CanonicalDigest(def) {
		t.Fatalf("migration rewrote accepted definition: %#v %v", actual, err)
	}
	authority := reopened.Datasets.(dataset.PublicationCoordinator)
	receipt, err = authority.PublicationReceipt(context.Background(), key)
	if err != nil || receipt.Admission == "" || receipt.Definition != kernel.CanonicalDigest(def) {
		t.Fatalf("migration omitted exact preparation receipt: %#v %v", receipt, err)
	}
	assertQueryUsesPublishedSource(t, reopened, def, plain)
	assertDatasetPreparationReads(t, reopened, def, plain)
	_, _, binding := publicationBindingOnMedium(t, reopened, def.SetID)
	if err := reopened.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	again, err := authority.PublicationReceipt(context.Background(), key)
	if err != nil || again != receipt {
		t.Fatalf("maintenance replaced migration identity: %#v %v", again, err)
	}
	_, _, current := publicationBindingOnMedium(t, reopened, def.SetID)
	if binding["generation"] != current["generation"] {
		t.Fatal("healthy migrated release rebuilt on repeated maintenance")
	}
}
