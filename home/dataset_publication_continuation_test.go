package home

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	retrieval "kc/discovery"
	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestDatasetContinuationKeepsOriginalCorpusAcrossRepairAndColdReopen(t *testing.T) {
	ws, plain, _ := newDatasetPreparationHome(t, nil)
	receipt, err := ws.Writer.Commit("seed-more-articles-for-pages", objectmodel.CommitChangeSet{
		TargetRepository: plain.id, TargetRef: snapshot.DefaultRef, BaseCommit: plain.at, ExpectedTargetCommit: plain.at,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/two"}, PathHint: "object/two.yaml", SchemaRef: "schema/article-plain", Value: map[string]any{"body": "runbook second"}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "article/three"}, PathHint: "object/three.yaml", SchemaRef: "schema/article-plain", Value: map[string]any{"body": "runbook third"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain.at = receipt.Result.NewCommit
	def, err := datasetPreparationPublisher(ws).Execute(context.Background(), discoverydatasets.DatasetPublication{Dataset: "fixed-pages", Revision: 1, Sources: []dataset.DatasetSource{plain.mapping()}})
	if err != nil {
		t.Fatal(err)
	}
	buildDatasetCorpus(t, ws)
	search := func(host *Home, token string) retrieval.SearchResult {
		t.Helper()
		resolved, err := host.Datasets.ResolveDataset(def.SetID)
		if err != nil {
			t.Fatal(err)
		}
		request := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
		request.Limit = 1
		request.Continuation = token
		result, err := host.Plugins.SearchDataset(context.Background(), func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return host.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		}, DatasetPin(resolved), request)
		if err != nil || len(result.Hits) != 1 {
			t.Fatalf("fixed Dataset page: %#v %v", result, err)
		}
		return result
	}
	first := search(ws, "")
	if first.Continuation == "" {
		t.Fatal("first page did not produce continuation")
	}
	base, id, before := publicationBindingOnMedium(t, ws, def.SetID)
	remove := func() {
		t.Helper()
		if status, response := publicationMediumJSON(t, http.MethodDelete, base+"/_doc/"+url.PathEscape(id), nil); status >= 300 {
			t.Fatalf("remove accepted map: %d %#v", status, response)
		}
	}
	remove()
	if err := ws.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _, after := publicationBindingOnMedium(t, ws, def.SetID)
	if before["generation"] == after["generation"] {
		t.Fatal("maintenance did not actually replace physical generation")
	}
	remove()
	cfg := *ws.Deployment
	cfg.CacheDir = t.TempDir()
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{string(first.Hits[0].Object.Address.ObjectID): true}
	token := first.Continuation
	for token != "" {
		page := search(reopened, token)
		objectID := string(page.Hits[0].Object.Address.ObjectID)
		if seen[objectID] {
			t.Fatalf("continued page repeated %s", objectID)
		}
		seen[objectID] = true
		token = page.Continuation
	}
	for _, id := range []string{"article/plain", "article/two", "article/three"} {
		if !seen[id] {
			t.Fatalf("cold continuation lost original fixed corpus: %#v", seen)
		}
	}
	if status, _ := publicationMediumJSON(t, http.MethodGet, base+"/_doc/"+url.PathEscape(id), nil); status != http.StatusNotFound {
		t.Fatal("continuation repaired or recreated mutable publication map")
	}
}
