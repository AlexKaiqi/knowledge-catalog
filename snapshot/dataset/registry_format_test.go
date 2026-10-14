package dataset_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestCatalogMembershipLayoutInitializesChangesAndRecovers(t *testing.T) {
	const id = "kr://format/catalog"
	store := newCatalogMem(id)
	reg, err := catalog.CreateSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	initial, _ := reg.Head()
	assertCatalogFiles(t, store, initial, "authorityId: kr://format/catalog\nrepositories: []\n")
	cat, err := catalog.NewService(snapshot.NewRegistry(), reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []kernel.RepositoryID{"table-meta", "kr://kc/system"} {
		if err := cat.RegisterRepository(member); err != nil {
			t.Fatal(err)
		}
	}
	attached, _ := reg.Head()
	assertCatalogFiles(t, store, attached, "authorityId: kr://format/catalog\nrepositories:\n  - kr://kc/system\n  - table-meta\n")
	if err := cat.UnregisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	head, _ := reg.Head()
	assertCatalogFiles(t, store, head, "authorityId: kr://format/catalog\nrepositories:\n  - kr://kc/system\n")
	assertCatalogFiles(t, store, attached, "authorityId: kr://format/catalog\nrepositories:\n  - kr://kc/system\n  - table-meta\n")
	reopened, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AuthorityID != id || !reflect.DeepEqual(state.Repositories, []string{"kr://kc/system"}) {
		t.Fatalf("recovery lost accepted state: %#v", state)
	}
	if got, _ := reopened.Head(); got != head {
		t.Fatal("recovery rewrote authority")
	}
}

func assertCatalogFiles(t *testing.T, store *catalogMem, head, want string) {
	t.Helper()
	paths, err := store.ListFiles(kernel.CommitID(head))
	if err != nil || !reflect.DeepEqual(paths, []string{"registry.yaml"}) {
		t.Fatalf("membership layout: %v %v", paths, err)
	}
	body, err := store.ReadFile("registry.yaml", kernel.CommitID(head))
	if err != nil || string(body) != want {
		t.Fatalf("registry.yaml:\n%s\nwant:\n%s\n%v", body, want, err)
	}
}

func seedRegistryFiles(t *testing.T, files map[string]string) *catalogMem {
	t.Helper()
	store := newCatalogMem("kr://format/catalog")
	head, _ := store.Head(snapshot.DefaultRef)
	cs := snapshot.TreeChangeSet{TargetRepository: store.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head}
	for path, body := range files {
		cs.Changes = append(cs.Changes, snapshot.TreeChange{Path: path, Content: []byte(body)})
	}
	if _, err := store.ApplyTreeCommit(cs); err != nil {
		t.Fatal(err)
	}
	return store
}

const storedDataset = "setId: orders\nrevision: 1\nsources:\n  - repository: table-meta\n    selector: refs/heads/main\n    commit: T1\n    path: tables\n    subPath: tables\n"

func TestCatalogRegistryRejectsMalformedRecords(t *testing.T) {
	const goodCatalog = "id: kr://format/catalog\nrepositories: [table-meta]\n"
	control := seedRegistryFiles(t, map[string]string{"catalog.yaml": goodCatalog, "dataset-orders.yaml": storedDataset})
	if _, err := catalog.OpenSnapshotRegistry(control, ""); err != nil {
		t.Fatalf("valid control rejected: %v", err)
	}
	cases := []struct{ name, path, body string }{
		{"missing id", "catalog.yaml", "repositories: []\n"},
		{"unknown metadata", "catalog.yaml", goodCatalog + "owner: admin\n"},
		{"duplicate YAML key", "catalog.yaml", goodCatalog + "id: other\n"},
		{"wrong archived type", "catalog.yaml", goodCatalog + "archived: 'true'\n"},
		{"null id", "catalog.yaml", "id: null\nrepositories: []\n"},
		{"null memberships", "catalog.yaml", "id: kr://format/catalog\nrepositories: null\n"},
		{"wrong membership type", "catalog.yaml", "id: kr://format/catalog\nrepositories: [42]\n"},
		{"timestamp membership", "catalog.yaml", "id: kr://format/catalog\nrepositories: [2026-10-10]\n"},
		{"binary membership", "catalog.yaml", "id: kr://format/catalog\nrepositories: [!!binary dGFibGUtbWV0YQ==]\n"},
		{"empty membership", "catalog.yaml", "id: kr://format/catalog\nrepositories: [' ']\n"},
		{"duplicate membership", "catalog.yaml", "id: kr://format/catalog\nrepositories: [table-meta, table-meta]\n"},
		{"multiple documents", "catalog.yaml", goodCatalog + "---\nid: another\n"},
		{"wrong current filename", "dataset-wrong.yaml", storedDataset},
		{"missing dataset identity", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "setId: orders\n", "")},
		{"invalid revision", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "revision: 1", "revision: 0")},
		{"wrong revision type", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "revision: 1", "revision: '1'")},
		{"floating revision", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "revision: 1", "revision: 1.0")},
		{"empty sources", "dataset-orders.yaml", "setId: orders\nrevision: 1\nsources: []\n"},
		{"unfrozen source", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "    commit: T1\n", "")},
		{"missing selector", "dataset-orders.yaml", strings.ReplaceAll(storedDataset, "    selector: refs/heads/main\n", "")},
		{"unknown source", "dataset-orders.yaml", storedDataset + "    extra: true\n"},
		{"null optional flag", "dataset-orders.yaml", storedDataset + "retired: null\n"},
		{"inconsistent items", "dataset-orders.yaml", storedDataset + "items:\n  - repository: table-meta\n    commit: T2\n    kind: prefix\n    prefix: tables\n    target: tables\n"},
		{"wrong release filename", "release-wrong-v1.yaml", storedDataset},
		{"legacy unknown field", "repository-table-meta.yaml", "repository: table-meta\nsecret: hidden\n"},
		{"legacy missing identity", "repository-table-meta.yaml", "repository: ''\n"},
		{"legacy filename mismatch", "repository-wrong.yaml", "repository: table-meta\n"},
		{"legacy ambiguous identity", "repository-table-meta.yaml", "repository: table-meta\nrepositoryId: table-meta\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"catalog.yaml": goodCatalog}
			if strings.HasPrefix(tc.path, "repository-") {
				files["catalog.yaml"] = "id: kr://format/catalog\n"
			}
			files[tc.path] = tc.body
			store := seedRegistryFiles(t, files)
			before, _ := store.Head(snapshot.DefaultRef)
			if _, err := catalog.OpenSnapshotRegistry(store, ""); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
				t.Fatalf("malformed registry must return DATASET_INVALID: %v", err)
			}
			if after, _ := store.Head(snapshot.DefaultRef); after != before {
				t.Fatal("validation mutated authority")
			}
		})
	}
}

