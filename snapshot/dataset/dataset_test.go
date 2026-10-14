package dataset_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestPublishedDatasetPinListsFrozenFileItems(t *testing.T) {
	s := setupFed(t)
	defined, err := s.catalog.DefineDataset("slice", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("policies"), SubPath: "policies"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(defined.Items) != 1 || defined.Items[0].Kind != catalog.DatasetItemPrefix || defined.Items[0].Prefix != "policies" {
		t.Fatalf("publish must store prefix items: %#v", defined.Items)
	}
	if defined.Sources[0].Commit == "" {
		t.Fatal("publish must freeze the source commit")
	}
	resolved, err := s.catalog.ResolveDataset("slice")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Items) != 1 || resolved.Items[0].Commit != defined.Sources[0].Commit {
		t.Fatalf("resolve pin must carry the published file list: %#v", resolved.Items)
	}
	if resolved.Repositories["kr://acme/public/core"] != defined.Sources[0].Commit {
		t.Fatal(resolved.Repositories, defined.Sources[0].Commit)
	}
}

func TestDatasetPathAllowedRootAndPrefix(t *testing.T) {
	root := []catalog.DatasetItem{{
		Target: "kr://acme/public/core", Repository: "kr://acme/public/core", Kind: catalog.DatasetItemPrefix,
	}}
	if !catalog.DatasetPathAllowed(root, "kr://acme/public/core", "any/file.yaml") {
		t.Fatal("root prefix must include every path")
	}
	prefix := []catalog.DatasetItem{{
		Target: "docs", Repository: "kr://acme/public/core", Kind: catalog.DatasetItemPrefix, Prefix: "docs",
	}}
	if !catalog.DatasetPathAllowed(prefix, "kr://acme/public/core", "docs/a.yaml") {
		t.Fatal("prefix must include nested paths")
	}
	if catalog.DatasetPathAllowed(prefix, "kr://acme/public/core", "other/a.yaml") {
		t.Fatal("prefix must exclude sibling paths")
	}
}

func TestDefineDatasetRejectsDuplicateDatasetTargets(t *testing.T) {
	s := setupFed(t)
	_, err := s.catalog.DefineDataset("dup", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("notes")},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main", Path: catalog.MountPath("notes")},
	})
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
}

func TestDatasetPathAllowedEmptyItemsDeny(t *testing.T) {
	if catalog.DatasetPathAllowed(nil, "kr://acme/public/core", "any/file.yaml") {
		t.Fatal("empty dataset list must not admit the whole repository")
	}
}

func TestDefineDatasetFailsClosedWithoutAttachedSnapshot(t *testing.T) {
	_, registry := createCatalogRegistry(t, "kr://empty/catalog")
	cat, err := catalog.NewService(snapshot.NewRegistry(), registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("kr://empty/source"); err != nil {
		t.Fatal(err)
	}
	_, err = cat.DefineDataset("task", 1, []catalog.DatasetSource{
		{Repository: "kr://empty/source", Selector: snapshot.DefaultRef},
	})
	testkit.ExpectCode(t, err, kernel.ErrDatasetInvalid)
}

func TestResolveDatasetExposesVersionRef(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineDataset("named", 2, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.catalog.ResolveDataset("named")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Ref != "v2" || resolved.Revision != 2 {
		t.Fatalf("latest pin must expose vN for the stored revision: %#v", resolved)
	}
}

func TestPublishedPrefixDatasetDoesNotReadSiblingPaths(t *testing.T) {
	s := setupFed(t)
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("note/outside", map[string]any{"body": "secret"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.catalog.DefineDataset("docs", 1, []catalog.DatasetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main", Path: catalog.MountPath("policy"), SubPath: "policy"},
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := testkit.FederatedRead(s.catalog, "docs", "policy/P-103")
	if err != nil || len(listed) != 1 {
		t.Fatalf("prefix dataset must read listed files: %#v %v", listed, err)
	}
	secret, err := testkit.FederatedRead(s.catalog, "docs", "note/outside")
	if err != nil || len(secret) != 0 {
		t.Fatalf("prefix dataset must not read files outside the published list: %#v %v", secret, err)
	}
}
