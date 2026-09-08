package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/catalog"
	"kc/kernel"
)

func TestProductSearchTransmitsRepositoryBasis(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_WORKSPACE", "")
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
	result := Run([]string{"--server", server.URL, "--as", "agent:reader", "knowledge", "search", "--repo", "kr://acme/source", "--ref", "refs/heads/release", "--commit", "fixed", "--query", "note"})
	if result.Status != 0 {
		t.Fatal(result.Stdout)
	}
	if got["repository"] != "kr://acme/source" || got["ref"] != "refs/heads/release" || got["commit"] != "fixed" {
		t.Fatalf("search lost repository basis: %#v", got)
	}
	var request knowledgeSearchRequest
	raw, _ := json.Marshal(got)
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	flags := request.flags()
	if FlagString(flags, "repo") != "kr://acme/source" || FlagString(flags, "ref") != "refs/heads/release" || FlagString(flags, "commit") != "fixed" {
		t.Fatalf("HTTP search lost repository basis: %#v", flags)
	}
}

func TestProductTemporaryTaskPinReachesEveryKnowledgeDTO(t *testing.T) {
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	t.Setenv("KC_HOME", t.TempDir())
	t.Setenv("KC_WORKSPACE", "ambient-must-not-replace-explicit-pin")
	t.Setenv("KC_AUTH_TOKEN", "")
	raw := `{"workspaceId":"","revision":1,"repositories":{"kr://acme/source":"fixed"},"pinId":"pin-id","definition":{"workspaceId":"","revision":1,"sources":[{"repository":"kr://acme/source","selector":"refs/heads/main"}]}}`
	for _, args := range [][]string{
		{"read", "--object", "note/one"}, {"resolve", "--object", "note/one"},
		{"search", "--query", "note"}, {"relations", "--object", "note/one"},
		{"provenance", "--object", "note/one"}, {"log", "--object", "note/one"},
		{"schema", "describe", "--object", "schema/note"},
		{"binding", "show", "--object", "note/one", "--aspect", "preview"},
		{"access", "--object", "note/one", "--aspect", "preview"},
		{"invoke", "--object", "resource/one", "--operation", "GET", "--input", "{}"},
	} {
		t.Run(strings.Join(args[:1], " "), func(t *testing.T) {
			var got map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()
			argv := append([]string{"--server", server.URL, "--as", "agent:reader", "knowledge"}, args...)
			argv = append(argv, "--pin", raw)
			result := Run(argv)
			if result.Status != 0 {
				t.Fatal(result.Stdout)
			}
			if got["definition"] == nil || got["pin"] == nil || got["workspace"] != nil {
				t.Fatalf("lost or replaced temporary task coordinates: %#v", got)
			}
			pin := got["pin"].(map[string]any)
			if pin["definition"] != nil || pin["pinId"] != "pin-id" {
				t.Fatalf("pin/definition were not separated on wire: %#v", got)
			}
			// Decode using the production handler's strict DTO, not the client type.
			body, _ := json.Marshal(got)
			var q knowledgeReadRequest
			if args[0] == "read" {
				if err := catalog.DecodeJSON(body, &q); err != nil {
					t.Fatal(err)
				}
				if !servingWorkspace(q.flags()) {
					t.Fatal("HTTP request did not select composed read")
				}
			}
		})
	}
}

func TestTemporaryDefinitionDoesNotBypassConsumeGrant(t *testing.T) {
	q := AllowQuery{Principal: "reader", Action: "knowledge.read", Catalog: "kr://acme/catalog"}
	read := AllowRule{ID: "read", Principal: "reader", Actions: []string{"knowledge.read"}, Repo: "kr://acme/source"}
	if err := authorizeWorkspaceKnowledge([]AllowRule{read}, q, true); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("temporary composition bypassed consume: %v", err)
	}
	named := AllowRule{ID: "named", Principal: "reader", Actions: []string{"workspace.consume"}, Catalog: q.Catalog, Workspace: "named-only"}
	if err := authorizeWorkspaceKnowledge([]AllowRule{named, read}, q, true); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("named-only grant authorized arbitrary temporary composition: %v", err)
	}
	named.Workspace = ""
	if err := authorizeWorkspaceKnowledge([]AllowRule{named, read}, q, true); err != nil {
		t.Fatal(err)
	}
	q.Action = "knowledge.search"
	if err := authorizeWorkspaceKnowledge([]AllowRule{named, read}, q, true); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("consume implied SEARCH: %v", err)
	}
}

