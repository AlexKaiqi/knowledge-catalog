package home

import (
	"encoding/json"
	"errors"
	"kc/internal/testkit"
	"kc/kernel"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestManagedRepositoryConfigurationRequiresExplicitCreatorPolicy(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.ManagedRepositories = &ManagedRepositoryConfig{Driver: "lakefs", DSN: "http://lakefs.example.test", Root: "s3://kc-authority"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("managed provisioning accepted an absent creator policy")
	}
	cfg.ManagedRepositories.CreatorActions = []string{"identity.grant"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("managed provisioning accepted privilege escalation policy")
	}
	cfg.ManagedRepositories.CreatorActions = []string{"writer.commit", "object.read"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func managedFixture(t *testing.T) (DeploymentConfig, *Home, *int) {
	return managedFixtureWithInterceptor(t, nil)
}

func managedFixtureWithInterceptor(t *testing.T, intercept func(http.ResponseWriter, *http.Request) bool) (DeploymentConfig, *Home, *int) {
	t.Helper()
	creates := new(int)
	var mu sync.Mutex
	fake := testkit.NewLakeFSFakeWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		if intercept != nil && intercept(w, r) {
			return true
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/repositories" {
			mu.Lock()
			*creates++
			mu.Unlock()
		}
		return false
	})
	cfg := deploymentFixture(t)
	cfg.ManagedRepositories = &ManagedRepositoryConfig{Driver: "lakefs", DSN: fake.Origin(), Root: "s3://kc-authority", PublicURL: "https://kc.example.test", CreatorActions: []string{"writer.commit", "object.read"}}
	if err := InitializeDeployment(cfg, func(dir, principal string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return cfg, ws, creates
}

func TestManagedRepositoryPersistsAllocationAndDoesNotRepeatGrant(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/source", CommandID: "create-one", Principal: "alice"}
	grants := 0
	grant := func(g ManagedRepositoryGrant) error {
		grants++
		if err := ValidateDeploymentState(cfg); err != nil {
			t.Fatalf("grant callback could not independently validate durable state: %v", err)
		}
		if g.AllocationID == "" || g.Principal != req.Principal || g.RepositoryID != req.RepositoryID || len(g.Actions) != 2 {
			t.Fatalf("wrong creator grant: %#v", g)
		}
		return nil
	}
	result, err := ws.CreateManagedRepository(req, grant)
	if err != nil || result.Status != "APPLIED" || result.Head != "c0" {
		t.Fatalf("create: %#v %v", result, err)
	}
	if ws.Datasets.HasRepository(kernel.RepositoryID(req.RepositoryID)) {
		t.Fatal("create must not register the repository in Catalog")
	}
	if err := ws.AttachRepository(kernel.RepositoryID(req.RepositoryID)); err != nil {
		t.Fatal(err)
	}
	if !ws.Datasets.HasRepository(kernel.RepositoryID(req.RepositoryID)) {
		t.Fatal("attach must register the repository in Catalog")
	}
	if _, ok := ws.Store.Get(kernel.RepositoryID(req.RepositoryID)); !ok {
		t.Fatal("created repository missing from runtime inventory")
	}
	if _, err := ws.CreateManagedRepository(req, grant); err != nil {
		t.Fatal(err)
	}
	if grants != 1 || *creates != 1 {
		t.Fatalf("replay repeated effects: grants=%d creates=%d", grants, *creates)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	// Turning off new provisioning must not erase historical bindings.
	cfg.ManagedRepositories = nil
	reopened, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	result, err = reopened.CreateManagedRepository(req, grant)
	if err != nil || result.Status != "REPLAYED" || result.Head != "c0" {
		t.Fatalf("durable replay: %#v %v", result, err)
	}
	if grants != 1 || *creates != 1 {
		t.Fatal("recovery repeated provisioning or creator policy")
	}
}

func TestManagedRepositoryResumesGrantFailureWithoutCreatingAnotherSource(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{RepositoryID: "kr://managed/retry", CommandID: "retry", Principal: "alice"}
	allocation := ""
	if _, err := ws.CreateManagedRepository(req, func(g ManagedRepositoryGrant) error { allocation = g.AllocationID; return errors.New("grant failed") }); err == nil {
		t.Fatal("grant failure returned success")
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	result, err := reopened.CreateManagedRepository(req, func(g ManagedRepositoryGrant) error {
		if g.AllocationID != allocation {
			t.Fatal("retry changed allocation")
		}
		return nil
	})
	if err != nil || result.Status != "APPLIED" || *creates != 1 {
		t.Fatalf("retry: %#v creates=%d %v", result, *creates, err)
	}
	req.RepositoryID = "kr://managed/different"
	if _, err := reopened.CreateManagedRepository(req, func(ManagedRepositoryGrant) error { t.Fatal("digest conflict granted permissions"); return nil }); err == nil {
		t.Fatal("same command accepted different repository")
	}
}

func TestManagedRepositoryLegacyDeploymentRequiresExplicitLedgerInitialization(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := readDeploymentState(cfg)
	if err != nil {
		t.Fatal(err)
	}
	state.ManagedStoreInitialized = false
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, deploymentMarker), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cfg.StateDir, managedLedgerFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDeployment(cfg); err == nil {
		t.Fatal("new provisioning silently initialized an old deployment ledger")
	}
	if _, err := os.Stat(filepath.Join(cfg.StateDir, managedLedgerFile)); !os.IsNotExist(err) {
		t.Fatal("open created the managed ledger")
	}
	if err := InitializeDeployment(cfg, func(string, string) error { t.Fatal("upgrade repeated bootstrap policy"); return nil }); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if *creates != 0 {
		t.Fatal("enabling managed pool created a source")
	}
	state, err = readDeploymentState(cfg)
	if err != nil || !state.ManagedStoreInitialized {
		t.Fatalf("upgrade did not persist ledger receipt: %#v %v", state, err)
	}
}
