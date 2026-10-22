package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func systemSeedPolicies(dir, _ string) error {
	return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
}

func TestDeploymentRequiresActualSystemBinding(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Repositories = nil
	for _, err := range []error{cfg.Validate(), InitializeDeployment(cfg, systemSeedPolicies)} {
		if kernel.CodeOf(err) != kernel.ErrUsageInvalid || !strings.Contains(err.Error(), string(objectmodel.SystemRepositoryID)) {
			t.Fatalf("absent system binding was accepted: %v", err)
		}
	}
	if _, err := os.Stat(cfg.StateDir); !os.IsNotExist(err) {
		t.Fatalf("invalid deployment created durable state: %v", err)
	}
}

func TestDeploymentPublishesAndRestoresActualSystem(t *testing.T) {
	cfg := deploymentFixture(t)
	binding, _ := cfg.Binding(objectmodel.SystemRepositoryID)
	physical, err := openExistingAuthority(repositoryHomeRepo(binding))
	if err != nil {
		t.Fatal(err)
	}
	defer closeManagedSource(physical)
	empty, _ := physical.Head(snapshot.DefaultRef)
	if err := InitializeDeployment(cfg, systemSeedPolicies); err != nil {
		t.Fatal(err)
	}
	published, _ := physical.Head(snapshot.DefaultRef)
	if published == empty {
		t.Fatal("deployment did not publish to its actual system authority")
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := ws.Store.Get(objectmodel.SystemRepositoryID)
	head, _ := source.Head(snapshot.DefaultRef)
	if head != published || !source.HasCommit(published) {
		t.Fatal("runtime substituted a different system authority")
	}
	assertRuntimeSystemReadOnly(t, ws, source, head)
	repo, err := ws.Reader.Require(objectmodel.SystemRepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	read, err := repo.Read("kc/concept/object", published)
	if err != nil || read.Commit != published {
		t.Fatalf("actual system object was not readable: %#v %v", read, err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeployment(cfg, func(string, string) error { t.Fatal("bootstrap ran twice"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	source, _ = restarted.Store.Get(objectmodel.SystemRepositoryID)
	if head, _ := source.Head(snapshot.DefaultRef); head != published {
		t.Fatal("init or startup rewrote the system publication")
	}
}

func assertRuntimeSystemReadOnly(t *testing.T, ws *Home, source snapshot.Store, head kernel.CommitID) {
	t.Helper()
	if _, ok := snapshot.TreeStoreOf(source); ok {
		t.Fatal("runtime system exposed raw file writes")
	}
	if _, ok := snapshot.BulkTreeIngesterOf(source); ok {
		t.Fatal("runtime system exposed bulk file writes")
	}
	if err := source.CreateRef("refs/heads/other", head); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("CreateRef: %v", err)
	}
	if _, err := source.Merge(snapshot.DefaultRef, head, head); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("Merge: %v", err)
	}
	if err := source.Archive(); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("Archive: %v", err)
	}
	_, err := ws.TreeWriter.Commit("mutate-system-file", snapshot.TreeChangeSet{
		TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
		Changes: []snapshot.TreeChange{{Path: "README.md", Content: []byte("replacement")}},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("TreeWriter mutated runtime system: %v", err)
	}
	if after, _ := source.Head(snapshot.DefaultRef); after != head || source.Archived() {
		t.Fatal("rejected runtime writes changed the system authority")
	}
}

func TestDeploymentStartupAndReinitDoNotRepublishMissingSystem(t *testing.T) {
	cfg := deploymentFixture(t)
	if err := InitializeDeployment(cfg, systemSeedPolicies); err != nil {
		t.Fatal(err)
	}
	fake := testkit.NewLakeFSFake(t)
	cfg.Repositories[0].DSN = fake.DSN(fake.NewRepo())
	physical, err := openExistingAuthority(repositoryHomeRepo(cfg.Repositories[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer closeManagedSource(physical)
	before, _ := physical.Head(snapshot.DefaultRef)
	if ws, err := OpenDeployment(cfg); err == nil {
		ws.Close()
		t.Fatal("startup accepted an unpublished system authority")
	}
	if err := InitializeDeployment(cfg, systemSeedPolicies); err == nil {
		t.Fatal("reinit republished a lost system authority")
	}
	if after, _ := physical.Head(snapshot.DefaultRef); after != before {
		t.Fatal("restore check wrote to the empty system authority")
	}
	files, err := physical.(snapshot.TreeReader).ListFiles(before)
	if err != nil || len(files) != 0 {
		t.Fatalf("restore check seeded files: %v %v", files, err)
	}
	fake.Close()
	if ws, err := OpenDeployment(cfg); err == nil {
		ws.Close()
		t.Fatal("unavailable system authority was replaced with an embedded repository")
	}
}

func TestComponentOpenRequiresPublishedSystem(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := InitHome(dir, func() RegistryBinding { b := systemComponentBinding(t); b.ID = "kr://test/registry"; return b }()); err != nil {
		t.Fatal(err)
	}
	if ws, err := Open(dir); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		if ws != nil {
			ws.Close()
		}
		t.Fatalf("component open mounted a default system repository: %v", err)
	}
}

func TestSystemInitializationAndRestoreUseSeparateAuthorityPorts(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Repositories[0] = RepositoryBinding{ID: string(objectmodel.SystemRepositoryID), Driver: "lakefs", DSN: "https://lakefs.example/system"}
	physical := testkit.MakeTreeStore(t, string(objectmodel.SystemRepositoryID))
	original := authorityDrivers["lakefs"]
	initializes, restores := 0, 0
	driver := original
	driver.validate = func(HomeRepo) error { return nil }
	driver.open = func(abs string, item HomeRepo) (snapshot.Store, error) {
		if item.ID != string(objectmodel.SystemRepositoryID) {
			return original.open(abs, item)
		}
		initializes++
		return physical, nil
	}
	driver.openExisting = func(abs string, item HomeRepo) (snapshot.Store, error) {
		if item.ID != string(objectmodel.SystemRepositoryID) {
			return original.openExisting(abs, item)
		}
		restores++
		return physical, nil
	}
	authorityDrivers["lakefs"] = driver
	defer func() { authorityDrivers["lakefs"] = original }()
	if err := InitializeDeployment(cfg, systemSeedPolicies); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ws.Close()
	if err := InitializeDeployment(cfg, systemSeedPolicies); err != nil {
		t.Fatal(err)
	}
	if initializes != 1 || restores != 2 {
		t.Fatalf("startup or reinit opened an initialization port: init=%d restore=%d", initializes, restores)
	}
}

func systemComponentBinding(t *testing.T) RegistryBinding {
	t.Helper()
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	return RegistryBinding{Driver: "lakefs", DSN: fake.DSN(fake.NewRepo())}
}
