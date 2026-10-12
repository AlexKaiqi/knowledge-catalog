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

	repository := "kr://acme/public/parity"
	workspace := "agent"
	principal := "agent:parity"
	body(t, kc(home, "init"))
	seedRepo(t, home, repository)
	body(t, kc(home, "put", "--command-id", "v1", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"one"}`))
	body(t, kc(home, "put", "--command-id", "v2", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"two"}`))
	body(t, kc(home, "dataset", "define", "--dataset", workspace, "--revision", "1", "--source", repository+"=refs/heads/main"))
	body(t, kc(home, "allow", "--principal", principal, "--cmd", "read-workspace", "--dataset", workspace))

	handler := serv.HTTPHandlerWithOptions(home, serv.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cliRead := func() map[string]any {
		return readSingleHit(t, body(t, kcRemote(t, server.URL, principal, "read", "--dataset", workspace, "--object", "Policy:one")))
	}
	httpRead := func(as string) (int, any) {
		status, payload, _ := httpSurfaceRequest(t, server, http.MethodPost, "/knowledge/v1/objects:read",
			map[string]any{"dataset": workspace, "object": "Policy:one"}, as)
		return status, payload
	}

	fromCLI := cliRead()
	assertSameResult := func() {
		t.Helper()
		status, payload := httpRead(principal)
		if status != http.StatusOK {
			t.Fatalf("HTTP read status=%d payload=%#v", status, payload)
		}
		results := []map[string]any{cliRead(), readSingleHit(t, payload)}
		for _, result := range results {
			for _, field := range []string{"objectId", "commit", "value"} {
				want, exists := fromCLI[field]
				got, present := result[field]
				if !exists || !present || !reflect.DeepEqual(got, want) {
					t.Fatalf("%s drift from fixed task result: baseline=%#v result=%#v", field, fromCLI, result)
				}
			}
		}
	}

	// 同一任务：CLI 与 HTTP 都落在 workspace pin 的同一领域结果上。
	if fromCLI["objectId"] != "Policy:one" || fromCLI["commit"] == "" || fromCLI["commit"] == nil {
		t.Fatalf("fixed task result must retain object identity and commit: %#v", fromCLI)
	}
	if got := fromCLI["value"]; !reflect.DeepEqual(got, map[string]any{"body": "two"}) {
		t.Fatalf("workspace pin must serve the defined version: %#v", got)
	}
	assertSameResult()

	// 仓库推进（v3）：两个入口都保持原完整结果与 commit。
	body(t, kc(home, "put", "--command-id", "v3", "--repo", repository, "--object", "Policy:one", "--value", `{"body":"three"}`))
	assertSameResult()

	// 无权主体：两个入口一致拒绝（授权不随入口变化）。
	expectCode(t, kcRemote(t, server.URL, "agent:intruder", "read", "--dataset", workspace, "--object", "Policy:one"), "FORBIDDEN")
	if status, payload := httpRead("agent:intruder"); status != http.StatusForbidden || asMap(t, asMap(t, payload)["error"])["code"] != "FORBIDDEN" {
		t.Fatalf("unauthorized HTTP read status=%d payload=%#v", status, payload)
	}
}

func readSingleHit(t *testing.T, payload any) map[string]any {
	t.Helper()
	hits, ok := payload.([]any)
	if !ok || len(hits) != 1 {
		t.Fatalf("read must resolve one object: %#v", payload)
	}
	return asMap(t, hits[0])
}