func TestCatalogRegistryLegacyReadIsReadOnlyAndNextChangeConsolidates(t *testing.T) {
	store := seedRegistryFiles(t, map[string]string{
		"catalog.yaml":               "id: kr://format/catalog\n",
		"repository-table-meta.yaml": "repository: table-meta\n",
	})
	before, _ := store.Head(snapshot.DefaultRef)
	reg, err := catalog.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(snapshot.NewRegistry(), reg)
	if err != nil {
		t.Fatal(err)
	}
	if !cat.HasRepository("table-meta") {
		t.Fatal("legacy member lost")
	}
	if after, _ := store.Head(snapshot.DefaultRef); after != before {
		t.Fatal("legacy recovery migrated authority")
	}
	if err := cat.RegisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	if after, _ := store.Head(snapshot.DefaultRef); after != before {
		t.Fatal("idempotent registration migrated authority")
	}
	if err := cat.RegisterRepository("sales-semantic"); err != nil {
		t.Fatal(err)
	}
	head, _ := reg.Head()
	assertCatalogFiles(t, store, head, "authorityId: kr://format/catalog\nrepositories:\n  - sales-semantic\n  - table-meta\n")
	if _, err := store.ReadFile("repository-table-meta.yaml", before); err != nil {
		t.Fatal("legacy history lost", err)
	}
}

func TestCatalogRegistryRejectsAmbiguousLayoutAndPublication(t *testing.T) {
	const metadata = "id: kr://format/catalog\nrepositories: [table-meta]\n"
	release := fmt.Sprintf("release-%x-v1.yaml", sha256.Sum256([]byte("orders")))
	cases := []struct {
		name  string
		files map[string]string
	}{
		{"mixed memberships", map[string]string{"catalog.yaml": metadata, "repository-table-meta.yaml": "repository: table-meta\n"}},
		{"missing metadata", map[string]string{"dataset-orders.yaml": storedDataset}},
		{"current differs from release", map[string]string{"catalog.yaml": metadata, "dataset-orders.yaml": strings.ReplaceAll(storedDataset, "path: tables", "path: other"), release: storedDataset}},
		{"release contains policy", map[string]string{"catalog.yaml": metadata, release: storedDataset + "retired: true\n"}},
		{"current precedes release", map[string]string{"catalog.yaml": metadata, "dataset-orders.yaml": storedDataset, fmt.Sprintf("release-%x-v2.yaml", sha256.Sum256([]byte("orders"))): strings.ReplaceAll(storedDataset, "revision: 1", "revision: 2")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := seedRegistryFiles(t, tc.files)
			if _, err := catalog.OpenSnapshotRegistry(store, ""); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
				t.Fatalf("malformed state accepted: %v", err)
			}
		})
	}
}

