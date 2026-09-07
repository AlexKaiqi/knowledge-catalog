package home

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func deploymentFixture(t *testing.T) DeploymentConfig {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "authority.git")
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git: %s: %v", out, err)
	}
	return DeploymentConfig{Version: 1, StateDir: filepath.Join(root, "state"), CacheDir: filepath.Join(root, "cache"), Catalogs: []CatalogBinding{{ID: "kr://test/catalog", Remote: remote}}, Auth: "local", BootstrapPrincipal: "owner"}
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
	cfg.Catalogs[0].Remote = filepath.Join(cfg.CacheDir, "authority.git")
	if err := cfg.Validate(); err == nil {
		t.Fatal("authority inside cache accepted")
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
				cfg.Catalogs[0].Remote = "file://" + filepath.Join(cfg.CacheDir, "authority.git")
			} else {
				alias := filepath.Join(filepath.Dir(cfg.CacheDir), "alias")
				if err := os.Symlink(cfg.CacheDir, alias); err != nil {
					t.Fatal(err)
				}
				cfg.Catalogs[0].Remote = filepath.Join(alias, "authority.git")
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
	if out, err := exec.Command("git", "--git-dir", cfg.Catalogs[0].Remote, "update-ref", "-d", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fault injection: %s %v", out, err)
	}
	if err := InitializeDeployment(cfg, seed); err == nil {
		t.Fatal("init recreated lost Catalog with empty membership")
	}
	if out, err := exec.Command("git", "--git-dir", cfg.Catalogs[0].Remote, "show-ref", "--verify", "refs/heads/main").CombinedOutput(); err == nil {
		t.Fatalf("missing authority ref was recreated: %s", out)
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
