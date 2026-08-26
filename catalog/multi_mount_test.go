package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestOneRepositoryCanProjectSeveralDisjointSubtrees(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://acme/team/knowledge")
	cat := testkit.OpenCatalog(t, setup.Store)
	sources := []catalog.WorkspaceSource{
		{Repository: setup.RepositoryID, Selector: "refs/heads/main", Path: catalog.MountPath("docs/team"), SubPath: "handbook"},
		{Repository: setup.RepositoryID, Selector: "refs/heads/main", Path: catalog.MountPath("knowledge/runbooks"), SubPath: "runbooks"},
	}
	def, err := cat.DefineWorkspace("agent", 1, sources)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := cat.ResolveDefinition(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Repositories) != 1 {
		t.Fatalf("one repository must still produce one pin coordinate: %#v", resolved.Repositories)
	}
	mounts, err := catalog.ListVirtualMountsAt(def, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 || mounts[0].Commit != mounts[1].Commit {
		t.Fatalf("mounts must share one commit: %#v", mounts)
	}

	changed := append([]catalog.WorkspaceSource{}, sources...)
	changed[1].Path = catalog.MountPath("knowledge/operations")
	if catalog.HashResolved("agent", sources, resolved.Repositories) == catalog.HashResolved("agent", changed, resolved.Repositories) {
		t.Fatal("all mount paths must participate in PinID")
	}
}

func TestRepeatedRepositoryMustShareCoordinateAndDisjointSubPaths(t *testing.T) {
	setup := testkit.NewSetup(t, "kr://acme/team/knowledge")
	cat := testkit.OpenCatalog(t, setup.Store)
	base := catalog.WorkspaceSource{
		Repository: setup.RepositoryID, Selector: "refs/heads/main", Path: catalog.MountPath("docs/team"), SubPath: "handbook",
	}
	for name, second := range map[string]catalog.WorkspaceSource{
		"selector": {Repository: setup.RepositoryID, Selector: "refs/heads/stable", Path: catalog.MountPath("runbooks"), SubPath: "runbooks"},
		"subpath":  {Repository: setup.RepositoryID, Selector: "refs/heads/main", Path: catalog.MountPath("docs/details"), SubPath: "handbook/details"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cat.DefineWorkspace("bad-"+name, 1, []catalog.WorkspaceSource{base, second})
			testkit.ExpectCode(t, err, kernel.ErrWorkspaceInvalid)
		})
	}
}
