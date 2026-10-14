package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	consumerplugin "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestHomeAssemblyFailureReleasesAcquiredAuthorityHandles(t *testing.T) {
	for _, stage := range []string{"hydration", "plugin-controller", "object-coordinator"} {
		t.Run(stage, func(t *testing.T) {
			fake := testkit.NewLakeFSFake(t)
			source, err := snapshot.OpenLakeFS("kr://lifecycle/source", fake.DSN(fake.NewRepo()), fake.Credential())
			if err != nil {
				t.Fatal(err)
			}
			writeConnectedPluginConfig(t, source, []consumerplugin.PluginSelection{{Name: "declarative-index", Instance: "main"}})
			sourceBefore := testkit.MustHead(t, source, snapshot.DefaultRef)
			store := snapshot.NewRegistry()
			if err := store.Add(source); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			authority, err := snapshot.OpenLakeFS("kr://lifecycle/registry", fake.DSN(fake.NewRepo()), fake.Credential())
			if err != nil {
				t.Fatal(err)
			}
			registry, err := dataset.CreateSnapshotRegistry(authority, snapshot.DefaultRef)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = registry.Close() })
			cat, err := dataset.NewService(store, registry)
			if err != nil {
				t.Fatal(err)
			}
			before := testkit.MustHead(t, authority, snapshot.DefaultRef)
			home := t.TempDir()
			projections := filepath.Join(home, "projections")
			settings := StoresFile{Index: "none", Layout: LayoutFile{Projections: projections}}
			want := "cache limits cannot be negative"
			if stage == "hydration" {
				settings.HydrationCache = &HydrationCacheConfig{MaxEntries: -1}
			} else {
				want = "controller.db"
				if stage == "object-coordinator" {
					want = "object-maintenance.db"
				}
				if err := os.MkdirAll(filepath.Join(projections, want), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ws, err := assemble(home, HomeFile{Registry: HomeRegistry{ID: registry.AuthorityID()}}, settings, store, cat, registry, false)
			if ws != nil {
				_ = ws.Close()
			}
			if err == nil || !strings.Contains(err.Error(), want) || ws != nil {
				t.Fatalf("did not reach the selected real assembly failure: home=%v error=%v want=%s", ws, err, want)
			}
			if stage == "object-coordinator" {
				if info, err := os.Stat(filepath.Join(projections, "controller.db")); err != nil || !info.Mode().IsRegular() {
					t.Fatalf("Object Coordinator failed before the actual plugin acquired its control store: %v", err)
				}
			}
			if ids := store.IDs(); len(ids) != 0 {
				t.Errorf("failed assembly retained its actual authority directory: %v", ids)
			}
			if head := testkit.MustHead(t, authority, snapshot.DefaultRef); head != before {
				t.Fatalf("failed assembly changed accepted Registry authority: %s -> %s", before, head)
			}
			if sourceHead := testkit.MustHead(t, source, snapshot.DefaultRef); sourceHead != sourceBefore {
				t.Fatalf("failed assembly changed source authority: %s", sourceHead)
			}
		})
	}
}
