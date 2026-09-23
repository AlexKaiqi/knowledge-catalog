package home

import (
	"strings"
	"testing"

	"kc/kernel"
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

func TestNamedLakeFSBindingUsesOriginAndStoragePrefix(t *testing.T) {
	driver, err := authorityFor("lakefs")
	if err != nil {
		t.Fatal(err)
	}
	pool := ManagedRepositoryConfig{Driver: "lakefs", DSN: "http://lakefs.example", Root: "s3://kc-authority", PublicURL: "https://kc.example.test"}
	request := ManagedRepositoryRequest{RepositoryID: "kr://kaiqidong/notes", Name: "physical", Principal: "kaiqidong"}
	binding, err := driver.managedBinding(pool, request, "allocation")
	if err != nil || binding.Driver != "lakefs" || binding.DSN != "http://lakefs.example/physical" || binding.Dir != "" {
		t.Fatalf("LakeFS binding: %#v err=%v", binding, err)
	}
	want := pool.PublicURL + "/repositories/physical"
	if got := driver.managedURL(pool, binding); got != want {
		t.Fatalf("LakeFS management URL: %s want %s", got, want)
	}
}

func TestNamedLakeFSRepositoryIDMatchesGravelerName(t *testing.T) {
	id, err := namedManagedRepositoryID(ManagedRepositoryRequest{Name: "table-meta", Principal: "admin"}, ManagedRepositoryConfig{Driver: "lakefs"}, "4efb667886bab5ab6c8c6af59245edac")
	if err != nil || id != "table-meta" {
		t.Fatalf("LakeFS protocol id must be the Graveler name: %q %v", id, err)
	}
	id, err = namedManagedRepositoryID(ManagedRepositoryRequest{Name: "团队规范", Principal: "alice"}, ManagedRepositoryConfig{Driver: "gitea"}, "deadbeef")
	if err != nil || id != "kr://alice/repo-deadbeef" {
		t.Fatalf("Gitea keeps a logical coordinate: %q %v", id, err)
	}
	_, err = namedManagedRepositoryID(ManagedRepositoryRequest{Name: "物理层", Principal: "admin"}, ManagedRepositoryConfig{Driver: "lakefs"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("non-slug LakeFS name: %v", err)
	}
}

func TestNamedLakeFSBindingRejectsNonGravelerName(t *testing.T) {
	driver, err := authorityFor("lakefs")
	if err != nil {
		t.Fatal(err)
	}
	pool := ManagedRepositoryConfig{Driver: "lakefs", DSN: "http://lakefs.example", Root: "s3://kc-authority", PublicURL: "https://kc.example.test"}
	_, err = driver.managedBinding(pool, ManagedRepositoryRequest{RepositoryID: "kr://kaiqidong/notes", Name: "物理层", Principal: "kaiqidong"}, "allocation")
	if err == nil || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("non-slug LakeFS name: %v", err)
	}
}

func TestNamedLakeFSBindingUsesGravelerNameAndExplicitManagementURL(t *testing.T) {
	driver, err := authorityFor("lakefs")
	if err != nil {
		t.Fatal(err)
	}
	pool := ManagedRepositoryConfig{Driver: "lakefs", DSN: "http://lakefs.example.test", Root: "s3://kc-authority", PublicURL: "https://kc.example.test"}
	request := ManagedRepositoryRequest{RepositoryID: "kr://kaiqidong/notes", Name: "team-spec", Principal: "kaiqidong"}
	binding, err := driver.managedBinding(pool, request, "allocation")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(binding.DSN, pool.DSN+"/") {
		t.Fatalf("LakeFS tenant did not stay under the pool origin: %s", binding.DSN)
	}
	if !strings.Contains(binding.DSN, "team-spec") {
		t.Fatal("Graveler name disappeared from allocated tenant")
	}
	managementURL := driver.managedURL(pool, binding)
	if !strings.HasPrefix(managementURL, pool.PublicURL+"/repositories/") {
		t.Fatalf("LakeFS management URL ignored the explicit PublicURL: %s", managementURL)
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
