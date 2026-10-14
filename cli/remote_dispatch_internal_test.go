package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	inv "kc/invocation"
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kcclient "kc/client"
)

type remoteDispatchRouteCase struct {
	path     string
	method   string
	target   string
	flags    map[string]FlagValue
	operands []string
}

var remoteDispatchRoutes = []remoteDispatchRouteCase{
	{path: "read", method: http.MethodPost, target: "/object/v1/objects:read", flags: map[string]FlagValue{"commit": "commit-A"}, operands: []string{"urn:kc:object:repo-A:policy%2FA#/body/en"}},
	{path: "resolve", method: http.MethodPost, target: "/object/v1/objects:resolve", flags: map[string]FlagValue{"repo": "repo-A", "object": "policy/A", "aspect": "body", "member": "en", "commit": "commit-A"}},
	{path: "search", method: http.MethodPost, target: "/object/v1/search", flags: map[string]FlagValue{"dataset": "agent", "query": "runbook", "limit": "2"}},
	{path: "relations", method: http.MethodPost, target: "/object/v1/relations:query", flags: map[string]FlagValue{"dataset": "agent", "object": "kc://repo-A/policy/A", "limit": "2"}},
	{path: "traverse", method: http.MethodPost, target: "/object/v1/traverse:query", flags: map[string]FlagValue{"dataset": "agent", "object": "kc://repo-A/policy/A", "max-hops": "2", "limit": "2"}},
	{path: "provenance", method: http.MethodPost, target: "/object/v1/provenance:describe", flags: map[string]FlagValue{"repo": "repo-A", "object": "policy/A", "commit": "commit-A"}},
	{path: "log", method: http.MethodPost, target: "/object/v1/log:query", flags: map[string]FlagValue{"repo": "repo-A", "object": "policy/A", "commit": "commit-A", "limit": "2"}},
	{path: "schema describe", method: http.MethodPost, target: "/object/v1/schemas:describe", flags: map[string]FlagValue{"repo": "repo-A", "object": "policy/A", "commit": "commit-A"}},
	{path: "schema list", method: http.MethodPost, target: "/object/v1/schemas:list", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A", "limit": "2"}},
	{path: "binding show", method: http.MethodPost, target: "/object/v1/bindings:resolve", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A", "object": "policy/A", "aspect": "body", "member": "en"}},
	{path: "access", method: http.MethodPost, target: "/object/v1/resources:access", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A", "object": "policy/A", "aspect": "body", "member": "en"}},
	{path: "invoke", method: http.MethodPost, target: "/object/v1/resources:access", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A", "object": "policy/A", "operation": "query", "input": `{"sql":"select 1"}`}},
	{path: "show", method: http.MethodGet, target: "/snapshot/v1/inventory"},
	{path: "registry audit", method: http.MethodGet, target: "/snapshot/v1/audit?limit=2"},
	{path: "attach", method: http.MethodPost, target: "/snapshot/v1/repositories"},
	{path: "create", method: http.MethodPost, target: "/snapshot/v1/connections"},
	{path: "detach", method: http.MethodDelete, target: "/snapshot/v1/repositories/repo-A"},
	{path: "dataset define", method: http.MethodPost, target: "/snapshot/v1/datasets"},
	{path: "dataset retire", method: http.MethodPost, target: "/snapshot/v1/datasets/agent/retire"},
	{path: "pin", method: http.MethodPost, target: "/snapshot/v1/datasets/agent/resolve"},
	{path: "pin check", method: http.MethodPost, target: "/snapshot/v1/datasets/agent/check"},
	{path: "writer put", method: http.MethodPost, target: "/object/v1/objects:put", flags: map[string]FlagValue{"kind": "Member", "command-id": "command-A", "value": `{"body":"runbook"}`, "ref": "refs/heads/main", "message": "useful"}, operands: []string{"urn:kc:object:repo-A:policy%2FA#/body/en"}},
	{path: "writer head", method: http.MethodGet, target: "/writer/v1/repositories/repo-A/head?ref=refs%2Fheads%2Fmain"},
	{path: "writer receipt", method: http.MethodGet, target: "/writer/v1/receipts/command-A"},
	{path: "governance proposal create", method: http.MethodPost, target: "/governance/v1/proposals"},
	{path: "governance preview create", method: http.MethodPost, target: "/governance/v1/previews"},
	{path: "governance preview validate", method: http.MethodPost, target: "/governance/v1/previews:validate"},
	{path: "governance validation record", method: http.MethodPost, target: "/governance/v1/validations"},
	{path: "governance proposal merge", method: http.MethodPost, target: "/governance/v1/proposals:merge"},
	{path: "grant add", method: http.MethodPost, target: "/admin/v1/grants"},
	{path: "grant list", method: http.MethodGet, target: "/admin/v1/grants?action=object.read&principal=user%3Aalice"},
	{path: "grant remove", method: http.MethodDelete, target: "/admin/v1/grants/rule-A"},
	{path: "operations projection sync", method: http.MethodPost, target: "/operations/v1/projections:sync", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A", "ref": "refs/heads/main"}},
	{path: "operations projection notice", method: http.MethodPost, target: "/operations/v1/projections:notice", flags: map[string]FlagValue{"repo": "repo-A", "object": "policy/A", "aspect": "body", "ref": "refs/heads/main"}},
	{path: "operations projection describe", method: http.MethodPost, target: "/operations/v1/projections:describe", flags: map[string]FlagValue{"repo": "repo-A", "commit": "commit-A"}},
	{path: "operations access-spec describe", method: http.MethodPost, target: "/operations/v1/access-specs:describe", flags: map[string]FlagValue{"dataset": "agent"}},
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
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	protocolHandler := serv.HTTPHandlerWithOptions(t.TempDir(), serv.HTTPServerOptions{})
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
				if test.path == "grant list" {
					_ = json.NewEncoder(w).Encode(map[string]any{"rules": []inv.AllowRule{{ID: "rule-A", Principal: "user:alice", Repo: "repo-A", Actions: []string{"object.read"}}}})
					return
				}
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
			if test.flags != nil {
				flags = test.flags
			}
			if test.path == "create" {
				flags = map[string]FlagValue{"name": "repo-A"}
			}
			if test.path == "traverse" {
				flags["max-hops"] = "2"
			}
			if test.path == "governance preview create" {
				flags["pin"] = `{"setId":"agent","revision":1,"repositories":{"repo-A":"commit-A"},"pinId":"pin-A"}`
			}
			if test.path == "log" || test.path == "provenance" {
				delete(flags, "aspect")
				delete(flags, "member")
			}
			if test.path == "access" {
				delete(flags, "operation")
				delete(flags, "input")
			}
			if test.path == "invoke" {
				delete(flags, "aspect")
				delete(flags, "member")
			}
			if len(test.operands) > 0 {
				bound := false
				for _, entry := range objectCLIEntries {
					if entry.Path == test.path {
						if err := bindObjectOperands(entry, flags, test.operands); err != nil {
							t.Fatal(err)
						}
						bound = true
						break
					}
				}
				if !bound {
					t.Fatalf("Object command %s has no operand binding", test.path)
				}
			}
			output, err := runFixtureRemoteRequest(context.Background(), client, test.path, flags, kcclient.RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if test.path == "pin" {
				_, ok := output.(inv.TaskDatasetPin)
				if !ok {
					t.Fatalf("response = %#v", output)
				}
			} else if test.path == "grant list" {
				body, ok := output.(map[string]any)
				if !ok || body["allow"] != true || body["ruleId"] != "rule-A" {
					t.Fatalf("legacy grant question response = %#v", output)
				}
			} else {
				body, ok := output.(map[string]any)
				if !ok || body["target"] != test.target {
					t.Fatalf("response = %#v", output)
				}
			}

			// Replay the exact remote-client payload through the production
			// handler. The empty home may reject the operation later, but a
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
		"dataset": "agent", "repo": "repo-A",
		"object": "policy/A", "aspect": "body", "member": "en", "operation": "query", "input": `{"sql":"select 1"}`,
		"query": "runbook", "limit": "2", "revision": "1",
		"source":     []string{"repo-A=refs/heads/main@"},
		"command-id": "command-A", "value": `{"body":"runbook"}`,
		"proposal-id": "proposal-A", "proposal": "proposal-A", "candidate": "refs/heads/proposal-A",
		"preview": "preview-A", "validation": "validation-A", "suite": "contract", "outcome": "PASSED",
		"principal": "user:alice", "action": "object.read", "id": "rule-A",
		"on": "merge", "phase": "pre", "run": "verify", "require": "contract",
		"commit": "commit-A", "ref": "refs/heads/main", "trace-id": "trace-A", "message": "useful",
	}
}
