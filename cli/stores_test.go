package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
)

func TestStoreConfigRejectsSecrets(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	listed := asMap(t, body(t, kc(h, "store-ls")))
	if listed["repository"] != "filegit" || listed["index"] != "none" || listed["profile"] != "local" {
		t.Fatalf("%#v", listed)
	}
	if listed["postgres"] != nil {
		t.Fatalf("local init should not invent postgres: %#v", listed["postgres"])
	}
	layout := asMap(t, listed["layout"])
	if layout["repos"] != "repos" || layout["projections"] != "projections" || layout["checkouts"] != "checkouts" {
		t.Fatalf("layout %#v", layout)
	}
	if strings.Contains(kc(h, "store-ls").Stdout, "sandbox:sandbox") {
		t.Fatal("store-ls leaked a password")
	}
	expectMsg(t, kc(h, "store-set", "--driver", "postgres", "--dsn", "postgres://sandbox:secret@127.0.0.1:5433/sandbox"), "postgres")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/semantic", "--driver", "postgres"), "postgres")
	raw, err := os.ReadFile(filepath.Join(h, "stores.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "password") {
		t.Fatalf("stores.yaml contained a secret: %s", raw)
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		t.Fatalf("stores.yaml should be YAML, got JSON: %s", raw)
	}
	if !strings.Contains(string(raw), "repository: filegit") || !strings.Contains(string(raw), "index: none") {
		t.Fatalf("stores.yaml missing local store: %s", raw)
	}
	if strings.Contains(string(raw), "filegit:") {
		t.Fatalf("stores.yaml should not hold local dirs: %s", raw)
	}
	expectMsg(t, kc(h, "store-set", "--driver", "redis", "--host", "127.0.0.1", "--port", "16379"), "unknown store driver redis")
	expectMsg(t, kc(h, "store-set", "--index", "redis"), "projection provider")
	expectMsg(t, kc(h, "store-set", "--repository", "redis"), "unknown repository driver redis")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/redis-not-repo", "--driver", "redis"), "unknown repository driver redis")
	expectMsg(t, kc(h, "store-set", "--repository", "stream"), "unknown repository driver stream")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/stream-not-repo", "--driver", "stream"), "unknown repository driver stream")
	expectMsg(t, kc(h, "store-set", "--driver", "mysql"), "unknown store driver mysql")
	expectMsg(t, kc(h, "store-set", "--driver", "elasticsearch", "--url", "http://127.0.0.1:9200"), "unknown store driver elasticsearch")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/mysql-not-repo", "--driver", "mysql"), "unknown repository driver mysql")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/gitea-no-dsn", "--driver", "gitea"), "requires --dsn")
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/gitea-secret", "--driver", "gitea", "--dsn", "http://u:p@127.0.0.1:3001/kc/core"), "must not contain secrets")
	body(t, kc(h, "store-set", "--repository", "gitea"))
	if asMap(t, body(t, kc(h, "store-ls")))["repository"] != "gitea" {
		t.Fatal("store-set --repository gitea")
	}

	body(t, kc(h, "store-set", "--profile", "scale"))
	scaleListed := asMap(t, body(t, kc(h, "store-ls")))
	if scaleListed["profile"] != "scale" || scaleListed["repository"] != "dolt" || scaleListed["index"] != "opensearch" || scaleListed["cache"] != nil {
		t.Fatalf("scale profile %#v", scaleListed)
	}
	expectMsg(t, kc(h, "store-set", "--driver", "opensearch", "--url", "http://user:secret@127.0.0.1:9200"), "must not contain secrets")
	body(t, kc(h, "store-set", "--index", "opensearch", "--driver", "opensearch", "--url", "http://127.0.0.1:9200"))
	again := asMap(t, body(t, kc(h, "store-ls")))
	if again["index"] != "opensearch" || again["cache"] != nil {
		t.Fatalf("%#v", again)
	}
	st := asMap(t, body(t, kc(h, "status")))
	if asMap(t, st["stores"])["index"] != "opensearch" {
		t.Fatalf("%#v", st["stores"])
	}
}

