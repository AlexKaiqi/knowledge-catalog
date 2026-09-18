package home

import (
	"os"
	"path/filepath"
	"testing"
)

func deploymentFixture(t *testing.T) DeploymentConfig {
	t.Helper()
	root := t.TempDir()
	catalogDir := filepath.Join(root, "catalog-authority")
	t.Cleanup(func() { _ = os.RemoveAll(catalogDir) })
	return DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "state"), CacheDir: filepath.Join(root, "cache"), Catalogs: []CatalogBinding{{ID: "kr://test/catalog", Driver: "dolt", Dir: catalogDir}}, Auth: "local", BootstrapPrincipal: "owner"}
}
func TestDeploymentAcceptsLakeFSGravelerRepositoryID(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Repositories = []RepositoryBinding{{ID: "table-meta", Driver: "lakefs", DSN: "http://lakefs.example/table-meta"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Repositories[0].ID = "kc-system"
	if err := cfg.Validate(); err == nil {
		t.Fatal("platform Graveler name accepted as business Snapshot binding")
	}
}

func TestDeploymentRejectsWriteInRepositoryAccess(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.RepositoryAccess = []RepositoryAccess{{ID: "kr://acme/public/core", AuthenticatedActions: []string{"knowledge.read", "writer.commit"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("authenticated default must not include write")
	}
}

func TestDeploymentRejectsEvidenceRetentionAboveHardCap(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Evidence = &EvidenceConfig{HotRetention: "200d"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("hotRetention above 180d accepted")
	}
}

func TestDeploymentDoesNotInitializeOnOpen(t *testing.T) {
	cfg := deploymentFixture(t)
	if _, err := OpenDeployment(cfg); err == nil {
		t.Fatal("uninitialized deployment opened")
	}
	if _, err := os.Stat(cfg.StateDir); !os.IsNotExist(err) {
		t.Fatal("opening deployment created durable state")
	}
}
func TestDeploymentRejectsInstanceBoundAuthorities(t *testing.T) {
	cfg := deploymentFixture(t)
	cfg.Catalogs[0].Dir = filepath.Join(cfg.CacheDir, "catalog-authority")
	if err := cfg.Validate(); err == nil {
		t.Fatal("authority inside cache accepted")
	}
	cfg = deploymentFixture(t)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Catalogs[0].Driver = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Catalog without Snapshot driver accepted")
	}
	cfg = deploymentFixture(t)
	cfg.StateDir = filepath.Join(cfg.CacheDir, "state")
	if err := cfg.Validate(); err == nil {
		t.Fatal("durable state inside cache accepted")
	}
}

func TestDeploymentRecoversCatalogAfterCacheLoss(t *testing.T) {
	cfg := deploymentFixture(t)
	seed := func(dir, principal string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	before, err := ws.Registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	ws, err = OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	after, err := ws.Registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("recovery changed Catalog head: %s -> %s", before, after)
	}
	if err := os.Remove(filepath.Join(cfg.StateDir, "gates.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDeployment(cfg); err == nil {
		t.Fatal("missing durable gate policy recreated")
	}
	if err := InitializeDeployment(cfg, seed); err == nil {
		t.Fatal("init reset missing gate policy")
	}
}
func TestDeploymentRejectsSymlinkAndFileURLAuthoritiesInCache(t *testing.T) {
	for _, kind := range []string{"url", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			cfg := deploymentFixture(t)
			if err := os.MkdirAll(cfg.CacheDir, 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "url" {
				cfg.Catalogs[0].Driver = "lakefs"
				cfg.Catalogs[0].Dir = ""
				cfg.Catalogs[0].DSN = "file://" + filepath.Join(cfg.CacheDir, "catalog")
			} else {
				alias := filepath.Join(filepath.Dir(cfg.CacheDir), "alias")
				if err := os.Symlink(cfg.CacheDir, alias); err != nil {
					t.Fatal(err)
				}
				cfg.Catalogs[0].Dir = filepath.Join(alias, "catalog-authority")
			}
			if err := cfg.Validate(); err == nil {
				t.Fatal("instance-bound authority accepted")
			}
		})
	}
}

func TestDeploymentRecoveryDoesNotRewriteControlLedger(t *testing.T) {
	cfg := deploymentFixture(t)
	seed := func(dir, principal string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(cfg.StateDir, "writer.db"))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	after, err := os.ReadFile(filepath.Join(cfg.StateDir, "writer.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("recovery rewrote the durable Writer ledger")
	}
}

func TestDeploymentCannotResetLostDurableVolume(t *testing.T) {
	cfg := deploymentFixture(t)
	calls := 0
	seed := func(dir, principal string) error {
		calls++
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.StateDir); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeployment(cfg, seed); err == nil {
		t.Fatal("existing Catalog received empty governance after volume loss")
	}
	if calls != 1 {
		t.Fatal("bootstrap principal was reissued")
	}
	if _, err := os.Stat(cfg.StateDir); !os.IsNotExist(err) {
		t.Fatal("lost durable data was recreated")
	}
}

func TestDeploymentMalformedPolicyFailsClosed(t *testing.T) {
	cfg := deploymentFixture(t)
	seed := func(dir, principal string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StateDir, "gates.json"), []byte(`{"rules":"corrupt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeploymentState(cfg); err == nil {
		t.Fatal("malformed gate contract interpreted as empty gates")
	}
}

func TestDeploymentDoesNotRecreateLostCatalogBranch(t *testing.T) {
	cfg := deploymentFixture(t)
	seed := func(dir, principal string) error {
		return os.WriteFile(filepath.Join(dir, "allow.json"), []byte(`{"version":2,"rules":[]}`), 0600)
	}
	if err := InitializeDeployment(cfg, seed); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfg.Catalogs[0].Dir); err != nil {
		t.Fatal(err)
	}
	if err := InitializeDeployment(cfg, seed); err == nil {
		t.Fatal("init recreated lost Catalog with empty membership")
	}
	if _, err := os.Stat(filepath.Join(cfg.Catalogs[0].Dir, ".dolt")); err == nil {
		t.Fatal("missing Catalog Snapshot authority was recreated")
	}
}

func TestDeploymentRejectsIgnoredLayoutAndDriverDefaults(t *testing.T) {
	for _, change := range []func(*DeploymentConfig){
		func(c *DeploymentConfig) { c.Stores.Layout.Catalogs = "alternate" },
		func(c *DeploymentConfig) { c.Stores.Repository = "gitea" },
		func(c *DeploymentConfig) { c.Stores.Profile = "local" },
	} {
		cfg := deploymentFixture(t)
		change(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Error("accepted fixture-only setting that deployment would ignore")
		}
	}
}

func TestDeploymentRejectsLegacyCatalogGitRemoteField(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "deployment.yaml")
	content := "version: 1\n" +
		"stateDir: " + filepath.Join(root, "state") + "\n" +
		"cacheDir: " + filepath.Join(root, "cache") + "\n" +
		"auth: local\n" +
		"bootstrapPrincipal: owner\n" +
		"catalogs:\n" +
		"  - id: kr://test/catalog\n" +
		"    remote: file://" + filepath.Join(root, "catalog.git") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDeployment(path); err == nil {
		t.Fatal("legacy Git remote Catalog field accepted")
	}
}
