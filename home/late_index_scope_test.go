package home

import (
	"context"
	"testing"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestDatasetPreparationUsesCurrentSourcesWithoutReopeningPlugins(t *testing.T) {
	ws, _, _ := newDatasetPreparationHome(t, nil)
	plugins, access := ws.Plugins, ws.LiveAccess
	if err := plugins.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake := testkit.NewLakeFSFake(t)
	late := seedDatasetPreparationSource(t, fake, "late")
	raw, err := snapshot.OpenLakeFS(late.id, late.dsn, fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if plugins.ConfiguredFor(late.id) {
		t.Fatal("source was admitted before the host registered it")
	}
	if err := ws.Store.Add(raw); err != nil {
		t.Fatal(err)
	}
	if err := ws.Datasets.RegisterRepository(late.id); err != nil {
		t.Fatal(err)
	}
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{
		Dataset: "late-source", Revision: 1, Sources: []dataset.DatasetSource{late.mapping()},
	})
	if err != nil {
		t.Fatalf("active installed instance did not adopt the newly admitted source: %v", err)
	}
	assertDatasetPreparationReads(t, ws, def, late)
	fixed, err := ws.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return ws.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	result, err := plugins.SearchDataset(context.Background(), lookup, DatasetPin(fixed), retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Repository != late.id || result.Hits[0].Object.Commit != late.at {
		t.Fatalf("new source did not retain its prepared Dataset basis: %#v %v", result, err)
	}
	if ws.Plugins != plugins || ws.Index != plugins || ws.LiveAccess != access {
		t.Fatal("source admission reopened the installed runtime")
	}
	ws.Store.Delete(late.id)
	if plugins.ConfiguredFor(late.id) {
		t.Fatal("removed source retained plugin membership")
	}
	if _, err := plugins.SearchDataset(context.Background(), lookup, DatasetPin(fixed), retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("removed source reached the fixed instance: %v", err)
	}
	if closer, ok := raw.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}
