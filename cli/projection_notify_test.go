package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func TestProjectionNotifyHTTPRejectsObservationBody(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/catalog"))
	handler := cli.HTTPHandler(home)
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
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("notify with observation body status=%d payload=%s", response.StatusCode, payload)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err, string(payload))
	}
	if asMap(t, decoded["error"])["code"] != "USAGE_INVALID" {
		t.Fatalf("notify with observation body must be USAGE_INVALID: %#v", decoded)
	}
}

func TestProjectionNotifyPullsBoundStateWithoutChangingHEAD(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	home := testkit.TempDir(t)
	repositoryID := "kr://acme/public/core"
	body(t, kc(home, "local", "init", "--catalog", "kr://acme/catalog"))
	body(t, kc(home, "local", "store", "set", "--index", "opensearch"))
	body(t, kc(home, "local", "store", "set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repositoryID)
	body(t, kc(home, "writer", "put", "--command-id", "notify-schema", "--repo", repositoryID,
		"--object", "schema/job.runtime",
		"--value", `{"entity":"Job","aspect":"runtime","fields":{"status":{"type":"string","access":["text","filter"]}}}`))
	body(t, kc(home, "writer", "put", "--command-id", "notify-binding", "--repo", repositoryID,
		"--object", "Job:orders", "--aspect", "runtime", "--schema-ref", "schema/job.runtime", "--value", "null",
		"--value-source", `{"kind":"binding","binding":{"mode":"state","runtime":"scheduler","protocol":"resource-access/v1","operations":{"lookup":{"call":"job.status"}}}}`))
	head := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))
	before := head["commit"]

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
	t.Setenv("KC_RESOURCE_ACCESS_URL", runtime.URL)

	notified := asMap(t, body(t, kc(home, "operations", "projection", "notice",
		"--repo", repositoryID, "--object", "Job:orders", "--aspect", "runtime")))
	if notified["repository"] != repositoryID || notified["basisCommit"] != before || notified["revision"] == "" {
		t.Fatalf("notify must publish State at the unchanged HEAD: %#v want commit %v", notified, before)
	}
	status = "stopped"
	again := asMap(t, body(t, kc(home, "operations", "projection", "notice",
		"--repo", repositoryID, "--object", "Job:orders", "--aspect", "runtime", "--source-revision", "r2")))
	if again["revision"] == "" || again["revision"] == notified["revision"] || again["basisCommit"] != before {
		t.Fatalf("second notice must publish a new observation on the same commit: %#v vs %#v", again, notified)
	}
	after := asMap(t, body(t, kc(home, "writer", "head", "--repo", repositoryID)))
	if after["commit"] != before {
		t.Fatalf("notify moved Repository HEAD: %#v", after)
	}
}