func TestLocalStoreConfig(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	engines, err := os.ReadFile(filepath.Join(h, "stores.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"repository: filegit", "index: none"} {
		if !strings.Contains(string(engines), needle) {
			t.Fatalf("missing %q in %s", needle, engines)
		}
	}
	for _, banned := range []string{"filegit:", "repos/_catalog"} {
		if strings.Contains(string(engines), banned) {
			t.Fatalf("stores.yaml should not hold layout: %s", engines)
		}
	}
	layout, err := os.ReadFile(filepath.Join(h, "layout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"repos: repos", "projections: projections", "catalogs: catalogs", "checkouts: checkouts"} {
		if !strings.Contains(string(layout), needle) {
			t.Fatalf("missing %q in %s", needle, layout)
		}
	}
	if strings.Contains(string(layout), "catalog:") && !strings.Contains(string(layout), "catalogs:") {
		t.Fatalf("layout still uses singular catalog path: %s", layout)
	}
	if strings.Contains(string(layout), "_catalog") {
		t.Fatalf("layout still uses repos/_catalog: %s", layout)
	}
	body(t, kc(h, "store-set", "--repository", "filegit", "--index", "none"))
	expectMsg(t, kc(h, "store-set", "--index", "memory"), "projection provider")
	expectMsg(t, kc(h, "store-set", "--index", "sqlite"), "projection provider")
	body(t, kc(h, "store-set", "--repos-dir", "repos", "--projections-dir", "projections"))
	body(t, kc(h, "store-set", "--driver", "filegit", "--dir", "repos"))
	added := asMap(t, body(t, kc(h, "repo-add", "--repo", "kr://acme/public/core")))
	if len(fmt.Sprint(added["head"])) < 40 {
		t.Fatal(added)
	}
	st := asMap(t, body(t, kc(h, "status")))
	repos := st["repos"].([]any)
	if len(repos) != 1 {
		t.Fatalf("%#v", st["repos"])
	}
	item := asMap(t, repos[0])
	if item["driver"] != "filegit" || !strings.Contains(fmt.Sprint(item["dir"]), "repos/") {
		t.Fatalf("%#v", item)
	}
}

func TestLocalProfileHasNoSearchProjection(t *testing.T) {
	t.Setenv("KC_TEST_OPENSEARCH_URL", "")
	h := testkit.TempDir(t)
	repo := "kr://acme/public/local-only"
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "repo-add", "--repo", repo))
	body(t, kc(h, "put", "--command-id", "local-1", "--repo", repo,
		"--object", "runbook/local", "--value", `{"body":"exact read stays available"}`))
	body(t, kc(h, "define-workspace", "--workspace", "local", "--revision", "1",
		"--source", repo+"=refs/heads/main@"))

	values := body(t, kc(h, "read", "--workspace", "local", "--object", "runbook/local")).([]any)
	if len(values) != 1 || asMap(t, asMap(t, values[0])["value"])["body"] != "exact read stays available" {
		t.Fatalf("local exact read failed: %#v", values)
	}
	body(t, kc(h, "vfs-list", "--workspace", "local"))

	failed := kc(h, "search", "--workspace", "local", "--query", "exact")
	expectCode(t, failed, "CAPABILITY_UNSATISFIED")
	expectMsg(t, failed, "OpenSearch")
}

func TestHomeLayoutDiscoversFromDisk(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	if _, err := os.Stat(filepath.Join(h, "workspace.json")); err == nil {
		t.Fatal("init must not write workspace.json")
	}
	if _, err := os.Stat(filepath.Join(h, "catalogs", "kr_acme_catalog", ".git")); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "repo-add", "--repo", "kr://acme/public/core"))
	if _, err := os.Stat(filepath.Join(h, "repos", "kr_acme_public_core", ".git")); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "catalog-add", "--catalog", "kr://acme/docs/catalog"))
	if _, err := os.Stat(filepath.Join(h, "catalogs", "kr_acme_docs_catalog", ".git")); err != nil {
		t.Fatal(err)
	}
	listed := asMap(t, body(t, kc(h, "store-ls")))
	layout := asMap(t, listed["layout"])
	if layout["catalogs"] != "catalogs" || layout["catalog"] != nil {
		t.Fatalf("layout %#v", layout)
	}
}

