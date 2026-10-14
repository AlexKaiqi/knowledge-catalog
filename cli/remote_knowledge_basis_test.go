package cli

import (
	"encoding/json"
	"kc/invoke"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductSearchTransmitsRepositoryBasis(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_DATASET", "")
	t.Setenv("KC_AUTH_TOKEN", "")
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/knowledge/v1/search" {
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	result := Run([]string{"--server", server.URL, "--as", "agent:reader", "search", "--repo", "kr://acme/source", "--ref", "refs/heads/release", "--commit", "fixed", "--query", "note"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if got["repository"] != "kr://acme/source" || got["ref"] != "refs/heads/release" || got["commit"] != "fixed" {
		t.Fatalf("search lost repository basis: %#v", got)
	}
	var request serv.KnowledgeSearchRequest
	raw, _ := json.Marshal(got)
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.Flags()
	if invoke.FlagString(flags, "repo") != "kr://acme/source" || invoke.FlagString(flags, "ref") != "refs/heads/release" || invoke.FlagString(flags, "commit") != "fixed" {
		t.Fatalf("HTTP search lost repository basis: %#v", flags)
	}
}

func TestProductCLIRejectsPinOnEveryKnowledgeCommand(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	raw := `{"setId":"","revision":1,"repositories":{"kr://acme/source":"fixed"},"pinId":"pin-id","definition":{"setId":"","revision":1,"sources":[{"repository":"kr://acme/source","selector":"refs/heads/main"}]}}`
	for _, args := range [][]string{
		{"read", "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fone"}, {"resolve", "--object", "note/one"},
		{"search", "--query", "note"}, {"relations", "--object", "note/one"},
		{"provenance", "--object", "note/one"}, {"log", "--object", "note/one"},
		{"schema", "describe", "--object", "schema/note"},
		{"binding", "show", "--object", "note/one", "--aspect", "preview"},
		{"access", "--object", "note/one", "--aspect", "preview"},
		{"invoke", "--object", "resource/one", "--operation", "GET", "--input", "{}"},
	} {
		t.Run(strings.Join(args[:1], " "), func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			argv := append([]string{"--server", server.URL, "--as", "agent:reader"}, args...)
			argv = append(argv, "--pin", raw)
			result := Run(argv)
			if result.Status == 0 || !strings.Contains(result.Stdout, "rejects --pin") {
				t.Fatalf("product CLI must reject --pin: %s", result.Stdout)
			}
			if called {
				t.Fatal("rejected --pin must not issue a request")
			}
		})
	}
}

func TestProductKnowledgeRejectsMixedTemporaryAndRepositoryBasis(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_AUTH_TOKEN", "")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	for _, flag := range []string{"pin", "dataset-file"} {
		result := Run([]string{"--server", server.URL, "--as", "agent:reader", "read", "urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fone", "--" + flag, "explicit-task.json"})
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("explicit --%s was silently discarded: %s", flag, result.Stdout)
		}
	}
	if called {
		t.Error("mixed basis must fail before issuing a request")
	}
}

func TestHTTPKnowledgeRejectsMixedPinAndRepositoryBasis(t *testing.T) {
	handler := serv.HTTPHandlerWithOptions(t.TempDir(), serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	for _, route := range []string{"/knowledge/v1/objects:read", "/knowledge/v1/search"} {
		request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"reference":"urn:kc:object:kr%3A%2F%2Facme%2Fsource:note%2Fone","repository":"kr://acme/source","pin":{"setId":"","revision":1,"repositories":{"kr://acme/source":"fixed"}}}`))
		if route == "/knowledge/v1/search" {
			request = httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"repository":"kr://acme/source","pin":{"setId":"","revision":1,"repositories":{"kr://acme/source":"fixed"}},"query":"note"}`))
		}
		request.Header.Set("X-Kc-As", "agent:reader")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if !strings.Contains(response.Body.String(), "USAGE_INVALID") || (route == "/knowledge/v1/search" && !strings.Contains(response.Body.String(), "do not mix")) || (route == "/knowledge/v1/objects:read" && !strings.Contains(response.Body.String(), `unknown field \"repository\"`)) {
			t.Errorf("mixed HTTP basis reached repository path: %s", response.Body.String())
		}
	}
}
