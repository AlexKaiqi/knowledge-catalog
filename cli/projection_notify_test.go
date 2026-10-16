package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
)

func TestRetiredProjectionNotifyHTTPHasNoMutationRoute(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	raw, err := json.Marshal(map[string]any{
		"repository": "kr://acme/public/core",
		"value":      map[string]any{"status": "running"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/operations/v1/projections:notice", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:observer")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("notify with observation body status=%d payload=%s", response.StatusCode, payload)
	}

}

func TestDatasetProjectionSyncRefreshesStateWithoutChangingHEAD(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/core"
	body(t, kc(home, "local", "init"))
	body(t, kc(home, "local", "store", "set", "--index", "opensearch"))
	body(t, kc(home, "local", "store", "set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repositoryID)
	status := "running"
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/access" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": map[string]any{"status": status},
			"basis": map[string]any{
				"bindingGeneration": "g1",
				"consistency":       "repeatable",
				"sourceRevision":    status,
				"observedAt":        "2026-08-27T00:00:00Z",
			},
		})
	}))
	t.Cleanup(runtime.Close)
	body(t, kc(home, "writer", "put", testObjectReference(t, repositoryID, "schema/job.runtime", ""), "--repo", repositoryID, "--command-id", "notify-schema", "--value", `{"entity":"Job","aspect":"runtime","fields":{"status":{"type":"string"}}}`))
	body(t, kc(home, "writer", "put", testObjectReference(t, repositoryID, "job/orders", "/properties"), "--repo", repositoryID, "--command-id", "notify-entity", "--value", `{"name":"orders"}`))
	descriptor := putStateBinding(t, home, repositoryID, "job/orders", "runtime", "schema/job.runtime", runtime.URL)
	shared := bindingfixture.Descriptor(runtime.URL)
	shared["identity"] = "shared"
	rawDescriptor, err := json.Marshal(shared)
	if err != nil {
		t.Fatal(err)
	}
	body(t, kc(home, "writer", "put", "urn:kc:object:"+descriptor, "--repo", repositoryID, "--command-id", "shared-descriptor", "--value", string(rawDescriptor)))
	seedTextDeclaration(t, home, repositoryID, "index/job-state", "schema/job.runtime", "status")
	seedIndexDeclaration(t, home, repositoryID, "index/job-state", "status-filter", map[string]any{
		"catalog": "declarative-index/opensearch@2026-10",
		"inputs":  []any{map[string]any{"from": "schema/job.runtime#/fields/status"}},
		"build":   map[string]any{"kind": "keyword"}, "uses": []any{"filter"},
	})
	body(t, kc(home, "dataset", "define", "--dataset", "jobs", "--revision", "1", "--source", repositoryID))
	head := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))
	before := head["commit"].(string)
	properties := asMap(t, body(t, kc(home, "read", "urn:kc:object:job/orders#/properties", "--repo", repositoryID, "--commit", before)))
	stateOf := func(report any) map[string]any {
		t.Helper()
		receipt := asMap(t, report)
		members, ok := receipt["members"].([]any)
		if receipt["dataset"] != "jobs" || receipt["revision"] != float64(1) || !ok || len(members) != 1 || asMap(t, members[0])["repository"] != repositoryID || asMap(t, members[0])["commit"] != before {
			t.Fatalf("Dataset sync lost its accepted revision or fixed source vector: %#v", receipt)
		}
		state := datasetProjectionState(t, report, repositoryID)
		revision, _ := state["revision"].(string)
		digest, _ := state["observationDigest"].(string)
		if state["mode"] != "rebuild" || revision == "" || digest == "" {
			t.Fatalf("Dataset sync omitted its published observation facts: %#v", state)
		}
		return state
	}
	assertObserved := func(report any, expected string) {
		t.Helper()
		read := asMap(t, report)
		observations, ok := read["observations"].([]any)
		if read["repository"] != repositoryID || read["commit"] != before || asMap(t, read["value"])["status"] != expected || !ok || len(observations) != 1 {
			t.Fatalf("State read lost its fixed declaration or current value: %#v", read)
		}
		observation := asMap(t, observations[0])
		declarationDigest, _ := observation["declarationDigest"].(string)
		if observation["declarationCommit"] != before || declarationDigest == "" || asMap(t, observation["basis"])["sourceRevision"] != expected {
			t.Fatalf("State read lost its new observation basis: %#v", observation)
		}
	}

	notified := stateOf(body(t, kc(home, "operations", "projection", "sync", "--dataset", "jobs", "--revision", "1")))
	assertObserved(body(t, kc(home, "read", "urn:kc:object:job/orders#/runtime", "--dataset", "jobs", "--revision", "1")), "running")
	status = "stopped"
	again := stateOf(body(t, kc(home, "operations", "projection", "sync", "--dataset", "jobs", "--revision", "1")))
	if again["revision"] == notified["revision"] || again["observationDigest"] == notified["observationDigest"] {
		t.Fatalf("second refresh must publish a new observation on the same commit: %#v vs %#v", again, notified)
	}
	assertObserved(body(t, kc(home, "read", "urn:kc:object:job/orders#/runtime", "--dataset", "jobs", "--revision", "1")), "stopped")
	after := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))
	if after["commit"] != before {
		t.Fatalf("Dataset sync moved Repository HEAD: %#v", after)
	}
	if current := asMap(t, body(t, kc(home, "read", "urn:kc:object:job/orders#/properties", "--repo", repositoryID, "--commit", before))); !reflect.DeepEqual(current["value"], properties["value"]) {
		t.Fatalf("State refresh changed stored Snapshot content: before=%#v after=%#v", properties, current)
	}

	// Exercise actual HTTP assembly with no background runtime installed. The
	// explicit refresh request may observe its source without configuring a worker.
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", "operator"))
	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	status = "archived"
	remote := stateOf(body(t, kcRemote(t, server.URL, "operator", "operations", "projection", "sync", "--dataset", "jobs", "--revision", "1")))
	if remote["revision"] == again["revision"] || remote["observationDigest"] == again["observationDigest"] {
		t.Fatalf("HTTP refresh without a worker runtime lost its local source or fixed basis: %#v", remote)
	}
	assertObserved(body(t, kcRemote(t, server.URL, "operator", "read", "urn:kc:object:job/orders#/runtime", "--dataset", "jobs", "--revision", "1")), "archived")
	search := asMap(t, postAs(t, server.URL, "/object/v1/search", "operator", map[string]any{"dataset": "jobs", "revision": 1, "equal": []string{"status=archived"}}))
	hits, ok := search["hits"].([]any)
	view := asMap(t, search["searchView"])
	if !ok || len(hits) != 1 || asMap(t, view["snapshots"])[repositoryID] != before || asMap(t, view["projectionRevisions"])[repositoryID] != remote["revision"] {
		t.Fatalf("search did not use the refreshed observation on the fixed source vector: %#v", search)
	}
	if actual := asMap(t, body(t, kcRemote(t, server.URL, "operator", "writer", "head", "--repo", repositoryID))); actual["commit"] != before {
		t.Fatalf("HTTP refresh mutated Snapshot HEAD: %#v", actual)
	}
	if current := asMap(t, body(t, kcRemote(t, server.URL, "operator", "read", "urn:kc:object:job/orders#/properties", "--repo", repositoryID, "--commit", before))); !reflect.DeepEqual(current["value"], properties["value"]) {
		t.Fatalf("HTTP State refresh changed stored Snapshot content: before=%#v after=%#v", properties, current)
	}
}
