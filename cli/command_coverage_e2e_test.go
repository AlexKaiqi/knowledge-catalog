package cli_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/internal/testkit"
)

// This journey owns the public commands that do not naturally occur in the
// provider/consumer/governance journeys. Each assertion protects observable
// protocol behavior; it is not a call-only coverage fixture.
func TestCatalogViewsChecksSnapshotExportAndMountMaintenance(t *testing.T) {
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
	body(t, kc(home, "catalog", "workspace", "define", "--workspace", "coverage", "--revision", "1",
		"--source", repositoryID+"=refs/heads/main@knowledge"))

	repositories := asMap(t, body(t, kc(home, "catalog", "repository", "list")))
	listed := repositories["repositories"].([]any)
	if len(listed) != 1 || listed[0] != repositoryID {
		t.Fatalf("repository list must expose the registered Catalog member: %#v", repositories)
	}

	workspace := asMap(t, body(t, kc(home, "catalog", "workspace", "show", "--workspace", "coverage")))
	if workspace["workspaceId"] != "coverage" || workspace["revision"] != float64(1) {
		t.Fatalf("workspace show must return the named definition: %#v", workspace)
	}
	expectCode(t, kc(home, "catalog", "workspace", "show", "--workspace", "missing"), "WORKSPACE_INVALID")

	checked := asMap(t, body(t, kc(home, "catalog", "workspace", "check", "--workspace", "coverage")))
	if checked["workspaceId"] != "coverage" || checked["outcome"] != "PASSED" || len(checked["issues"].([]any)) != 0 {
		t.Fatalf("workspace check must validate the command's resolved pin: %#v", checked)
	}
	expectCode(t, kc(home, "catalog", "workspace", "check", "--workspace", "missing"), "WORKSPACE_INVALID")

	accessPlan := asMap(t, body(t, kc(home, "operations", "access", "describe", "--workspace", "coverage")))
	if accessPlan["workspaceId"] != "coverage" || len(accessPlan["specs"].([]any)) != 1 {
		t.Fatalf("access describe must return one logical spec per pinned member: %#v", accessPlan)
	}

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
	resource := asMap(t, body(t, kc(home, "resource", "access", "--workspace", "coverage",
		"--object", "Service:coverage", "--aspect", "health")))
	observations := resource["observations"].([]any)
	if len(observations) != 1 || asMap(t, asMap(t, observations[0])["value"])["status"] != "healthy" {
		t.Fatalf("resource access must resolve the pinned declaration and call the runtime: %#v", resource)
	}
	direct := asMap(t, body(t, kc(home, "resource", "access", "--workspace", "coverage",
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
	expectCode(t, kc(home, "resource", "access", "--workspace", "coverage",
		"--object", "resource/coverage", "--operation", "missing", "--input", `{}`), "CAPABILITY_UNSATISFIED")
	expectCode(t, kc(home, "resource", "access", "--workspace", "coverage",
		"--object", "resource/coverage", "--input", `{}`), "USAGE_INVALID")

	exportFile := filepath.Join(t.TempDir(), "snapshot.jsonl")
	receipt := asMap(t, body(t, kc(home, "maintenance", "snapshot", "export", "--repo", repositoryID,
		"--ref", "refs/heads/main", "--out", exportFile)))
	if receipt["repository"] != repositoryID || receipt["objects"].(float64) < 1 {
		t.Fatalf("snapshot export must return a non-empty receipt: %#v", receipt)
	}
	assertJSONLines(t, exportFile, int(receipt["objects"].(float64)))
	expectCode(t, kc(home, "maintenance", "snapshot", "export", "--repo", repositoryID,
		"--ref", "refs/heads/main", "--out", exportFile), "USAGE_INVALID")

	checkoutDir := filepath.Join(t.TempDir(), "checkout")
	expectCode(t, kc(home, "maintenance", "workspace", "status", "--workspace", "coverage", "--to", checkoutDir), "USAGE_INVALID")
	expectCode(t, kc(home, "maintenance", "workspace", "sync", "--workspace", "coverage", "--to", checkoutDir), "USAGE_INVALID")
	body(t, kc(home, "maintenance", "workspace", "checkout", "--workspace", "coverage", "--to", checkoutDir))

	status := asMap(t, body(t, kc(home, "maintenance", "workspace", "status", "--workspace", "coverage", "--to", checkoutDir)))
	statusMounts := status["mounts"].([]any)
	if len(statusMounts) != 1 || asMap(t, statusMounts[0])["repository"] != repositoryID {
		t.Fatalf("workspace status must report every pinned mount: %#v", status)
	}

	synced := asMap(t, body(t, kc(home, "maintenance", "workspace", "sync", "--workspace", "coverage", "--to", checkoutDir)))
	syncMounts := synced["mounts"].([]any)
	if len(syncMounts) != 1 || asMap(t, syncMounts[0])["repository"] != repositoryID {
		t.Fatalf("workspace sync must report every mount outcome: %#v", synced)
	}
}

func assertJSONLines(t *testing.T, path string, want int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("snapshot export row %d is not JSON: %v", count+1, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("snapshot export wrote %d rows, receipt reports %d", count, want)
	}
}