func TestTemporaryReplayRejectsChangedMembershipAndLayout(t *testing.T) {
	source := catalog.WorkspaceSource{Repository: "kr://acme/source", Selector: "refs/heads/main"}
	def := catalog.WorkspaceDefinition{Revision: 1, Sources: []catalog.WorkspaceSource{source}}
	pin := catalog.ResolvedWorkspace{Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.Repository: "fixed"}}
	pin.PinID = catalog.HashResolved("", def.Sources, pin.Repositories)
	raw, _ := json.Marshal(pin)
	if _, err := decodeReplayPin(def, string(raw)); err != nil {
		t.Fatal(err)
	}
	def.Sources[0].Path = catalog.MountPath("changed-layout")
	if _, err := decodeReplayPin(def, string(raw)); kernel.CodeOf(err) != kernel.ErrWorkspaceInvalid {
		t.Fatalf("changed layout reused pin identity: %v", err)
	}
	def.Sources[0] = source
	def.Sources = append(def.Sources, catalog.WorkspaceSource{Repository: "kr://acme/other", Selector: "refs/heads/main"})
	if _, err := decodeReplayPin(def, string(raw)); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("changed membership reused pin: %v", err)
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
	for _, flag := range []string{"pin", "workspace-file"} {
		result := Run([]string{"--server", server.URL, "--as", "agent:reader", "knowledge", "read", "--repo", "kr://acme/source", "--" + flag, "explicit-task.json", "--object", "note/one"})
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("explicit --%s was silently discarded: %s", flag, result.Stdout)
		}
	}
	if called {
		t.Error("mixed basis must fail before issuing a request")
	}
}

func TestHTTPKnowledgeRejectsMixedPinAndRepositoryBasis(t *testing.T) {
	handler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	for _, route := range []string{"/knowledge/v1/objects:read", "/knowledge/v1/search"} {
		request := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"repository":"kr://acme/source","pin":{"workspaceId":"","revision":1,"repositories":{"kr://acme/source":"fixed"}},"object":"note/one"}`))
		if route == "/knowledge/v1/search" {
			request = httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{"repository":"kr://acme/source","pin":{"workspaceId":"","revision":1,"repositories":{"kr://acme/source":"fixed"}},"query":"note"}`))
		}
		request.Header.Set("X-Kc-As", "agent:reader")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if !strings.Contains(response.Body.String(), "USAGE_INVALID") || !strings.Contains(response.Body.String(), "do not mix") {
			t.Errorf("mixed HTTP basis reached repository path: %s", response.Body.String())
		}
	}
}

func TestTemporaryCompositionUsesEveryMemberScopedResolveAndConsume(t *testing.T) {
	home := t.TempDir()
	rules := []AllowRule{
		{ID: "a", Principal: "reader", Repo: "kr://acme/a", Actions: []string{"workspace.resolve", "workspace.consume", "knowledge.read"}},
		{ID: "b", Principal: "reader", Repo: "kr://acme/b", Actions: []string{"workspace.resolve", "workspace.consume", "knowledge.read"}},
	}
	if err := WriteAllow(home, AllowFile{Rules: rules}); err != nil {
		t.Fatal(err)
	}
	definition := &catalog.WorkspaceDefinition{Revision: 1, Sources: []catalog.WorkspaceSource{{Repository: "kr://acme/a", Selector: "refs/heads/main"}, {Repository: "kr://acme/b", Selector: "refs/heads/main"}}}
	flags := map[string]FlagValue{"as": "reader", "catalog": "kr://acme/catalog", workspaceDefinitionFlag: definition}
	for _, action := range []string{"workspace.resolve", "knowledge.read"} {
		if err := authorize(home, action, flags, nil); err != nil {
			t.Fatalf("member-scoped %s blocked: %v", action, err)
		}
	}
	if err := authorize(home, "catalog.read", flags, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("member share widened catalog access: %v", err)
	}
	definition.Sources = append(definition.Sources, catalog.WorkspaceSource{Repository: "kr://acme/unshared", Selector: "refs/heads/main"})
	for _, action := range []string{"workspace.resolve", "knowledge.read"} {
		if err := authorize(home, action, flags, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("unshared member allowed %s: %v", action, err)
		}
	}
}