func TestStoreConfigMigratesLegacyJSON(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	if err := os.Remove(filepath.Join(h, "stores.yaml")); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"opensearch":{"url":"http://10.0.0.8:9200"}}`)
	if err := os.WriteFile(filepath.Join(h, "stores.json"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	listed := asMap(t, body(t, kc(h, "store-ls")))
	if asMap(t, listed["opensearch"])["url"] != "http://10.0.0.8:9200" {
		t.Fatalf("%#v", listed)
	}
	body(t, kc(h, "store-set", "--driver", "opensearch", "--url", "http://10.0.0.8:9200"))
	if _, err := os.Stat(filepath.Join(h, "stores.json")); !os.IsNotExist(err) {
		t.Fatal("legacy stores.json should be removed after store-set")
	}
	raw, err := os.ReadFile(filepath.Join(h, "stores.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "10.0.0.8") {
		t.Fatalf("%s", raw)
	}
}

func TestStoreConfigMigratesCombinedYAML(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	combined := []byte(`repository: filegit
index: none
layout:
  projections: data/projections
filegit:
  dir: data/repos
  catalog: data/repos/_catalog
  catalogs: data/repos/_catalogs
opensearch:
  url: http://10.0.0.8:9200
`)
	if err := os.WriteFile(filepath.Join(h, "stores.yaml"), combined, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h, "layout.yaml")); err != nil {
		t.Fatal(err)
	}
	listed := asMap(t, body(t, kc(h, "store-ls")))
	if listed["repository"] != "filegit" || listed["index"] != "none" {
		t.Fatalf("%#v", listed)
	}
	layout := asMap(t, listed["layout"])
	if layout["repos"] != "data/repos" || layout["projections"] != "data/projections" {
		t.Fatalf("layout %#v", layout)
	}
	body(t, kc(h, "store-set", "--repository", "filegit"))
	engines, err := os.ReadFile(filepath.Join(h, "stores.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(engines), "filegit:") || strings.Contains(string(engines), "data/repos") {
		t.Fatalf("combined yaml should split on store-set: %s", engines)
	}
	if !strings.Contains(string(engines), "10.0.0.8") {
		t.Fatalf("opensearch url dropped: %s", engines)
	}
	split, err := os.ReadFile(filepath.Join(h, "layout.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(split), "repos: data/repos") || !strings.Contains(string(split), "projections: data/projections") {
		t.Fatalf("%s", split)
	}
}

func TestScaleProfileRepoAddDolt(t *testing.T) {
	if testing.Short() {
		t.Skip("live Dolt repo-add belongs to the adapter suite")
	}
	h := testkit.TempDir(t)
	body(t, kc(h, "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(h, "store-set", "--profile", "scale"))
	added := asMap(t, body(t, kc(h, "repo-add", "--repo", "kr://acme/public/core")))
	if added["repositoryId"] != "kr://acme/public/core" || len(fmt.Sprint(added["head"])) < 20 {
		t.Fatalf("scale dolt repo-add %#v", added)
	}
	st := asMap(t, body(t, kc(h, "status")))
	item := asMap(t, st["repos"].([]any)[0])
	if item["driver"] != "dolt" {
		t.Fatalf("driver %#v", item)
	}
	repoDir := filepath.Join(h, fmt.Sprint(item["dir"]))
	if _, err := os.Stat(filepath.Join(repoDir, ".dolt")); err != nil {
		t.Fatalf("native Dolt metadata missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("Dolt adapter silently created Git metadata: %v", err)
	}
}
