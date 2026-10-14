package plugin_test

import (
	"context"
	"errors"
	consumerplugin "kc/discovery/plugin"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/discovery/service"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestIndexPluginsSyncOneSourceDoesNotRunAnotherInstancesMaintenance(t *testing.T) {
	f := newPluginFixture(t)
	f.pool.rebuildFailures = map[string]error{"declarative-index/main-a": errors.New("source A physical preparation failed")}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	source := f.setups[1]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	executor := service.ProjectionSyncExecutor{Repositories: pluginRepositoryLookup{f}, Projection: plugins, Controller: plugins}
	result, err := executor.Execute(context.Background(), service.ProjectionSyncRequest{Repository: source.RepositoryID, Commit: at})
	if err != nil {
		t.Fatalf("source B's independent sync ran source A's physical failure: %v", err)
	}
	if result.State != nil || result.Snapshot.Repository != source.RepositoryID || result.Snapshot.BasisCommit != at || result.Snapshot.ObjectCount != 1 || result.Snapshot.AccessDigest == "" {
		t.Fatalf("source B did not actually prepare its source basis: %#v", result)
	}
	query, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	assertPluginHit(t, query, err, source.RepositoryID, at, "runbook source 2 at a")
	targets, err := plugins.Targets()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Repository != source.RepositoryID {
			t.Fatalf("source B's scoped sync created another instance's maintenance target: %#v", target)
		}
	}
	if len(targets) != 1 || targets[0].DesiredCommit != at || targets[0].AppliedCommit != at || targets[0].Status != index.TargetReady {
		t.Fatalf("source B's own maintenance target was not ready: %#v", targets)
	}
	if repositories := f.pool.repositories("declarative-index/main-a"); len(repositories) != 0 {
		t.Fatalf("source B's sync acquired source A's physical resources: %v", repositories)
	}
}

func TestIndexPluginsKeepUnavailableConfigurationFailureLocalToItsSource(t *testing.T) {
	_, invalid := consumerplugin.ParseRepositoryPlugins(map[string]string{consumerplugin.RepositoryPluginsKey: `{"version":1,"plugins":[`})
	if invalid == nil {
		t.Fatal("malformed Repository metadata did not produce the real parser failure")
	}
	for name, fault := range map[string]error{
		"invalid-metadata": invalid,
		"metadata-read":    kernel.Fail(kernel.ErrPreconditionFailed, "source metadata authority unavailable"),
	} {
		t.Run(name, func(t *testing.T) {
			f := newPluginFixture(t)
			a, b := f.setups[0], f.setups[1]
			configs := f.configs()
			delete(configs.Repositories, a.RepositoryID)
			configs.Unavailable = map[kernel.RepositoryID]error{a.RepositoryID: fault}
			entry := f.entry(t)
			plugins, err := consumerplugin.OpenIndexPlugins(configs, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
			if err != nil {
				t.Fatalf("source A's configuration failure blocked independent source B: %v", err)
			}
			t.Cleanup(func() { _ = plugins.Close() })
			atA, atB := testkit.MustHead(t, a.Repo, snapshot.DefaultRef), testkit.MustHead(t, b.Repo, snapshot.DefaultRef)
			repoA, err := f.lookup(a.RepositoryID)
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := repoA.Read("article/one", atA)
			if err != nil || canonical.Repository != a.RepositoryID || canonical.Commit != atA || canonical.Value.(map[string]any)["body"] != "runbook source 1 at a" {
				t.Fatalf("plugin configuration failure blocked A's independent fixed object read: %#v %v", canonical, err)
			}
			query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
			before := f.pool.operations.Load()
			for task, err := range map[string]error{
				"query":   func() error { _, e := plugins.SearchAtContext(context.Background(), repoA, atA, query); return e }(),
				"prepare": plugins.PrepareServingBasis(context.Background(), repoA, atA),
				"sync":    func() error { _, e := plugins.EnsureAt(repoA, atA); return e }(),
			} {
				if !errors.Is(err, fault) {
					t.Errorf("A %s replaced its original unavailable configuration failure: %v", task, err)
				}
			}
			if f.pool.operations.Load() != before {
				t.Fatal("source A's failed configuration opened another source's physical contribution")
			}
			repoB, err := f.lookup(b.RepositoryID)
			if err != nil {
				t.Fatal(err)
			}
			if err := plugins.PrepareServingBasis(context.Background(), repoB, atB); err != nil {
				t.Fatalf("A's failed metadata blocked B's real preparation: %v", err)
			}
			result, err := plugins.SearchAtContext(context.Background(), repoB, atB, query)
			assertPluginHit(t, result, err, b.RepositoryID, atB, "runbook source 2 at a")
			for _, set := range []struct {
				name    string
				sources []dataset.DatasetSource
			}{
				{"only-b", []dataset.DatasetSource{{Repository: b.RepositoryID, BaseRev: string(atB), Selector: snapshot.DefaultRef, Path: dataset.MountPath("b")}}},
				{"with-a", []dataset.DatasetSource{{Repository: a.RepositoryID, BaseRev: string(atA), Selector: snapshot.DefaultRef, Path: dataset.MountPath("a")}, {Repository: b.RepositoryID, BaseRev: string(atB), Selector: snapshot.DefaultRef, Path: dataset.MountPath("b")}}},
			} {
				if _, err := f.cat.DefineDataset(set.name, 1, set.sources); err != nil {
					t.Fatal(err)
				}
				resolved, err := f.cat.ResolveDataset(set.name)
				if err != nil {
					t.Fatal(err)
				}
				pin := pluginDatasetPin(resolved)
				before := f.pool.operations.Load()
				overview, err := plugins.Overview(pin)
				if set.name == "with-a" {
					if !errors.Is(err, fault) || len(overview) != 0 {
						t.Fatalf("mixed overview silently claimed A had no plugin: %#v %v", overview, err)
					}
					if _, err := (service.AccessDescribeExecutor{Repositories: f.lookup, Plugins: plugins}).Execute(context.Background(), service.AccessDescribeRequest{Pin: pin}); !errors.Is(err, fault) {
						t.Fatalf("actual access explanation lost original unknown-configuration failure: %v", err)
					}
				} else if err != nil || len(overview) != 1 || overview[0].Repository != b.RepositoryID || overview[0].Commit != atB || overview[0].Instance != "main-b" || overview[0].Help != "kc plugin declarative-index help" {
					t.Fatalf("A's failed metadata blocked B's actual source explanation: %#v %v", overview, err)
				}
				if f.pool.operations.Load() != before {
					t.Fatal("source explanation performed physical query/maintenance")
				}
			}
			if namespaces := f.pool.namespaces(); len(namespaces) != 1 || namespaces[0] != "declarative-index/main-b" {
				t.Fatalf("A's failed configuration acquired physical resources: %v", namespaces)
			}
			unknownOnly, err := consumerplugin.OpenIndexPlugins(consumerplugin.RepositoryPluginConfigs{Unavailable: configs.Unavailable}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = unknownOnly.Close() })
			if !unknownOnly.Configured() {
				t.Fatal("an unavailable configured source was treated as no required plugin preparation")
			}
			if err := unknownOnly.PrepareServingBasis(context.Background(), repoA, atA); !errors.Is(err, fault) {
				t.Fatalf("unavailable-only assembly skipped or replaced required preparation failure: %v", err)
			}
		})
	}
}
