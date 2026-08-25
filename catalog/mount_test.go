package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

// mountFed mounts two plain members under fixed ids so mount tests can
// declare Path without caring about knowledge content.
func mountFed(t *testing.T) *catalog.Catalog {
	t.Helper()
	store := snapshot.NewRegistry()
	for _, id := range []string{"kr://acme/public/core", "kr://acme/public/core2"} {
		if err := store.Add(testkit.MakeRepository(t, id)); err != nil {
			t.Fatal(err)
		}
	}
	return testkit.OpenCatalog(t, store)
}

// A recipe where no source declares Path is untouched: it is the pre-Loom
// federated-read shape and must keep working without any mount validation.
func TestDefineWorkspaceWithoutPathIsUnaffected(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "v")
	if _, err := catalog.RouteMount(def, "any/file.md"); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("a recipe with no declared mount path must refuse routing, got %v", err)
	}
}

// Invariant 1: once any source declares Path, every source must (root is an
// explicit Path of "", not an absent one).
func TestDefineWorkspaceRejectsPartialMountDeclaration(t *testing.T) {
	cat := mountFed(t)
	_, err := cat.DefineWorkspace("mixed", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main"},
	})
	testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
}

// Invariant 2: two mounts cannot claim the same path, and one cannot nest
// inside another — path ownership must stay unique in both directions.
func TestDefineWorkspaceRejectsCollidingPaths(t *testing.T) {
	cat := mountFed(t)
	_, err := cat.DefineWorkspace("dup-path", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	})
	testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
}

func TestDefineWorkspaceRejectsNestedPaths(t *testing.T) {
	cat := mountFed(t)
	_, err := cat.DefineWorkspace("nested", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("refs")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	})
	testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
}

// Root does not conflict with anything: it is the fallback, not a prefix.
func TestDefineWorkspaceAcceptsRootAlongsideNestedMount(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("alice-notes", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "alice-notes")

	route, err := catalog.RouteMount(def, "analysis/churn.md")
	if err != nil {
		t.Fatal(err)
	}
	if route.Repository != "kr://acme/public/core" || route.Path != "analysis/churn.md" {
		t.Fatalf("root mount must own unmatched paths verbatim: %#v", route)
	}

	route, err = catalog.RouteMount(def, "refs/semantic/metrics/dau.md")
	if err != nil {
		t.Fatal(err)
	}
	if route.Repository != "kr://acme/public/core2" || route.Path != "metrics/dau.md" {
		t.Fatalf("nested mount must strip its own prefix, not the root's: %#v", route)
	}
}

// SubPath adds back a prefix inside the member repository: mounting only
// docs/knowledge/ from a monorepo, the in-repo path must carry that prefix.
func TestRouteMountAppliesSubPath(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("sub", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main",
			Path: catalog.MountPath("kb"), SubPath: "docs/knowledge"},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "sub")
	route, err := catalog.RouteMount(def, "kb/metrics/dau.md")
	if err != nil {
		t.Fatal(err)
	}
	if route.Path != "docs/knowledge/metrics/dau.md" {
		t.Fatalf("subPath must be reattached: %#v", route)
	}
}

// A path nobody's mount covers, and no root exists to catch it, is refused —
// silently routing it somewhere would violate unique path ownership.
func TestRouteMountRejectsUnownedPath(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("no-root", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "no-root")
	_, err := catalog.RouteMount(def, "analysis/churn.md")
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
}

// Cross-mount edits split into one batch per repository (K-01): the caller
// gets N groups to COMMIT independently, never one write spanning repos.
func TestRouteMountsSplitsByRepository(t *testing.T) {
	cat := mountFed(t)
	if _, err := cat.DefineWorkspace("split", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("")},
		{Repository: "kr://acme/public/core2", Selector: "refs/heads/main", Path: catalog.MountPath("refs/semantic")},
	}); err != nil {
		t.Fatal(err)
	}
	def := mustWorkspace(t, cat, "split")
	groups, err := catalog.RouteMounts(def, []string{
		"analysis/churn.md",
		"analysis/retention.md",
		"refs/semantic/metrics/dau.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 repository groups, got %#v", groups)
	}
	if len(groups["kr://acme/public/core"]) != 2 || len(groups["kr://acme/public/core2"]) != 1 {
		t.Fatalf("unexpected grouping: %#v", groups)
	}
}

func mustWorkspace(t *testing.T, cat *catalog.Catalog, workspaceID string) catalog.WorkspaceDefinition {
	t.Helper()
	def, err := cat.Workspace(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return def
}
