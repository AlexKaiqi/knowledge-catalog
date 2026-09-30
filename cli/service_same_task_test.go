package cli_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"kc/internal/testkit"
	serv "kc/server"
)

// service.md U1：同一任务经 CLI 与 HTTP 执行。相同主体与范围下两个入口得到
// 相同的领域结果；仓库推进后两个入口都不泄漏最新；无权主体在两个入口被
// 一致拒绝。
func TestSameTaskViaCLIAndHTTPYieldsSameDomainResult(t *testing.T) {
	home := testkit.TempDir(t)
	catalogID := "kr://acme/catalog"
	repository := "kr://acme/public/parity"
	workspace := "agent"
	principal := "agent:parity"
	body(t, kc(home, "init", "--catalog", catalogID))
	seedRepo(t, home, repository)
	body(t, kc(home, "put", "--command-id", "v1", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"one"}`))
	body(t, kc(home, "put", "--command-id", "v2", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"two"}`))
	body(t, kc(home, "dataset", "define", "--dataset", workspace, "--revision", "1", "--source", repository+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", principal, "--cmd", "read-workspace", "--catalog", catalogID, "--dataset", workspace))

	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cliRead := func() map[string]any {
		hits, ok := body(t, kcRemote(t, server.URL, principal, "read", "--dataset", workspace, "--object", "Policy:one")).([]any)
		if !ok || len(hits) != 1 {
			t.Fatalf("CLI read must resolve one object: %#v", hits)
		}
		return asMap(t, hits[0])
	}
	httpRead := func(as string) (int, any) {
		status, payload, _ := httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read",
			map[string]any{"dataset": workspace, "object": "Policy:one"}, as)
		return status, payload
	}

	// 同一任务：CLI 与 HTTP 都落在 workspace pin 的同一领域结果上。
	fromCLI := cliRead()
	status, payload := httpRead(principal)
	if status != http.StatusOK {
		t.Fatalf("HTTP read status=%d payload=%#v", status, payload)
	}
	hits, _ := payload.([]any)
	if len(hits) != 1 {
		t.Fatalf("HTTP read must resolve one object: %#v", payload)
	}
	fromHTTP := asMap(t, hits[0])
	for _, field := range []string{"objectId", "commit", "value"} {
		gotCLI, okCLI := fromCLI[field]
		gotHTTP, okHTTP := fromHTTP[field]
		if !okCLI || !okHTTP {
			t.Fatalf("both entrances must expose %s: cli=%#v http=%#v", field, fromCLI, fromHTTP)
		}
		if !reflect.DeepEqual(gotCLI, gotHTTP) {
			t.Fatalf("%s drift between entrances: cli=%#v http=%#v", field, gotCLI, gotHTTP)
		}
	}
	if got := fromCLI["value"]; !reflect.DeepEqual(got, map[string]any{"body": "two"}) {
		t.Fatalf("workspace pin must serve the defined version: %#v", got)
	}

	// 仓库推进（v3）：两个入口都不从最新读。
	body(t, kc(home, "put", "--command-id", "v3", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"three"}`))
	if got := cliRead()["value"]; reflect.DeepEqual(got, map[string]any{"body": "three"}) {
		t.Fatalf("CLI must keep the workspace pin, got latest: %#v", got)
	}
	_, payload = httpRead(principal)
	hits, _ = payload.([]any)
	if len(hits) == 1 && reflect.DeepEqual(asMap(t, hits[0])["value"], map[string]any{"body": "three"}) {
		t.Fatalf("HTTP must keep the workspace pin, got latest: %#v", payload)
	}

	// 无权主体：两个入口一致拒绝（授权不随入口变化）。
	if result := kcRemote(t, server.URL, "agent:intruder", "read", "--dataset", workspace, "--object", "Policy:one"); result.Status == 0 {
		t.Fatalf("unauthorized CLI read must fail: %#v", result)
	}
	if status, payload := httpRead("agent:intruder"); status != http.StatusForbidden {
		t.Fatalf("unauthorized HTTP read status=%d payload=%#v", status, payload)
	}
}
