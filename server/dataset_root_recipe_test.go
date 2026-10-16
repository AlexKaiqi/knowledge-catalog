package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	discoverydatasets "kc/discovery/datasets"
	apphome "kc/home"
	"kc/identity"
	"kc/internal/telemetry"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

func TestDatasetRootRecipeFailuresKeepTheAcceptedRelease(t *testing.T) {
	for _, failure := range []string{"write rejected", "write response lost", "preparation unavailable"} {
		t.Run(failure, func(t *testing.T) {
			f := newRootRecipeFailure(t)
			before := dataset.NormalizeState(f.catalog.DumpState())
			sourceHead := testkit.MustHead(t, f.source, snapshot.DefaultRef)
			authorityHead := testkit.MustHead(t, f.authority, snapshot.DefaultRef)
			unavailable := kernel.Fail(kernel.ErrTemporaryUnavailable, "root recipe test failure")
			switch failure {
			case "write rejected":
				f.writer.failure = unavailable
			case "write response lost":
				f.writer.responseFailure = unavailable
			case "preparation unavailable":
				f.preparationFailure = unavailable
			}
			failed := f.publish(t, http.StatusServiceUnavailable)
			if failed["error"].(map[string]any)["code"] != string(kernel.ErrTemporaryUnavailable) {
				t.Fatalf("writeback failure lost its contract error: %#v", failed)
			}
			if !reflect.DeepEqual(dataset.NormalizeState(f.catalog.DumpState()), before) || testkit.MustHead(t, f.authority, snapshot.DefaultRef) != authorityHead {
				t.Fatal("failed root publication exposed a candidate or moved the authority")
			}
			afterWrite := testkit.MustHead(t, f.source, snapshot.DefaultRef)
			if (afterWrite == sourceHead) != (failure == "write rejected") {
				t.Fatalf("source write outcome was not preserved: before=%s after=%s", sourceHead, afterWrite)
			}
			if failure != "write rejected" {
				tree, _ := snapshot.TreeStoreOf(f.source)
				raw, err := tree.ReadFile(dataset.DatasetFileName, afterWrite)
				if err != nil || !bytes.Equal(raw, f.recipe) {
					t.Fatalf("completed independent recipe write was lost: %q %v", raw, err)
				}
			}
			f.writer.failure, f.preparationFailure = nil, nil
			f.writer.responseFailure = nil
			accepted := f.publish(t, http.StatusOK)
			current, err := f.catalog.Set("files")
			if err != nil || current.Revision != 2 || accepted["recipeCommit"] != string(current.Sources[0].Commit) {
				t.Fatalf("retry did not accept the exact recipe source: %#v %#v %v", accepted, current, err)
			}
			if failure != "write rejected" && testkit.MustHead(t, f.source, snapshot.DefaultRef) != afterWrite {
				t.Fatal("retry duplicated a recipe write whose response had been lost")
			}
		})
	}
}

func TestDatasetRootRecipeUsesTheRequestAuthorizationObserver(t *testing.T) {
	f := newRootRecipeFailure(t)
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "root-recipe-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	f.facade.runtime = runtime
	caller := identity.HTTPIdentity{Principal: "publisher"}
	resources := f.facade.snapshotResources(f.facade.readHome, caller, "dataset-define", false, false)
	definition, err := f.catalog.Set("files")
	if err != nil {
		t.Fatal(err)
	}
	definition.Revision = 2
	if _, _, err := resources.BeforePublish(context.Background(), caller, f.catalog, definition); err != nil {
		t.Fatal(err)
	}
	metrics := httptest.NewRecorder()
	runtime.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, line := range strings.Split(metrics.Body.String(), "\n") {
		if strings.HasPrefix(line, "kc_authorization_decisions_total{") &&
			strings.Contains(line, `kc_operation="dataset-define"`) && strings.Contains(line, `kc_authorization_decision="allow"`) && strings.HasSuffix(line, " 1") {
			return
		}
	}
	t.Fatalf("root write authorization was not recorded by the request runtime:\n%s", metrics.Body.String())
}

type rootRecipeFailure struct {
	dir                string
	source             snapshot.Store
	authority          snapshot.Store
	catalog            dataset.Service
	writer             *rootRecipeWriter
	handler            http.Handler
	facade             *httpFacade
	recipe             []byte
	preparationFailure error
}

type rootRecipeWriter struct {
	snapshot.RawWriter
	failure         error
	responseFailure error
}

func (w *rootRecipeWriter) Commit(command string, changes snapshot.TreeChangeSet) (snapshot.Receipt, error) {
	if w.failure != nil {
		return snapshot.Receipt{}, w.failure
	}
	receipt, err := w.RawWriter.Commit(command, changes)
	if err == nil {
		err = w.responseFailure
	}
	return receipt, err
}

func newRootRecipeFailure(t *testing.T) *rootRecipeFailure {
	t.Helper()
	f := &rootRecipeFailure{dir: t.TempDir(), source: testkit.MakeTreeStore(t, "sales-semantic"), authority: testkit.MakeTreeStore(t, "registry")}
	stores := snapshot.NewRegistry()
	if err := stores.Add(f.source); err != nil {
		t.Fatal(err)
	}
	registry, err := dataset.CreateSnapshotRegistry(f.authority, "")
	if err != nil {
		t.Fatal(err)
	}
	f.catalog, err = dataset.NewService(stores, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.catalog.RegisterRepository(f.source.ID()); err != nil {
		t.Fatal(err)
	}
	root := ""
	sources := []dataset.DatasetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef, Path: &root}}
	if _, err := f.catalog.DefineDataset("files", 1, sources); err != nil {
		t.Fatal(err)
	}
	recipe, ok := dataset.RecipeFromDataset(dataset.Dataset{SetID: "files", Revision: 2, Sources: sources})
	if !ok {
		t.Fatal("root selection has no portable recipe")
	}
	f.recipe, err = dataset.FormatDatasetRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := snapshot.NewRawWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.writer = &rootRecipeWriter{RawWriter: writer}
	if err := inv.WriteAllow(f.dir, inv.AllowFile{Rules: []access.Rule{{ID: "publisher", Principal: "publisher", Actions: []string{"*"}}}}); err != nil {
		t.Fatal(err)
	}
	// This file-only source has no Reader; no object dependencies can exist.
	preparation := discoverydatasets.Preparation{Dependencies: func(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) {
		return nil, f.preparationFailure
	}}
	opened := &Home{Dir: f.dir, Store: stores, Datasets: f.catalog, Registry: registry, TreeWriter: f.writer,
		DatasetPreparation: preparation, File: apphome.HomeFile{Registry: apphome.HomeRegistry{ID: string(f.authority.ID())}}}
	f.facade = &httpFacade{home: f.dir, readHome: opened, options: HTTPServerOptions{AuthMode: "local"}}
	f.handler = f.facade.snapshotStoreService()
	return f
}

func (f *rootRecipeFailure) publish(t *testing.T, status int) map[string]any {
	t.Helper()
	root := ""
	raw, err := json.Marshal(map[string]any{"dataset": "files", "revision": 2,
		"sources": []dataset.DatasetSource{{Repository: f.source.ID(), Selector: snapshot.DefaultRef, Path: &root}}})
	if err != nil {
		t.Fatal(err)
	}
	return grantRequestForTest(t, f.handler, "publisher", http.MethodPost, "/snapshot/v1/datasets", string(raw), status)
}
