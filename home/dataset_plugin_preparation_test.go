package home

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	index "kc/discovery/projection"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func TestDatasetPublicationPreparesOnlyConfiguredSourceContributions(t *testing.T) {
	ws, plain, indexed := newDatasetPreparationHome(t, "declarative-index", 0)
	plainRepo, err := ws.Reader.Require(plain.id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Plugins.PrepareServingBasis(context.Background(), plainRepo, plain.at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("direct unconfigured preparation stopped rejecting a missing contribution: %v", err)
	}
	indexedRepo, err := ws.Reader.Require(indexed.id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Index.CheckSearchProjectionAt(indexedRepo, indexed.at); err == nil {
		t.Fatal("fixture already had a ready indexed basis before Dataset preparation")
	}
	for _, row := range []struct {
		name    string
		sources []datasetPreparationSource
	}{
		{"plain-only", []datasetPreparationSource{plain}},
		{"indexed-only", []datasetPreparationSource{indexed}},
		{"mixed", []datasetPreparationSource{plain, indexed}},
	} {
		t.Run(row.name, func(t *testing.T) {
			request := discoverydatasets.DatasetPublication{Dataset: row.name, Revision: 1}
			for _, source := range row.sources {
				request.Sources = append(request.Sources, source.mapping())
			}
			def, err := datasetPreparationPublisher(ws).Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("System's independent index required an unconfigured Dataset source to provide an index: %v", err)
			}
			assertDatasetPreparationReads(t, ws, def, row.sources...)
			if row.name != "plain-only" {
				meta, err := ws.Index.CheckSearchProjectionAt(indexedRepo, indexed.at)
				if err != nil || meta.Basis != indexed.at || meta.State != index.ProjectionStateReady {
					t.Fatalf("configured Dataset source skipped actual index preparation: meta=%#v error=%v", meta, err)
				}
				result, err := ws.Index.SearchAtContext(context.Background(), indexedRepo, indexed.at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
				if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != 1 || !reflect.DeepEqual(result.Hits[0].Object, indexed.canonical) ||
					!reflect.DeepEqual(result.Hits[0].Version, retrieval.VersionOf(indexed.canonical)) {
					t.Fatalf("configured source did not query its actual fixed canonical projection: result=%#v error=%v", result, err)
				}
			}
		})
	}
}

func TestDatasetPublicationKeepsConfigurationFailuresRequired(t *testing.T) {
	for _, row := range []struct {
		name   string
		plugin string
		status int
		code   kernel.ErrorCode
	}{
		{"unknown-entry", "missing-index", 0, kernel.ErrCapabilityUnsatisfied},
		{"metadata-unavailable", "declarative-index", http.StatusServiceUnavailable, kernel.ErrTemporaryUnavailable},
	} {
		t.Run(row.name, func(t *testing.T) {
			ws, plain, failed := newDatasetPreparationHome(t, row.plugin, row.status)
			old, err := ws.Datasets.DefineDataset("reading", 1, []dataset.DatasetSource{plain.mapping()})
			if err != nil {
				t.Fatal(err)
			}
			before := dataset.NormalizeState(ws.Datasets.DumpState())
			head, err := ws.Registry.Head()
			if err != nil {
				t.Fatal(err)
			}
			repo, err := ws.Reader.Require(failed.id, kernel.ErrObjectRefUnresolved)
			if err != nil {
				t.Fatal(err)
			}
			fault := ws.Plugins.PrepareServingBasis(context.Background(), repo, failed.at)
			if kernel.CodeOf(fault) != row.code {
				t.Fatalf("fixture lost the real source configuration failure: %v", fault)
			}
			_, err = datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{
				Dataset: "reading", Revision: 2, Sources: []dataset.DatasetSource{failed.mapping(), plain.mapping()},
			})
			if err == nil || err.Error() != fault.Error() {
				t.Fatalf("Dataset publication skipped a failed source as if it were unconfigured: %v", err)
			}
			if current := dataset.NormalizeState(ws.Datasets.DumpState()); !reflect.DeepEqual(current, before) {
				t.Fatalf("failed required preparation changed Dataset serving state: %#v", current)
			}
			persisted, err := ws.Registry.Load()
			if err != nil || !reflect.DeepEqual(dataset.NormalizeState(persisted), before) {
				t.Fatalf("failed required preparation changed accepted Registry records: state=%#v error=%v", persisted, err)
			}
			if after, err := ws.Registry.Head(); err != nil || after != head {
				t.Fatalf("failed required preparation advanced accepted Registry HEAD: head=%s error=%v", after, err)
			}
			assertDatasetPreparationReads(t, ws, old, plain)
		})
	}
}
