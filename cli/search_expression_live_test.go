package cli_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

func expressionSearchRequest(t *testing.T, server *httptest.Server, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/knowledge/v1/search", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Kc-As", "agent:http-test")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, payload
}

func expressionLeaf(op string, fields map[string]any) map[string]any {
	clause := map[string]any{"op": op}
	for key, value := range fields {
		clause[key] = value
	}
	return map[string]any{"clause": clause}
}

func expressionHitObject(t *testing.T, response map[string]any) string {
	t.Helper()
	hits, ok := response["hits"].([]any)
	if !ok || len(hits) != 1 {
		t.Fatalf("expected one hit: %#v", response)
	}
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	return asMap(t, knowledge["knowledgeRef"])["object"].(string)
}

func TestHTTPExpressionSearchAllAnySortContinuationAndValidation(t *testing.T) {
	opensearchURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	if opensearchURL == "" {
		if os.Getenv("KC_REQUIRE_LIVE_ADAPTERS") == "1" {
			t.Fatal("KC_TEST_OPENSEARCH_URL is required")
		}
		t.Skip("run make test-e2e")
	}
	home := testkit.TempDir(t)
	repository := "kr://acme/public/search-expression"
	catalog := "kr://acme/catalog"
	workspace := "agent"
	body(t, kc(home, "init", "--catalog", catalog))
	body(t, kc(home, "store-set", "--index", "opensearch"))
	body(t, kc(home, "store-set", "--driver", "opensearch", "--url", opensearchURL))
	seedRepo(t, home, repository)
	body(t, kc(home, "put", "--command-id", "expression-schema", "--repo", repository,
		"--object", "schema/runbook.search", "--value", `{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]},"team":{"type":"string","access":["filter"]},"severity":{"type":"number","access":["filter","sort"]}}}`))
	for _, item := range []struct {
		id, value string
	}{
		{"runbook/payment", `{"body":"payment freeze procedure","team":"payments","severity":3}`},
		{"runbook/capacity", `{"body":"capacity alert procedure","team":"platform","severity":1}`},
		{"runbook/database", `{"body":"database restore procedure","team":"payments","severity":2}`},
		{"runbook/other", `{"body":"payment miscellany","team":"other","severity":0}`},
	} {
		body(t, kc(home, "put", "--command-id", "expression-"+strings.TrimPrefix(item.id, "runbook/"), "--repo", repository,
			"--object", item.id, "--schema-ref", "schema/runbook.search", "--value", item.value))
	}
	body(t, kc(home, "define-workspace", "--workspace", workspace, "--revision", "1", "--source", repository+"=refs/heads/main@"))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--cmd", "read-workspace", "--catalog", catalog, "--workspace", workspace))
	body(t, kc(home, "allow", "--principal", "agent:http-test", "--action", "knowledge.read,knowledge.search", "--repo", repository))
	syncIndexes(t, home, repository)

	handler := cli.HTTPHandler(home)
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	payment := expressionLeaf("MATCH", map[string]any{"value": "payment", "mode": "AllTerms"})
	database := expressionLeaf("MATCH", map[string]any{"value": "database", "mode": "AllTerms"})
	paymentsTeam := expressionLeaf("EQ", map[string]any{"path": "team", "value": "payments"})
	expression := map[string]any{"all": []any{
		map[string]any{"any": []any{payment, database}}, paymentsTeam,
	}}
	base := map[string]any{
		"catalog": catalog, "workspace": workspace, "expression": expression,
		"order": map[string]any{"op": "SORT", "path": "severity", "order": "asc"}, "limit": 1,
	}

	status, first := expressionSearchRequest(t, server, base)
	if status != http.StatusOK || first["completeness"] != "complete" || expressionHitObject(t, first) != "runbook/database" {
		t.Fatalf("first page: status=%d payload=%#v", status, first)
	}
	continuation, _ := first["continuation"].(string)
	if continuation == "" {
		t.Fatalf("first page needs continuation: %#v", first)
	}
	secondRequest := cloneSearchBody(base)
	secondRequest["continuation"] = continuation
	status, second := expressionSearchRequest(t, server, secondRequest)
	if status != http.StatusOK || expressionHitObject(t, second) != "runbook/payment" {
		t.Fatalf("second page: status=%d payload=%#v", status, second)
	}
	if next, _ := second["continuation"].(string); next != "" {
		t.Fatalf("second page unexpectedly continues: %#v", second)
	}

	differentGrouping := cloneSearchBody(base)
	differentGrouping["expression"] = map[string]any{"any": []any{
		map[string]any{"all": []any{payment, paymentsTeam}}, database,
	}}
	differentGrouping["continuation"] = continuation
	status, mismatch := expressionSearchRequest(t, server, differentGrouping)
	if status == http.StatusOK || asMap(t, mismatch["error"])["code"] != "PRECONDITION_FAILED" {
		t.Fatalf("continuation accepted different grouping: status=%d payload=%#v", status, mismatch)
	}

	status, invalid := expressionSearchRequest(t, server, map[string]any{
		"workspace": workspace, "query": "legacy", "expression": expression,
	})
	if status == http.StatusOK || asMap(t, invalid["error"])["code"] != "USAGE_INVALID" {
		t.Fatalf("mixed expression/legacy query: status=%d payload=%#v", status, invalid)
	}
	status, invalid = expressionSearchRequest(t, server, map[string]any{
		"workspace": workspace, "expression": map[string]any{"any": []any{}},
	})
	if status == http.StatusOK || asMap(t, invalid["error"])["code"] != "USAGE_INVALID" {
		t.Fatalf("empty Any: status=%d payload=%#v", status, invalid)
	}
}

func cloneSearchBody(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
