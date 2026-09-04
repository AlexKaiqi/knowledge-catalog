package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/internal/testkit"
)

// This journey owns the public commands that do not naturally occur in the
// provider/consumer/governance journeys. Each assertion protects observable
// protocol behavior; it is not a call-only coverage fixture.
func TestCatalogViewsChecksAndKnowledgeResolve(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repositoryID := "kr://acme/public/core"

	body(t, kc(home, "local", "init", "--catalog", catalogID))
	body(t, kc(home, "local", "repository", "attach", "--repo", repositoryID))
	body(t, kc(home, "writer", "put", "--command-id", "coverage-schema", "--repo", repositoryID,
		"--object", "schema/policy.body",
		"--value", `{"entity":"Policy","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, kc(home, "writer", "put", "--command-id", "coverage-object", "--repo", repositoryID,
		"--object", "policy/coverage", "--schema-ref", "schema/policy.body", "--value", `{"body":"coverage"}`))
	body(t, kc(home, "writer", "put", "--command-id", "coverage-binding", "--repo", repositoryID,
		"--object", "Service:coverage", "--aspect", "health", "--value", "null",
		"--value-source", `{"kind":"binding","binding":{"mode":"state","runtime":"health","protocol":"resource-access/v1","operations":{"lookup":{"call":"health.lookup"}}}}`))
	body(t, kc(home, "writer", "put", "--command-id", "coverage-resource", "--repo", repositoryID,
		"--object", "resource/coverage", "--value",
		`{"kind":"ResourceDescriptor","runtime":"sql","protocol":"resource-access/v1","access":{"query":{"call":"sql.query"}}}`))
	head := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))
	if head["repository"] != repositoryID || head["commit"] == "" {
		t.Fatalf("writer head must expose a fixed Connector/preview base: %#v", head)
	}
	body(t, kc(home, "workspace", "define", "--workspace", "coverage", "--revision", "1",
		"--source", repositoryID+"=refs/heads/main@knowledge"))

	inventory := asMap(t, body(t, kc(home, "catalog", "list")))
	listedCatalogs := inventory["catalogs"].([]any)
	if len(listedCatalogs) != 1 || asMap(t, listedCatalogs[0])["id"] != catalogID {
		t.Fatalf("catalog list must return visible Catalog IDs: %#v", inventory)
	}
	if _, ok := asMap(t, listedCatalogs[0])["dir"]; ok {
		t.Fatalf("catalog list must not leak host paths: %#v", listedCatalogs[0])
	}

	repositories := asMap(t, body(t, kc(home, "catalog", "repo", "list")))
	listed := businessRepositories(repositories)
	if len(listed) != 1 || listed[0] != repositoryID {
		t.Fatalf("repository list must expose the registered Catalog member: %#v", repositories)
	}

	workspace := asMap(t, body(t, kc(home, "catalog", "workspace", "show", "--workspace", "coverage")))
	if workspace["workspaceId"] != "coverage" || workspace["revision"] != float64(1) {
		t.Fatalf("workspace show must return the named definition: %#v", workspace)
	}
	if _, ok := workspace["sources"]; ok {
		t.Fatalf("workspace show must not expose selectors: %#v", workspace)
	}
	if repos, _ := workspace["repositories"].([]any); len(repos) != 1 || repos[0] != repositoryID {
		t.Fatalf("workspace show must list member knowledge sources: %#v", workspace)
	}
	expectCode(t, kc(home, "catalog", "workspace", "show", "--workspace", "missing"), "WORKSPACE_INVALID")

	checked := asMap(t, body(t, kc(home, "catalog", "workspace", "check", "--workspace", "coverage")))
	if checked["workspaceId"] != "coverage" || checked["outcome"] != "PASSED" || len(checked["issues"].([]any)) != 0 {
		t.Fatalf("workspace check must validate the command's resolved pin: %#v", checked)
	}
	expectCode(t, kc(home, "catalog", "workspace", "check", "--workspace", "missing"), "WORKSPACE_INVALID")

	adhoc := asMap(t, body(t, kc(home, "workspace", "pin",
		"--source", repositoryID)))
	if asMap(t, adhoc["repositories"])[repositoryID] == "" || adhoc["pinId"] == "" {
		t.Fatalf("temporary Workspace resolve must freeze member commits without defining a named knowledge set: %#v", adhoc)
	}

	accessPlan := asMap(t, body(t, kc(home, "operations", "access-spec", "describe", "--workspace", "coverage")))
	if accessPlan["workspaceId"] != "coverage" || len(accessPlan["specs"].([]any)) != 1 {
		t.Fatalf("access describe must return one logical spec per pinned member: %#v", accessPlan)
	}

	// Schema discovery is bounded and pinned to one Repository basis, so a
	// consumer can browse it without first choosing a knowledge set.
	browsed := asMap(t, body(t, kc(home, "knowledge", "schema", "list", "--repo", repositoryID)))
	if browsed["repository"] != repositoryID || browsed["commit"] == "" || browsed["exhausted"] != true {
		t.Fatalf("schema browse must report the fixed basis and exhaustion: %#v", browsed)
	}
	browsedSchemas := browsed["schemas"].([]any)
	if len(browsedSchemas) != 1 {
		t.Fatalf("schema browse must page the published schema/* namespace: %#v", browsedSchemas)
	}
	if first := asMap(t, browsedSchemas[0]); first["objectId"] != "schema/policy.body" {
		t.Fatalf("schema browse returned an unexpected schema: %#v", first)
	}
	if coverage := asMap(t, browsed["coverage"]); coverage["total"] != float64(1) || coverage["complete"] != true {
		t.Fatalf("schema browse must declare coverage: %#v", coverage)
	}
	expectCode(t, kc(home, "knowledge", "schema", "list", "--repo", repositoryID,
		"--continuation", "not-a-cursor"), "USAGE_INVALID")
	defaultBrowse := asMap(t, body(t, kc(home, "knowledge", "schema", "list", "--repo", repositoryID)))
	zeroBrowse := asMap(t, body(t, kc(home, "knowledge", "schema", "list", "--repo", repositoryID, "--limit", "0")))
	if len(zeroBrowse["schemas"].([]any)) != len(defaultBrowse["schemas"].([]any)) || zeroBrowse["exhausted"] != true {
		t.Fatalf("schema browse --limit 0 must mean the default page: %#v vs %#v", zeroBrowse, defaultBrowse)
	}
	expectCode(t, kc(home, "knowledge", "schema", "list", "--repo", repositoryID, "--limit", "201"), "USAGE_INVALID")

	var directRequest map[string]any
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/access" {
			t.Errorf("resource runtime request = %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode resource runtime request: %v", err)
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if request["descriptor"] != nil {
			directRequest = request
			_ = json.NewEncoder(w).Encode(map[string]any{
				"operation": "query",
				"result":    map[string]any{"rows": []string{"1"}, "rowCount": 1},
				"basis":     map[string]any{"runtimeGeneration": "sql-v1"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{"status": "healthy"},
			"basis": map[string]any{
				"bindingGeneration": "coverage-runtime-v1",
				"consistency":       "repeatable",
				"sourceRevision":    "health-1",
				"observedAt":        "2026-08-30T00:00:00Z",
			},
		})
	}))
	t.Cleanup(runtime.Close)
	t.Setenv("KC_RESOURCE_ACCESS_URL", runtime.URL)
	resource := asMap(t, body(t, kc(home, "knowledge", "access", "--workspace", "coverage",
		"--object", "Service:coverage", "--aspect", "health")))
	observations := resource["observations"].([]any)
	if len(observations) != 1 || asMap(t, asMap(t, observations[0])["value"])["status"] != "healthy" {
		t.Fatalf("resource access must resolve the pinned declaration and call the runtime: %#v", resource)
	}
	direct := asMap(t, body(t, kc(home, "knowledge", "access", "--workspace", "coverage",
		"--object", "resource/coverage", "--operation", "query", "--input", `{"sql":"SELECT 1"}`)))
	if asMap(t, direct["result"])["rowCount"] != float64(1) || asMap(t, direct["basis"])["runtimeGeneration"] != "sql-v1" {
		t.Fatalf("descriptor operation must return the runtime result: %#v", direct)
	}
	coordinate := asMap(t, directRequest["descriptor"])
	if coordinate["objectId"] != "resource/coverage" || coordinate["repository"] != repositoryID || coordinate["commit"] == "" {
		t.Fatalf("runtime did not receive a pinned descriptor coordinate: %#v", directRequest)
	}
	if directRequest["operation"] != "query" || directRequest["call"] != "sql.query" || asMap(t, directRequest["input"])["sql"] != "SELECT 1" {
		t.Fatalf("runtime operation did not come from descriptor + input: %#v", directRequest)
	}
	expectCode(t, kc(home, "knowledge", "access", "--workspace", "coverage",
		"--object", "resource/coverage", "--operation", "missing", "--input", `{}`), "CAPABILITY_UNSATISFIED")
	expectCode(t, kc(home, "knowledge", "access", "--workspace", "coverage",
		"--object", "resource/coverage", "--input", `{}`), "USAGE_INVALID")

	resolved := body(t, kc(home, "knowledge", "resolve", "--workspace", "coverage", "--object", "policy/coverage")).([]any)
	if len(resolved) != 1 || asMap(t, resolved[0])["status"] != "RESOLVED" {
		t.Fatalf("knowledge resolve must report object status at the Workspace pin: %#v", resolved)
	}
	aspectResolved := body(t, kc(home, "knowledge", "resolve", "--workspace", "coverage",
		"--object", "Service:coverage", "--aspect", "health")).([]any)
	if len(aspectResolved) != 1 || asMap(t, aspectResolved[0])["status"] != "RESOLVED" ||
		asMap(t, asMap(t, aspectResolved[0])["address"])["aspectName"] != "health" {
		t.Fatalf("knowledge resolve --aspect must report Address status: %#v", aspectResolved)
	}
	expectCode(t, kc(home, "knowledge", "resolve", "--workspace", "coverage",
		"--object", "policy/coverage", "--member", "user:bob"), "USAGE_INVALID")
	expectCode(t, kc(home, "workspace", "pin", "--workspace", "coverage", "--object", "policy/coverage"), "USAGE_INVALID")
	absent := body(t, kc(home, "knowledge", "resolve", "--workspace", "coverage", "--object", "missing/coverage")).([]any)
	if len(absent) != 0 {
		t.Fatalf("workspace resolve of a missing object is an empty union: %#v", absent)
	}

	history := asMap(t, body(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage", "--limit", "1")))
	if history["exhausted"] != true || len(history["logs"].([]any)) != 1 {
		t.Fatalf("knowledge log must return a bounded page: %#v", history)
	}
	zeroLog := asMap(t, body(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage", "--limit", "0")))
	if zeroLog["exhausted"] != true || len(zeroLog["logs"].([]any)) != 1 {
		t.Fatalf("knowledge log --limit 0 must mean the default page: %#v", zeroLog)
	}
	expectCode(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage",
		"--continuation", "not-a-cursor"), "USAGE_INVALID")
	expectCode(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage",
		"--limit", "201"), "USAGE_INVALID")
	expectCode(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage",
		"--aspect", "health"), "USAGE_INVALID")
	expectCode(t, kc(home, "knowledge", "log", "--workspace", "coverage", "--object", "policy/coverage",
		"--member", "user:bob"), "USAGE_INVALID")
	missing := asMap(t, body(t, kc(home, "knowledge", "resolve", "--repo", repositoryID, "--object", "missing/coverage")))
	if missing["status"] != "UNRESOLVED" {
		t.Fatalf("maintainer resolve of a missing object must be UNRESOLVED: %#v", missing)
	}
}
