package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	retrieval "kc/discovery"
)

func TestPluginCapabilityCLITransportsMaterialAndFixedBasis(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	t.Setenv("KC_DATASET", "unrelated-inherited-dataset")
	const content = "---\nobject_id: ops/search\naspect_name: index\nmember_key: body\nschema_ref: schema/core/index-declaration/v1\n---\n{}\n"
	path := filepath.Join(t.TempDir(), "candidate.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	var calls int
	network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Kc-As") != "agent:trial-reader" || r.URL.RawQuery != "" {
			t.Errorf("identity/target changed: %#v", r)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/object/v1/plugins/declarative-index/capabilities":
			if r.Method != http.MethodGet || r.ContentLength != 0 {
				t.Errorf("describe acquired a mutation body: %#v", r)
			}
			_, _ = w.Write([]byte(`[{"plugin":"declarative-index","instance":"configured","catalog":"example@1","manifest":{"name":"example","version":"1"}}]`))
		case "/object/v1/plugins/declarative-index/declarations:trial":
			var request retrieval.DeclarationTrialRequest
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&request) != nil || request.Repository != "kr://trial/public" || request.Commit != "fixed-basis" || request.Ref != "" || len(request.Documents) != 1 || request.Documents[0].Content != content || request.Documents[0].Path != "candidate.yaml" {
				t.Errorf("trial lost material or inherited Dataset scope: %#v", request)
			}
			_, _ = w.Write([]byte(`{"repository":"kr://trial/public","commit":"fixed-basis","members":[{"status":"unsupported","reason":"analyzer unavailable","member":{"kind":"Member","objectId":"ops/search","aspectName":"index","memberKey":"body"}}]}`))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	}))
	t.Cleanup(network.Close)
	describe := kcRemote(t, network.URL, "agent:trial-reader", "plugin", "declarative-index", "describe")
	if describe.Status != 0 || !strings.Contains(describe.Stdout, "configured") {
		t.Fatalf("describe: %#v", describe)
	}
	trial := kcRemote(t, network.URL, "agent:trial-reader", "plugin", "declarative-index", "trial", "--repo", "kr://trial/public", "--commit", "fixed-basis", "--file", path)
	if trial.Status != 0 || !strings.Contains(trial.Stdout, "analyzer unavailable") {
		t.Fatalf("trial: %#v", trial)
	}
	if calls != 2 {
		t.Fatalf("expected one transport call per task, got %d", calls)
	}
	for _, args := range [][]string{
		{"plugin", "declarative-index", "describe", "--repo", "kr://secret"},
		{"plugin", "declarative-index", "trial", "--repo", "kr://trial/public"},
		{"plugin", "declarative-index", "trial", "--repo", "kr://trial/public", "--file", path, "--commit", "a", "--ref", "published"},
	} {
		expectCode(t, kcRemote(t, network.URL, "agent:trial-reader", args...), "USAGE_INVALID")
	}
	if calls != 2 {
		t.Fatal("invalid plugin task reached transport")
	}
}