func TestSnapshotRegistryReleaseLifecycleKeepsPublishedBytes(t *testing.T) {
	const id = "kr://format/catalog"
	authority := newCatalogMem(id)
	reg, err := catalog.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	source := newCatalogMem("table-meta")
	stores := snapshot.NewRegistry()
	if err := stores.Add(source); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(stores, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "table-meta", Selector: snapshot.DefaultRef, Path: catalog.MountPath("tables")}}
	if _, err := cat.DefineDataset("orders", 1, sources); err != nil {
		t.Fatal(err)
	}
	v1, _ := reg.Head()
	release := fmt.Sprintf("release-%x-v1.yaml", sha256.Sum256([]byte("orders")))
	accepted, err := authority.ReadFile(release, kernel.CommitID(v1))
	if err != nil {
		t.Fatal(err)
	}
	beforeSource, _ := source.Head(snapshot.DefaultRef)
	if _, err := source.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: beforeSource, ExpectedTargetCommit: beforeSource, Changes: []snapshot.TreeChange{{Path: "table.txt", Content: []byte("new version")}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cat.DefineDataset("orders", 2, sources); err != nil {
		t.Fatal(err)
	}
	if err := cat.RetireDataset("orders"); err != nil {
		t.Fatal(err)
	}
	if err := cat.UnregisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	head, _ := reg.Head()
	unchanged, err := authority.ReadFile(release, kernel.CommitID(head))
	if err != nil || !bytes.Equal(accepted, unchanged) {
		t.Fatal("accepted v1 changed", err)
	}
	paths, _ := authority.ListFiles(kernel.CommitID(head))
	if len(paths) != 4 {
		t.Fatalf("expected metadata, current and two releases: %v", paths)
	}
	reopened, err := catalog.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.Load()
	if err != nil || len(state.Repositories) != 0 || len(state.DatasetVersions) != 2 || !state.Datasets[0].Retired {
		t.Fatalf("lifecycle recovery: %#v %v", state, err)
	}
	if got, _ := reopened.Head(); got != head {
		t.Fatal("lifecycle recovery rewrote authority")
	}
	if state.DatasetVersions[0].Sources[0].Commit != beforeSource || state.DatasetVersions[1].Sources[0].Commit == beforeSource {
		t.Fatal("releases lost their fixed source coordinates", state.DatasetVersions)
	}
}

func TestSnapshotRegistryRejectsDatasetFilenameCollisionsWithoutCommit(t *testing.T) {
	authority := newCatalogMem("kr://format/catalog")
	reg, err := catalog.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(newCatalogMem("table-meta")); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(stores, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	sources := []catalog.DatasetSource{{Repository: "table-meta", Selector: snapshot.DefaultRef}}
	if _, err := cat.DefineDataset("orders:a", 1, sources); err != nil {
		t.Fatal(err)
	}
	head, _ := reg.Head()
	if _, err := cat.DefineDataset("orders/a", 1, sources); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatal("colliding identities accepted", err)
	}
	if got, _ := reg.Head(); got != head || len(cat.DumpState().Datasets) != 1 {
		t.Fatal("rejected collision changed accepted state")
	}
}

func TestSnapshotRegistryPreservesIntegerRevisionExactly(t *testing.T) {
	authority := newCatalogMem("kr://format/catalog")
	reg, err := catalog.CreateSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal(err)
	}
	stores := snapshot.NewRegistry()
	if err := stores.Add(newCatalogMem("table-meta")); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.NewService(stores, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.RegisterRepository("table-meta"); err != nil {
		t.Fatal(err)
	}
	revision := int(^uint(0) >> 1)
	wideRevision := int64(1)<<53 + 1
	if int64(int(wideRevision)) == wideRevision {
		revision = int(wideRevision)
	}
	if _, err := cat.DefineDataset("orders", revision, []catalog.DatasetSource{{Repository: "table-meta", Selector: snapshot.DefaultRef}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := catalog.OpenSnapshotRegistry(authority, "")
	if err != nil {
		t.Fatal("accepted integer revision became unreadable", err)
	}
	state, err := reopened.Load()
	if err != nil || state.Datasets[0].Revision != revision || state.DatasetVersions[0].Revision != revision {
		t.Fatalf("integer revision lost precision: %#v %v", state, err)
	}
}
