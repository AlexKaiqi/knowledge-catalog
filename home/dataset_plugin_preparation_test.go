package home

import (
	"context"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

// The deployed instance interprets every source of the deployment, so each
// Dataset publication prepares the fixed basis of each source it selects.
func TestDatasetPublicationPreparesSelectedSourcesThroughDeployedInstance(t *testing.T) {
	ws, plain, indexed := newDatasetPreparationHome(t, nil)
	for _, source := range []datasetPreparationSource{plain, indexed} {
		repo, err := ws.Reader.Require(source.id, kernel.ErrObjectRefUnresolved)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ws.Index.CheckSearchProjectionAt(repo, source.at); err == nil {
			t.Fatalf("fixture already had a ready basis for %s before Dataset preparation", source.id)
		}
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
				t.Fatalf("Dataset publication failed to prepare its selected sources: %v", err)
			}
			assertDatasetPreparationReads(t, ws, def, row.sources...)
			resolved, err := ws.Datasets.ResolveDataset(def.SetID)
			if err != nil {
				t.Fatal(err)
			}
			pin := DatasetPin(resolved)
			lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return ws.Reader.Require(id, kernel.ErrObjectRefUnresolved)
			}
			for _, source := range row.sources {
				repo, err := ws.Plugins.DatasetMember(lookup, pin, source.id)
				if err != nil {
					t.Fatal(err)
				}
				meta, err := ws.Index.CheckSearchProjectionAt(repo, source.at)
				if err != nil || meta.Basis == "" || meta.State != index.ProjectionStateReady {
					t.Fatalf("selected source %s skipped actual index preparation: meta=%#v error=%v", source.id, meta, err)
				}
			}
			result, err := ws.Plugins.SearchDataset(context.Background(), lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
			if err != nil || result.Completeness != retrieval.CompletenessComplete || len(result.Hits) != len(row.sources) || result.Dataset.ID != def.SetID || result.Dataset.Revision != def.Revision {
				t.Fatalf("published Dataset did not query one complete fixed corpus: %#v %v", result, err)
			}
			for _, source := range row.sources {
				found := false
				for _, hit := range result.Hits {
					if hit.Object.Repository == source.id {
						found = kernel.CanonicalDigest(hit.Object) == kernel.CanonicalDigest(source.canonical) && reflect.DeepEqual(hit.Version, retrieval.VersionOf(source.canonical))
					}
				}
				if !found {
					t.Fatalf("Dataset lost exact canonical source %s: %#v", source.id, result)
				}
			}
		})
	}
}

// Without an installed index instance a Dataset still publishes and reads its
// declarations as ordinary content; search explains the missing capability.
func TestDatasetPublicationWithoutIndexInstanceKeepsDeclarationsReadable(t *testing.T) {
	ws, plain, indexed := newDatasetPreparationHome(t, &consumerplugin.PluginsConfig{})
	request := discoverydatasets.DatasetPublication{Dataset: "reading", Revision: 1}
	for _, source := range []datasetPreparationSource{plain, indexed} {
		request.Sources = append(request.Sources, source.mapping())
	}
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("publication required an interpreter that is not installed: %v", err)
	}
	assertDatasetPreparationReads(t, ws, def, plain, indexed)
	repo, err := ws.Reader.Require(indexed.id, kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo
	resolved, err := ws.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ws.Plugins.SearchDataset(context.Background(), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return ws.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, DatasetPin(resolved), retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(result.Hits) != 0 {
		t.Fatalf("search without an installed index instance did not fail closed: result=%#v error=%v", result, err)
	}
}
