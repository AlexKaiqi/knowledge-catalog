package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kcclient "kc/client"
)

type remoteDispatchRouteCase struct {
	path   string
	method string
	target string
}

var remoteDispatchRoutes = []remoteDispatchRouteCase{
	{path: "knowledge read", method: http.MethodPost, target: "/knowledge/v1/objects:read"},
	{path: "knowledge resolve", method: http.MethodPost, target: "/knowledge/v1/objects:resolve"},
	{path: "knowledge search", method: http.MethodPost, target: "/knowledge/v1/search"},
	{path: "knowledge relations", method: http.MethodPost, target: "/knowledge/v1/relations:query"},
	{path: "knowledge provenance", method: http.MethodPost, target: "/knowledge/v1/provenance:describe"},
	{path: "knowledge log", method: http.MethodPost, target: "/knowledge/v1/log:query"},
	{path: "knowledge schema describe", method: http.MethodPost, target: "/knowledge/v1/schemas:describe"},
	{path: "knowledge schema list", method: http.MethodPost, target: "/knowledge/v1/schemas:list"},
	{path: "knowledge binding show", method: http.MethodPost, target: "/knowledge/v1/bindings:resolve"},
	{path: "knowledge access", method: http.MethodPost, target: "/knowledge/v1/resources:access"},
	{path: "catalog list", method: http.MethodGet, target: "/catalog/v1/catalogs"},
	{path: "catalog show", method: http.MethodGet, target: "/catalog/v1/catalogs/catalog-A"},
	{path: "catalog audit", method: http.MethodGet, target: "/catalog/v1/catalogs/catalog-A/audit?limit=2"},
	{path: "catalog archive", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/archive"},
	{path: "catalog repo list", method: http.MethodGet, target: "/catalog/v1/catalogs/catalog-A/repositories"},
	{path: "catalog repo register", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/repositories"},
	{path: "catalog repo archive", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/repositories/repo-A/archive"},
	{path: "workspace list", method: http.MethodGet, target: "/catalog/v1/catalogs/catalog-A/workspaces"},
	{path: "workspace show", method: http.MethodGet, target: "/catalog/v1/catalogs/catalog-A/workspaces/agent"},
	{path: "workspace define", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/workspaces"},
	{path: "workspace retire", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/workspaces/agent/retire"},
	{path: "workspace pin", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/workspaces/agent/resolve"},
	{path: "workspace check", method: http.MethodPost, target: "/catalog/v1/catalogs/catalog-A/workspaces/agent/check"},
	{path: "writer put", method: http.MethodPost, target: "/writer/v1/repositories/repo-A/commits"},
	{path: "writer head", method: http.MethodGet, target: "/writer/v1/repositories/repo-A/head?ref=refs%2Fheads%2Fmain"},
	{path: "writer receipt", method: http.MethodGet, target: "/writer/v1/receipts/command-A"},
	{path: "governance proposal create", method: http.MethodPost, target: "/governance/v1/proposals"},
	{path: "governance preview create", method: http.MethodPost, target: "/governance/v1/previews"},
	{path: "governance preview validate", method: http.MethodPost, target: "/governance/v1/previews:validate"},
	{path: "governance validation record", method: http.MethodPost, target: "/governance/v1/validations"},
	{path: "governance proposal merge", method: http.MethodPost, target: "/governance/v1/proposals:merge"},
	{path: "admin grant add", method: http.MethodPost, target: "/admin/v1/grants"},
	{path: "admin grant list", method: http.MethodGet, target: "/admin/v1/grants"},
	{path: "admin grant remove", method: http.MethodDelete, target: "/admin/v1/grants/rule-A"},
	{path: "operations projection sync", method: http.MethodPost, target: "/operations/v1/projections:sync"},
	{path: "operations projection notice", method: http.MethodPost, target: "/operations/v1/projections:notice"},
	{path: "operations projection describe", method: http.MethodPost, target: "/operations/v1/projections:describe"},
	{path: "operations access-spec describe", method: http.MethodPost, target: "/operations/v1/access-specs:describe"},
	{path: "operations hook add", method: http.MethodPost, target: "/operations/v1/hooks"},
	{path: "operations hook list", method: http.MethodGet, target: "/operations/v1/hooks"},
	{path: "operations hook remove", method: http.MethodDelete, target: "/operations/v1/hooks/rule-A"},
	{path: "operations gate add", method: http.MethodPost, target: "/operations/v1/gates"},
	{path: "operations gate list", method: http.MethodGet, target: "/operations/v1/gates"},
	{path: "operations gate remove", method: http.MethodDelete, target: "/operations/v1/gates/rule-A"},
	{path: "operations audit access", method: http.MethodPost, target: "/operations/v1/access-log:query"},
	{path: "operations audit trace", method: http.MethodPost, target: "/operations/v1/traces:query"},
	{path: "operations audit hitmap", method: http.MethodPost, target: "/operations/v1/hitmap:query"},
	{path: "operations feedback record", method: http.MethodPost, target: "/operations/v1/feedback"},
}

func TestRemoteTypedDispatchRoutesSupportedOperations(t *testing.T) {
	protocolHandler := HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{})
	if closer, ok := protocolHandler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	protocolServer := httptest.NewServer(protocolHandler)
	t.Cleanup(protocolServer.Close)

	for _, test := range remoteDispatchRoutes {
		t.Run(test.path, func(t *testing.T) {
			var capturedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.RequestURI() != test.target {
					t.Errorf("request = %s %s, want %s %s", request.Method, request.URL.RequestURI(), test.method, test.target)
				}
				capturedBody, _ = io.ReadAll(request.Body)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"target": test.target})
			}))
			t.Cleanup(server.Close)

			client, err := kcclient.New(kcclient.Config{BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: "agent:test"}}); err != nil {
				t.Fatal(err)
			}
			flags := remoteDispatchTestFlags()
			if test.path == "knowledge log" || test.path == "knowledge provenance" {
				delete(flags, "aspect")
				delete(flags, "member")
			}
			output, err := runRemoteRequest(context.Background(), client, test.path, flags, kcclient.RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			body, ok := output.(map[string]any)
			if !ok || body["target"] != test.target {
				t.Fatalf("response = %#v", output)
			}

			// Replay the exact remote-client payload through the production
			// handler. The empty Home may reject the operation later, but a
			// client/server DTO drift must be detected at the decode boundary.
			replay, err := http.NewRequest(test.method, protocolServer.URL+test.target, bytes.NewReader(capturedBody))
			if err != nil {
				t.Fatal(err)
			}
			if len(capturedBody) > 0 {
				replay.Header.Set("Content-Type", "application/json")
			}
			replay.Header.Set("X-Kc-As", "agent:transport-compatibility")
			response, err := protocolServer.Client().Do(replay)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
				t.Fatalf("production handler rejected the remote route: status=%d body=%s", response.StatusCode, raw)
			}
			if strings.Contains(string(raw), "decode request:") || strings.Contains(string(raw), "request body must contain one JSON object") {
				t.Fatalf("production handler rejected the remote DTO: status=%d body=%s", response.StatusCode, raw)
			}
		})
	}
}

func remoteDispatchTestFlags() map[string]FlagValue {
	return map[string]FlagValue{
		"catalog": "catalog-A", "workspace": "agent", "repo": "repo-A",
		"object": "policy/A", "aspect": "body", "member": "en", "operation": "query", "input": `{"sql":"select 1"}`,
		"query": "runbook", "limit": "2", "revision": "1",
		"source":     []string{"repo-A=refs/heads/main@"},
		"command-id": "command-A", "value": `{"body":"runbook"}`,
		"proposal-id": "proposal-A", "proposal": "proposal-A", "candidate": "refs/heads/proposal-A",
		"preview": "preview-A", "validation": "validation-A", "suite": "contract", "outcome": "PASSED",
		"principal": "user:alice", "action": "knowledge.read", "id": "rule-A",
		"on": "merge", "phase": "pre", "run": "verify", "require": "contract",
		"commit": "commit-A", "ref": "refs/heads/main", "trace-id": "trace-A", "message": "useful",
	}
}
