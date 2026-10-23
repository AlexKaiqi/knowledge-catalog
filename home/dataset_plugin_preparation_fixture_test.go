package home

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/declarativeindex/opensearch"
	discoverydatasets "kc/discovery/datasets"
	consumerplugin "kc/discovery/plugin"
	"kc/internal/datasetbridge"
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

func newDatasetPreparationHome(t *testing.T, plugins *consumerplugin.PluginsConfig) (*Home, datasetPreparationSource, datasetPreparationSource) {
	t.Helper()
	fake := testkit.NewLakeFSFake(t)
	plain := seedDatasetPreparationSource(t, fake, "plain")
	indexed := seedDatasetPreparationSource(t, fake, "indexed")
	cfg := deploymentFixture(t)
	cfg.Repositories = append(cfg.Repositories,
		RepositoryBinding{ID: string(plain.id), Driver: "lakefs", DSN: plain.dsn},
		RepositoryBinding{ID: string(indexed.id), Driver: "lakefs", DSN: indexed.dsn},
	)
	cfg.Plugins = plugins
	namespace := string(kernel.CanonicalDigest(t.TempDir()))
	cfg.Stores.Index = "opensearch"
	cfg.Stores.OpenSearch = opensearch.Config{URL: connectedPluginSearchEndpoint(t, namespace), Namespace: namespace, PrimaryShards: 1}
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
	installed := plugins == nil
	if ws.DatasetPreparation.Enabled != installed || ws.Plugins.ConfiguredFor(plain.id) != installed || ws.Plugins.ConfiguredFor(indexed.id) != installed {
		t.Fatalf("fixture must install the deployment's index instance for every source exactly when configured: installed=%t", installed)
	}
	return ws, plain, indexed
}

func seedDatasetPreparationSource(t *testing.T, fake *testkit.LakeFSFake, mount string) datasetPreparationSource {
	t.Helper()
	source := datasetPreparationSource{id: kernel.RepositoryID("kr://dataset-preparation/" + mount), mount: mount, dsn: fake.DSN(fake.NewRepo())}
	raw, err := snapshot.OpenLakeFS(source.id, source.dsn, fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	repo := testkit.OpenRepository(t, raw)
	root := testkit.MustHead(t, raw, snapshot.DefaultRef)
	schema := objectmodel.ObjectID("schema/article-" + mount)
	object, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: source.id, TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: schema}, PathHint: "object/schema.yaml", Value: map[string]any{
				"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}},
			}},
			testkit.IndexOn(objectmodel.ObjectID("index/"+mount), testkit.IndexText(schema, "body")),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID("article/" + mount)}, PathHint: "object/article.yaml", SchemaRef: string(schema), Value: map[string]any{
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

// datasetPreparationPublisher is the publication half only: it accepts an
// immutable release and builds nothing. Corpus work is the accepted release's
// own background task, driven through the controller's catch-up lane.
func datasetPreparationPublisher(ws *Home) discoverydatasets.DatasetPublisher {
	return discoverydatasets.DatasetPublisher{Registry: ws.Datasets,
		Authorize: func(ctx context.Context, _ discoverydatasets.DatasetPublication) error { return ctx.Err() },
		Prepare:   ws.DatasetPreparation.Execute,
	}
}

// buildDatasetCorpus runs the accepted releases' background build to completion,
// which is how a published Dataset becomes searchable.
func buildDatasetCorpus(t *testing.T, ws *Home) {
	t.Helper()
	if err := ws.Plugins.CatchUp(context.Background()); err != nil {
		t.Fatalf("accepted release corpus build failed: %v", err)
	}
}

// buildDatasetRelease builds one accepted release through the same adoption path
// the background lane uses, letting a test wrap the build's ticket. wrap may be
// nil. This is the build half only: the release must already be accepted. It
// takes no testing handle so a concurrent test may call it from a goroutine.
func buildDatasetRelease(ws *Home, key dataset.PublicationLookup, wrap func(discoverydatasets.PublicationProtection) discoverydatasets.PublicationProtection) error {
	authority, ok := ws.Datasets.(dataset.PublicationCoordinator)
	if !ok {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "fixture has no publication authority")
	}
	lookup := func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return ws.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}
	recovery := discoverydatasets.PublicationRecovery{Authority: authority, Definitions: ws.Datasets}
	return recovery.Adopt(context.Background(), key, func(ctx context.Context, accepted dataset.Dataset) (discoverydatasets.PublicationProtection, error) {
		resolved, err := ws.Datasets.ResolveDefinition(accepted)
		if err != nil {
			return nil, err
		}
		ticket, err := ws.Plugins.PrepareProtectedDatasetScope(ctx, lookup, DatasetPin(resolved))
		if err != nil || ticket == nil || wrap == nil {
			return ticket, err
		}
		return wrap(ticket), nil
	})
}

func assertDatasetPreparationReads(t *testing.T, ws *Home, def dataset.Dataset, sources ...datasetPreparationSource) {
	t.Helper()
	resolved, err := ws.Datasets.ResolveDataset(def.SetID)
	if err != nil {
		t.Fatal(err)
	}
	pin := datasetbridge.Pin(resolved)
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
