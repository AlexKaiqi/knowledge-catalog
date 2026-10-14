package home

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"kc/declarativeindex/opensearch"
	discoverydatasets "kc/discovery/datasets"
	consumerplugin "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

type datasetPreparationSource struct {
	id        kernel.RepositoryID
	dsn       string
	at        kernel.CommitID
	mount     string
	canonical objectmodel.ObjectValue
}

func newDatasetPreparationHome(t *testing.T, indexedPlugin string, metadataStatus int) (*Home, datasetPreparationSource, datasetPreparationSource) {
	t.Helper()
	var failedPath atomic.Value
	failedPath.Store("")
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if metadataStatus != 0 && r.Method == http.MethodGet && r.URL.Path == failedPath.Load().(string) {
			w.WriteHeader(metadataStatus)
			return true
		}
		return false
	})
	plain := seedDatasetPreparationSource(t, fake, "plain", "")
	indexed := seedDatasetPreparationSource(t, fake, "indexed", indexedPlugin)
	failedPath.Store("/api/v1/repositories/" + strings.TrimPrefix(indexed.dsn, fake.Origin()+"/") + "/metadata")
	cfg := deploymentFixture(t)
	cfg.Repositories = append(cfg.Repositories,
		RepositoryBinding{ID: string(plain.id), Driver: "lakefs", DSN: plain.dsn},
		RepositoryBinding{ID: string(indexed.id), Driver: "lakefs", DSN: indexed.dsn},
	)
	namespace := string(kernel.CanonicalDigest(t.TempDir()))
	cfg.Stores.Index = "opensearch"
	cfg.Stores.OpenSearch = opensearch.Config{URL: connectedPluginSearchEndpoint(t, namespace), Namespace: namespace, PrimaryShards: 1}
	// The independent System contribution makes the process have indexed
	// capabilities even when a Dataset's own source has no extension.
	system, err := snapshot.OpenLakeFS(kernel.RepositoryID(cfg.Repositories[0].ID), cfg.Repositories[0].DSN, testkit.LakeFSFakeCredential)
	if err != nil {
		t.Fatal(err)
	}
	writeConnectedPluginConfig(t, system, []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "main"}})
	if err := InitializeDeployment(cfg, func(dir, _ string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	if err := ws.VerifyIndexPlugins(); err != nil {
		t.Fatal(err)
	}
	for _, source := range []datasetPreparationSource{plain, indexed} {
		if err := ws.AttachRepository(source.id); err != nil {
			t.Fatal(err)
		}
	}
	if !ws.DatasetPreparation.Enabled || !ws.Plugins.ConfiguredFor(objectmodel.SystemRepositoryID) || ws.Plugins.ConfiguredFor(plain.id) {
		t.Fatal("fixture must have System indexing enabled and the plain source explicitly unconfigured")
	}
	return ws, plain, indexed
}

func seedDatasetPreparationSource(t *testing.T, fake *testkit.LakeFSFake, mount, plugin string) datasetPreparationSource {
	t.Helper()
	source := datasetPreparationSource{id: kernel.RepositoryID("kr://dataset-preparation/" + mount), mount: mount, dsn: fake.DSN(fake.NewRepo())}
	raw, err := snapshot.OpenLakeFS(source.id, source.dsn, fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	if plugin != "" {
		writeConnectedPluginConfig(t, raw, []consumerplugin.PluginSelection{{Name: plugin, Instance: "main"}})
	}
	repo := testkit.OpenRepository(t, raw)
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	object, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: source.id, TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, PathHint: "object/schema.yaml", Value: map[string]any{
				"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string", "access": []any{"text"}}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID("article/" + mount)}, PathHint: "object/article.yaml", SchemaRef: "schema/article", Value: map[string]any{
				"body": "runbook " + mount, "detail": "complete canonical " + mount,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(raw); err != nil {
		t.Fatal(err)
	}
	writer, err := snapshot.NewRawWriter(stores, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Commit("literal-note", snapshot.TreeChangeSet{
		TargetRepository: source.id, TargetRef: snapshot.DefaultRef, BaseCommit: object, ExpectedTargetCommit: object,
		Changes: []snapshot.TreeChange{{Path: "notes/readme.txt", Content: []byte("literal " + mount)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source.at = receipt.Result.NewCommit
	source.canonical, err = repo.Read(objectmodel.ObjectID("article/"+mount), source.at)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func (s datasetPreparationSource) mapping() dataset.DatasetSource {
	return dataset.DatasetSource{Repository: s.id, Selector: snapshot.DefaultRef, BaseRev: string(s.at), Path: dataset.MountPath(s.mount)}
}

func datasetPreparationPublisher(ws *Home) discoverydatasets.DatasetPublisher {
	return discoverydatasets.DatasetPublisher{Registry: ws.Datasets,
		Authorize: func(ctx context.Context, _ discoverydatasets.DatasetPublication) error { return ctx.Err() },
		Prepare:   ws.DatasetPreparation.Execute,
	}
}

func assertDatasetPreparationReads(t *testing.T, ws *Home, def dataset.Dataset, sources ...datasetPreparationSource) {
	t.Helper()
	resolved, err := ws.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	pin := reader.DatasetPin{SetID: resolved.SetID, Revision: resolved.Revision, Repositories: resolved.Repositories}
	for _, item := range resolved.Items {
		pin.Items = append(pin.Items, reader.DatasetItem{Target: item.Target, Repository: item.Repository, Commit: item.Commit, Kind: item.Kind, Prefix: item.Prefix, File: item.File})
	}
	serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return ws.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, pin)
	for _, source := range sources {
		file, err := ws.Datasets.ReadVirtualFileAt(def, resolved, source.mount+"/notes/readme.txt")
		if err != nil || file.Repository != source.id || file.Commit != source.at || string(file.Content) != "literal "+source.mount {
			t.Fatalf("published Dataset lost fixed literal access: file=%#v error=%v", file, err)
		}
		member, err := serving.Member(source.id)
		if err != nil {
			t.Fatal(err)
		}
		value, err := member.Read(source.canonical.Address.ObjectID, source.at)
		if err != nil || !reflect.DeepEqual(value, source.canonical) {
			t.Fatalf("published Dataset lost independent fixed Object access: value=%#v error=%v", value, err)
		}
	}
	if history, err := ws.Datasets.DatasetVersion(def.SetID, def.Revision); err != nil || !reflect.DeepEqual(history, def) {
		t.Fatalf("successful preparation did not accept the exact Dataset release: release=%#v error=%v", history, err)
	}
}
