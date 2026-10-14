package dataset_test

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

func TestLegacyRegistryRecoveryPreservesHistoryAndBindsActualAuthority(t *testing.T) {
	release := fmt.Sprintf("release-%x-v1.yaml", sha256.Sum256([]byte("orders")))
	store := seedRegistryFiles(t, map[string]string{
		"catalog.yaml":        "id: kr://format/catalog\nrepositories: [table-meta]\n",
		"dataset-orders.yaml": storedDataset,
		release:               storedDataset,
	})
	before, _ := store.Head(snapshot.DefaultRef)
	registry, err := dataset.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := dataset.NewService(snapshot.NewRegistry(), registry)
	if err != nil {
		t.Fatal(err)
	}
	state := dataset.NormalizeState(service.DumpState())
	if state.AuthorityID != string(store.ID()) || registry.AuthorityID() != string(store.ID()) {
		t.Fatal("legacy logical label replaced actual Snapshot authority")
	}
	version, err := service.DatasetVersion("orders", 1)
	if err != nil || version.Sources[0].Commit != "T1" {
		t.Fatalf("recovery lost accepted source coordinates: %#v %v", version, err)
	}
	history, err := service.Log(dataset.LogQuery{Dataset: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Commits) != 1 || history.Commits[0].Commit != string(before) {
		t.Fatalf("legacy audit history lost: %#v", history)
	}
	if after, _ := store.Head(snapshot.DefaultRef); after != before {
		t.Fatal("opening legacy registry wrote authority")
	}
	if err := service.RegisterRepository("new-member"); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Head(snapshot.DefaultRef)
	paths, _ := store.ListFiles(after)
	if slices.Contains(paths, "catalog.yaml") || !slices.Contains(paths, dataset.RegistryFile()) {
		t.Fatalf("explicit mutation did not migrate metadata: %v", paths)
	}
	recovered, err := dataset.OpenSnapshotRegistry(store, "")
	if err != nil {
		t.Fatal(err)
	}
	service, err = dataset.NewService(snapshot.NewRegistry(), recovered)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.DatasetVersion("orders", 1)
	if err != nil || !reflect.DeepEqual(got, version) {
		t.Fatalf("metadata migration changed accepted release: %#v %v", got, err)
	}
	history, err = service.Log(dataset.LogQuery{Dataset: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Commits) != 1 || history.Commits[0].Commit != string(before) {
		t.Fatalf("metadata migration invented a Dataset change: %#v", history)
	}
	if _, err := store.ReadFile("catalog.yaml", before); err != nil {
		t.Fatal("migration discarded original metadata history", err)
	}
}

func TestRegistryCannotReinitializeMissingOrRetiredMetadata(t *testing.T) {
	for _, files := range []map[string]string{
		{"dataset-orders.yaml": storedDataset},
		{"catalog.yaml": "id: old\nrepositories: []\narchived: true\n"},
		{"catalog.yaml": "id: old\nrepositories: []\n", "registry.yaml": "authorityId: kr://format/catalog\nrepositories: []\n"},
	} {
		store := seedRegistryFiles(t, files)
		before, _ := store.Head(snapshot.DefaultRef)
		for _, open := range []func(snapshot.Store, string) (dataset.Registry, error){dataset.OpenSnapshotRegistry, dataset.CreateSnapshotRegistry} {
			if _, err := open(store, ""); kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
				t.Fatalf("invalid existing authority was treated as new: files=%v error=%v", files, err)
			}
			if after, _ := store.Head(snapshot.DefaultRef); after != before {
				t.Fatal("rejected metadata advanced authority")
			}
		}
	}
}

func TestLegacyRegistryIdentityMismatchDoesNotOpenOrRecreateAuthority(t *testing.T) {
	store := seedRegistryFiles(t, map[string]string{
		"catalog.yaml":        "id: kr://foreign/registry\nrepositories: [table-meta]\n",
		"dataset-orders.yaml": storedDataset,
	})
	before, _ := store.Head(snapshot.DefaultRef)
	for _, open := range []func(snapshot.Store, string) (dataset.Registry, error){dataset.OpenSnapshotRegistry, dataset.CreateSnapshotRegistry} {
		if _, err := open(store, ""); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("wrong legacy authority identity was adopted: %v", err)
		}
		if after, _ := store.Head(snapshot.DefaultRef); after != before {
			t.Fatal("rejected legacy identity advanced Snapshot authority")
		}
	}
}

func TestDatasetServiceHasNoRegistryArchiveLifecycle(t *testing.T) {
	contract := reflect.TypeOf((*dataset.Service)(nil)).Elem()
	for _, name := range []string{"Archive", "Archived"} {
		if _, exists := contract.MethodByName(name); exists {
			t.Fatalf("retired registry lifecycle remains public: %s", name)
		}
	}
}
