package home

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitManagedHumanUsesUsername(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	result, err := ws.CreateManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, RepositoryID: "kr://managed/explicit", CommandID: "explicit-human", Principal: "kaiqidong"}, func(ManagedRepositoryGrant) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ManagementURL, "/kaiqidong/") {
		t.Fatalf("MANAGED-USER-NAME: explicit user create still uses platform account: %s", result.ManagementURL)
	}
}

func TestNamedDoltBindingUsesUsernameAndExplicitManagementURL(t *testing.T) {
	driver, err := authorityFor("dolt")
	if err != nil {
		t.Fatal(err)
	}
	pool := ManagedRepositoryConfig{Driver: "dolt", Root: t.TempDir(), PublicURL: "https://kc.example.test"}
	request := ManagedRepositoryRequest{RepositoryID: "kr://kaiqidong/notes", Name: "用户规范", Principal: "kaiqidong"}
	binding := driver.managedBinding(pool, request, "allocation")
	if binding.Dir != filepath.Join(pool.Root, "kaiqidong", "kc-allocation") {
		t.Fatalf("Dolt tenant did not retain username: %s", binding.Dir)
	}
	want := pool.PublicURL + "/repositories/" + url.PathEscape(request.RepositoryID)
	if got := driver.managedURL(pool, binding); got != want {
		t.Fatalf("Dolt management URL: %s want %s", got, want)
	}
	if !strings.Contains(binding.Dir, "kaiqidong") {
		t.Fatal("username disappeared from allocated tenant")
	}
}

func TestNamedManagedRepositoryDistinctStoresAndUsersStaySeparate(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	ws.Deployment.ManagedStores = map[string]ManagedRepositoryConfig{"one": *cfg.ManagedRepositories, "two": *cfg.ManagedRepositories}
	ws.Deployment.ManagedRepositories = nil
	ids := map[string]bool{}
	for _, principal := range []string{"alice", "bob"} {
		for _, store := range []string{"one", "two"} {
			result, err := ws.CreateNamedManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Name: "团队规范", Principal: principal, Store: store}, func(ManagedRepositoryGrant) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if ids[result.RepositoryID] || result.Owner != principal || result.Store != store || !strings.Contains(result.ManagementURL, "/"+principal+"/") {
				t.Fatalf("store or owner collision: %#v", result)
			}
			ids[result.RepositoryID] = true
		}
	}
	if *creates != 4 {
		t.Fatalf("physical allocation count = %d", *creates)
	}
	rows, err := ws.ListManagedRepositories("alice")
	if err != nil || len(rows) != 2 {
		t.Fatalf("own inventory: %#v %v", rows, err)
	}
}

func TestNamedManagedRepositoryHasDurableOwnerAndManagementURL(t *testing.T) {
	cfg, ws, creates := managedFixture(t)
	req := ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Name: "团队规范", Principal: "alice"}
	grants := 0
	grant := func(ManagedRepositoryGrant) error { grants++; return nil }
	result, err := ws.CreateNamedManagedRepository(req, grant)
	if err != nil {
		t.Fatalf("MANAGED-USER-NAME: %v", err)
	}
	if result.Owner != "alice" || result.Name != req.Name || result.Store != "gitea" || result.ManagementURL == "" || result.ProvisioningState != "READY" {
		t.Fatalf("MANAGED-DURABLE-MANAGEMENT-URL: %#v", result)
	}
	if !strings.Contains(result.RepositoryID, "kr://alice/") {
		t.Fatalf("logical owner missing: %s", result.RepositoryID)
	}
	if _, err := ws.CreateNamedManagedRepository(req, grant); err != nil {
		t.Fatal(err)
	}
	if grants != 1 || *creates != 1 {
		t.Fatalf("replay repeated effects grants=%d creates=%d", grants, *creates)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.ManagedRepositories = nil
	ws, err = OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	rows, err := ws.ListManagedRepositories("alice")
	if err != nil || len(rows) != 1 || rows[0].ManagementURL != result.ManagementURL {
		t.Fatalf("management URL lost after restart: %#v %v", rows, err)
	}
	if _, err := ws.GetOwnedManagedRepository("bob", result.RepositoryID); err == nil {
		t.Fatal("another user read private control-plane details")
	}
}

func TestNamedManagedRepositoryRequiresUnambiguousStore(t *testing.T) {
	cfg, ws, _ := managedFixture(t)
	ws.Deployment.ManagedStores = map[string]ManagedRepositoryConfig{"one": *cfg.ManagedRepositories, "two": *cfg.ManagedRepositories}
	ws.Deployment.ManagedRepositories = nil
	_, err := ws.CreateNamedManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Name: "notes", Principal: "alice"}, func(ManagedRepositoryGrant) error { t.Fatal("ambiguous pool granted access"); return nil })
	if err == nil || !strings.Contains(err.Error(), "store") {
		t.Fatalf("MANAGED-STORE-SELECTION: %v", err)
	}
}
