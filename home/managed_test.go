package home

import (
	"encoding/json"
	"errors"
	"kc/kernel"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestManagedRepositoryConfigurationRequiresExplicitCreatorPolicy(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.ManagedRepositories = &ManagedRepositoryConfig{Driver: "dolt", Root: t.TempDir()}
	if err := cfg.Validate(); err == nil {
		t.Fatal("managed provisioning accepted an absent creator policy")
	}
	cfg.ManagedRepositories.CreatorActions = []string{"identity.grant"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("managed provisioning accepted privilege escalation policy")
	}
	cfg.ManagedRepositories.CreatorActions = []string{"writer.commit", "knowledge.read"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func managedFixture(t *testing.T) (DeploymentConfig, *Home, *int) {
	t.Helper()
	t.Setenv("KC_GITEA_TOKEN", "test-token")
	var mu sync.Mutex
	objects := map[string]map[string]any{}
	creates := new(int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/user" {
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "kc"})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/repos" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			name := body["name"].(string)
			if objects[name] != nil {
				w.WriteHeader(409)
				return
			}
			*creates++
			objects[name] = map[string]any{"id": *creates, "description": body["description"], "default_branch": "main", "empty": false}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(objects[name])
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/repos/kc/"), "/")
		if len(parts) == 0 || objects[parts[0]] == nil {
			w.WriteHeader(404)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected managed source mutation %s %s", r.Method, r.URL.Path)
			w.WriteHeader(405)
			return
		}
		if len(parts) == 1 {
			_ = json.NewEncoder(w).Encode(objects[parts[0]])
			return
		}
		if parts[1] == "branches" && len(parts) == 3 && parts[2] == "main" {
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "initial-head"}})
			return
		}
		if parts[1] == "commits" || (parts[1] == "git" && len(parts) > 2 && parts[2] == "commits") {
			_ = json.NewEncoder(w).Encode(map[string]any{"sha": "initial-head"})
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(server.Close)
	cfg := deploymentFixture(t)
	cfg.ManagedRepositories = &ManagedRepositoryConfig{Driver: "gitea", DSN: server.URL + "/kc", CreatorActions: []string{"writer.commit", "knowledge.read"}}
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
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/source", CommandID: "create-one", Principal: "alice"}
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
	if err != nil || result.Status != "APPLIED" || result.Head != "initial-head" {
		t.Fatalf("create: %#v %v", result, err)
	}
	if !ws.Catalog.HasRepository(kernel.RepositoryID(req.RepositoryID)) {
		t.Fatal("created repository was not admitted")
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
	if err != nil || result.Status != "REPLAYED" || result.Head != "initial-head" {
		t.Fatalf("durable replay: %#v %v", result, err)
	}
	if grants != 1 || *creates != 1 {
		t.Fatal("recovery repeated provisioning or creator policy")
	}
}

func TestManagedRepositoryResumesGrantFailureWithoutCreatingAnotherSource(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/retry", CommandID: "retry", Principal: "alice"}
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
